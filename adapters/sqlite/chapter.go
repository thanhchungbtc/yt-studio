package sqlite

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/tbui/yt-studio/adapters/sqlite/sqlcgen"
	"github.com/tbui/yt-studio/domain/entity"
	"github.com/tbui/yt-studio/domain/repository"
)

var (
	_ repository.ChapterReader = (*Store)(nil)
	_ repository.ChapterWriter = (*Store)(nil)
)

// ChapterByID reads one chapter.
func (s *Store) ChapterByID(ctx context.Context, id entity.ChapterID) (entity.Chapter, error) {
	row, err := s.rq.GetChapterByID(ctx, string(id))
	if err != nil {
		return entity.Chapter{}, wrapNotFound(err, "chapter", string(id))
	}
	return chapterFromRow(row)
}

// ListChaptersByVideo reads a video's chapters in ordinal order.
func (s *Store) ListChaptersByVideo(ctx context.Context, videoID entity.VideoID) ([]entity.Chapter, error) {
	rows, err := s.rq.ListChaptersByVideo(ctx, string(videoID))
	if err != nil {
		return nil, fmt.Errorf("list chapters of %s: %w", videoID, err)
	}
	out := make([]entity.Chapter, 0, len(rows))
	for _, r := range rows {
		c, err := chapterFromRow(r)
		if err != nil {
			return nil, fmt.Errorf("decode chapter %s: %w", r.ID, err)
		}
		out = append(out, c)
	}
	return out, nil
}

// ReplaceChapters swaps a video's whole chapter set in one transaction. It is
// the blueprint-approval path: an approved outline defines the chapters.
func (s *Store) ReplaceChapters(ctx context.Context, videoID entity.VideoID, chapters []entity.Chapter) error {
	params := make([]sqlcgen.UpsertChapterParams, 0, len(chapters))
	for _, c := range chapters {
		p, err := chapterParams(c)
		if err != nil {
			return err
		}
		params = append(params, p)
	}
	return s.doTx(ctx, func(ctx context.Context, q *sqlcgen.Queries) error {
		if err := q.DeleteChaptersByVideo(ctx, string(videoID)); err != nil {
			return fmt.Errorf("clear chapters of %s: %w", videoID, err)
		}
		for _, p := range params {
			if err := q.UpsertChapter(ctx, p); err != nil {
				return fmt.Errorf("insert chapter %s: %w", p.ID, err)
			}
		}
		return nil
	})
}

// UpdateChapter writes back one chapter, including an operator's script edit.
func (s *Store) UpdateChapter(ctx context.Context, c entity.Chapter) error {
	p, err := chapterParams(c)
	if err != nil {
		return err
	}
	return s.do(ctx, func(ctx context.Context, q *sqlcgen.Queries) error {
		return q.UpsertChapter(ctx, p)
	})
}

func chapterParams(c entity.Chapter) (sqlcgen.UpsertChapterParams, error) {
	prompts := c.ImagePrompts
	if prompts == nil {
		prompts = []string{}
	}
	images := c.ImageAssetIDs
	if images == nil {
		images = []entity.AssetID{}
	}
	promptsJSON, err := encodeJSON(prompts)
	if err != nil {
		return sqlcgen.UpsertChapterParams{}, fmt.Errorf("encode image prompts: %w", err)
	}
	imagesJSON, err := encodeJSON(images)
	if err != nil {
		return sqlcgen.UpsertChapterParams{}, fmt.Errorf("encode image asset ids: %w", err)
	}
	return sqlcgen.UpsertChapterParams{
		ID:                string(c.ID),
		VideoID:           string(c.VideoID),
		Ordinal:           int64(c.Ordinal),
		Title:             c.Title,
		Summary:           c.Summary,
		Script:            c.Script,
		ImagePromptsJson:  promptsJSON,
		AudioAssetID:      assetIDPtr(c.AudioAssetID),
		ImageAssetIdsJson: imagesJSON,
		ClipAssetID:       assetIDPtr(c.ClipAssetID),
		DurationSeconds:   c.DurationSeconds,
		EstimatedWords:    int64(c.EstimatedWords),
		CreatedAt:         toUnix(c.CreatedAt),
		UpdatedAt:         toUnix(c.UpdatedAt),
	}, nil
}

