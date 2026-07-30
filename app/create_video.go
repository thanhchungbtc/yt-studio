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
	ImagesPerChapter int
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
	defaultImages int,
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
	images := in.ImagesPerChapter
	if images == 0 {
		images = defaultImages
	}

	seq, err := seqs.NextVideoSeq(ctx, channel.ID)
	if err != nil {
		return entity.Video{}, fmt.Errorf("mint video ref: %w", err)
	}
	ref, err := entity.NewRef(channel.Slug, seq)
	if err != nil {
		return entity.Video{}, fmt.Errorf("%w: %w", ErrValidation, err)
	}

	v, err := entity.NewVideo(entity.VideoID(newID()), channel.ID, ref, in.Title, in.Topic, chapters, images, now)
	if err != nil {
		return entity.Video{}, fmt.Errorf("%w: %w", ErrValidation, err)
	}
	if err := videos.CreateVideo(ctx, v); err != nil {
		return entity.Video{}, err
	}
	return v, nil
}
