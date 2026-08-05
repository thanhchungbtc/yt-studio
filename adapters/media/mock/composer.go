package mock

import (
	"context"
	"errors"
	"fmt"
	"io"

	"golang.org/x/sync/errgroup"

	"github.com/tbui/yt-studio/domain/entity"
	"github.com/tbui/yt-studio/domain/provider"
)

// Composer is the mock composition backend. It muxes a chapter's slides and
// narration into a real MP4 container, and concatenates chapter clips into the
// final render by stream copy — never by re-reading a file into memory.
type Composer struct {
	store provider.AssetStore
}

var _ provider.VideoComposer = (*Composer)(nil)

// NewComposer constructs the mock.
func NewComposer(store provider.AssetStore) *Composer {
	return &Composer{store: store}
}

// Clip composes exactly one chapter.
func (c *Composer) Clip(ctx context.Context, req provider.ClipRequest) (entity.AssetID, error) {
	if len(req.SlideAssetIDs) == 0 {
		return "", errors.New("mock composer: a clip needs at least one slide")
	}

	video := make([]mp4Sample, 0, len(req.SlideAssetIDs))
	for _, id := range req.SlideAssetIDs {
		s, err := c.sample(ctx, id, entity.AssetKindImage, 0)
		if err != nil {
			return "", err
		}
		video = append(video, s)
	}

	var audio []mp4Sample
	if req.AudioAssetID != "" {
		// The WAV header is skipped so the MP4 carries raw PCM, which is what the
		// `sowt` sample entry declares.
		s, err := c.sample(ctx, req.AudioAssetID, entity.AssetKindAudio, wavHeaderBytes)
		if err != nil {
			return "", err
		}
		if s.size > 0 {
			audio = append(audio, s)
		}
	}
	return c.compose(ctx, entity.AssetKindClip, video, audio)
}

// Concat joins every chapter clip into the final render. Each input is re-read as
// byte ranges and copied straight through, so no clip is ever held in memory.
func (c *Composer) Concat(ctx context.Context, req provider.ConcatRequest) (entity.AssetID, error) {
	if len(req.ClipAssetIDs) == 0 {
		return "", errors.New("mock composer: a concat needs at least one clip")
	}

	video := make([]mp4Sample, 0, len(req.ClipAssetIDs)*2)
	audio := make([]mp4Sample, 0, len(req.ClipAssetIDs))
	for _, id := range req.ClipAssetIDs {
		info, err := c.store.Stat(ctx, id, entity.AssetKindClip)
		if err != nil {
			return "", fmt.Errorf("stat clip %s: %w", id.Short(), err)
		}
		tracks, err := readMP4(func() (io.ReadSeekCloser, error) {
			return c.store.Open(ctx, id, entity.AssetKindClip)
		}, info.Size)
		if err != nil {
			return "", fmt.Errorf("read clip %s: %w", id.Short(), err)
		}
		video = append(video, tracks.video...)
		audio = append(audio, tracks.audio...)
	}
	return c.compose(ctx, entity.AssetKindFinal, video, audio)
}

// compose pipes the writer straight into the content-addressed store, so the
// output is hashed as it is produced and never lands in a buffer first.
func (c *Composer) compose(ctx context.Context, kind entity.AssetKind, video, audio []mp4Sample) (entity.AssetID, error) {
	pr, pw := io.Pipe()
	var stored provider.StoredAsset

	g, gctx := errgroup.WithContext(ctx)
	g.Go(func() error {
		_, err := writeMP4(pw, video, audio)
		// Closing with the error propagates it to the reader rather than letting Put
		// see a clean EOF on a truncated stream.
		return pw.CloseWithError(err)
	})
	g.Go(func() error {
		var err error
		stored, err = c.store.Put(gctx, kind, pr)
		return errors.Join(err, pr.CloseWithError(err))
	})
	if err := g.Wait(); err != nil {
		return "", fmt.Errorf("compose %s: %w", kind, err)
	}
	return stored.ID, nil
}

// sample describes a byte range of a stored asset, opened on demand.
func (c *Composer) sample(ctx context.Context, id entity.AssetID, kind entity.AssetKind, skip int64) (mp4Sample, error) {
	info, err := c.store.Stat(ctx, id, kind)
	if err != nil {
		return mp4Sample{}, fmt.Errorf("stat %s %s: %w", kind, id.Short(), err)
	}
	if skip > info.Size {
		return mp4Sample{}, fmt.Errorf("%s %s is shorter than its header", kind, id.Short())
	}
	return sectionSample(func() (io.ReadSeekCloser, error) {
		return c.store.Open(ctx, id, kind)
	}, skip, info.Size-skip), nil
}
