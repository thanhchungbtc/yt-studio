package app

import (
	"context"
	"fmt"
	"time"

	"github.com/tbui/yt-studio/domain/entity"
	"github.com/tbui/yt-studio/domain/repository"
)

// CreateVideoInput is the boundary shape for a new video. A zero chapter or
// image count falls back to the caller-supplied defaults, which come from
// settings rows.
type CreateVideoInput struct {
	ChannelKey       string
	Title            string
	Topic            string
	ChapterCount     int
	SlidesPerChapter int
	// ThumbnailCells is how many tiles the grid gets; zero takes the default. It
	// is kept on the video because the DAG cannot change width afterwards.
	ThumbnailCells int
	// TargetDurationMinutes is optional; zero leaves the length to the chapters.
	TargetDurationMinutes int
}

// CreateVideo mints a video in the draft state, with a ref taken from the
// channel's counter (DSS-1, DSS-2...). Nothing is scheduled until StartVideo.
func CreateVideo(
	ctx context.Context,
	channels repository.ChannelReader,
	seqs repository.ChannelWriter,
	videos repository.VideoWriter,
	newID func() string,
	now time.Time,
	defaultChapters int,
	defaultSlides int,
	defaultThumbnailCells int,
	in CreateVideoInput,
) (entity.Video, error) {
	channel, err := GetChannel(ctx, channels, in.ChannelKey)
	if err != nil {
		return entity.Video{}, err
	}
	chapters := in.ChapterCount
	if chapters == 0 {
		chapters = defaultChapters
	}
	slides := in.SlidesPerChapter
	if slides == 0 {
		slides = defaultSlides
	}
	cells := in.ThumbnailCells
	if cells == 0 {
		cells = defaultThumbnailCells
	}

	seq, err := seqs.NextVideoSeq(ctx, channel.ID)
	if err != nil {
		return entity.Video{}, fmt.Errorf("mint video ref: %w", err)
	}
	ref, err := entity.NewRef(channel.Slug, seq)
	if err != nil {
		return entity.Video{}, fmt.Errorf("%w: %w", ErrValidation, err)
	}

	v, err := entity.NewVideo(entity.VideoID(newID()), channel.ID, ref, in.Title, in.Topic,
		chapters, slides, cells, in.TargetDurationMinutes, now)
	if err != nil {
		return entity.Video{}, fmt.Errorf("%w: %w", ErrValidation, err)
	}
	if err := videos.CreateVideo(ctx, v); err != nil {
		return entity.Video{}, err
	}
	return v, nil
}