var _ repository.ChapterFieldWriter = (*Store)(nil)

// SetChapterScript records a generated or operator-edited narration.
func (s *Store) SetChapterScript(ctx context.Context, id entity.ChapterID, script string, durationSeconds float64) error {
	return s.do(ctx, func(ctx context.Context, q *sqlcgen.Queries) error {
		return q.SetChapterScript(ctx, sqlcgen.SetChapterScriptParams{
			Script:          script,
			DurationSeconds: durationSeconds,
			UpdatedAt:       toUnix(time.Now()),
			ID:              string(id),
		})
	})
}

// SetChapterPrompts records the chapter's slice of the coalesced prompt batch.
func (s *Store) SetChapterPrompts(ctx context.Context, id entity.ChapterID, prompts []string) error {
	if prompts == nil {
		prompts = []string{}
	}
	encoded, err := encodeJSON(prompts)
	if err != nil {
		return fmt.Errorf("encode image prompts: %w", err)
	}
	return s.do(ctx, func(ctx context.Context, q *sqlcgen.Queries) error {
		return q.SetChapterPrompts(ctx, sqlcgen.SetChapterPromptsParams{
			ImagePromptsJson: encoded,
			UpdatedAt:        toUnix(time.Now()),
			ID:               string(id),
		})
	})
}

// SetChapterPrompt replaces one prompt at its index, for an operator redrawing
// a single still. Indexed like SetChapterImage so it cannot carry back a stale
// copy of its siblings.
func (s *Store) SetChapterPrompt(ctx context.Context, id entity.ChapterID, index int, prompt string) error {
	if index < 0 {
		return fmt.Errorf("%w: prompt index must not be negative", entity.ErrInvalidChapter)
	}
	path := "$[" + strconv.Itoa(index) + "]"
	return s.do(ctx, func(ctx context.Context, q *sqlcgen.Queries) error {
		return q.SetChapterPrompt(ctx, sqlcgen.SetChapterPromptParams{
			Path:      path,
			Prompt:    prompt,
			UpdatedAt: toUnix(time.Now()),
			ID:        string(id),
		})
	})
}

// SetChapterAudio records the narration asset.
func (s *Store) SetChapterAudio(ctx context.Context, id entity.ChapterID, assetID entity.AssetID) error {
	value := string(assetID)
	return s.do(ctx, func(ctx context.Context, q *sqlcgen.Queries) error {
		return q.SetChapterAudio(ctx, sqlcgen.SetChapterAudioParams{
			AudioAssetID: &value,
			UpdatedAt:    toUnix(time.Now()),
			ID:           string(id),
		})
	})
}

// SetChapterImage records one still at its index. json_set makes this a single
// atomic statement, so two concurrent image tasks cannot lose each other's
// write.
func (s *Store) SetChapterImage(ctx context.Context, id entity.ChapterID, index int, assetID entity.AssetID) error {
	if index < 0 {
		return fmt.Errorf("%w: image index must not be negative", entity.ErrInvalidChapter)
	}
	path := "$[" + strconv.Itoa(index) + "]"
	return s.do(ctx, func(ctx context.Context, q *sqlcgen.Queries) error {
		return q.SetChapterImage(ctx, sqlcgen.SetChapterImageParams{
			Path:      path,
			AssetID:   string(assetID),
			UpdatedAt: toUnix(time.Now()),
			ID:        string(id),
		})
	})
}

// SetChapterClip records the composed clip.
func (s *Store) SetChapterClip(ctx context.Context, id entity.ChapterID, assetID entity.AssetID) error {
	value := string(assetID)
	return s.do(ctx, func(ctx context.Context, q *sqlcgen.Queries) error {
		return q.SetChapterClip(ctx, sqlcgen.SetChapterClipParams{
			ClipAssetID: &value,
			UpdatedAt:   toUnix(time.Now()),
			ID:          string(id),
		})
	})
}
