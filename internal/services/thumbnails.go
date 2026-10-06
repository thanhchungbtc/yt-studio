package services

import (
	"bytes"
	"context"
	"encoding/json"

	"github.com/tbui/yt-studio/internal/app"
	"github.com/tbui/yt-studio/internal/domain/entity"
)

// ThumbnailService edits a video's thumbnail.
type ThumbnailService struct{ b *Backend }

// NewThumbnailService creates a ThumbnailService.
func NewThumbnailService(b *Backend) *ThumbnailService { return &ThumbnailService{b: b} }

// SaveDesign stores the builder's opaque document.
func (s *ThumbnailService) SaveDesign(ctx context.Context, key string, design any) (VideoDTO, error) {
	v, err := s.b.video(ctx, key)
	if err != nil {
		return VideoDTO{}, err
	}
	encoded, err := json.Marshal(design)
	if err != nil {
		return VideoDTO{}, app.Invalid("design", "must be JSON")
	}
	v, err = app.SaveThumbnailDesign(ctx, s.b.Store, s.b.Store, v.ID, entity.ThumbnailDesign(encoded))
	if err != nil {
		return VideoDTO{}, err
	}
	return s.b.videoDTO(ctx, v)
}

// ApplyOverride makes a PNG from the builder the published thumbnail.
func (s *ThumbnailService) ApplyOverride(ctx context.Context, key string, png []byte) (VideoDTO, error) {
	if len(png) > entity.MaxThumbnailBytes {
		return VideoDTO{}, app.Invalid("thumbnail", "is too large")
	}
	v, err := s.b.video(ctx, key)
	if err != nil {
		return VideoDTO{}, err
	}
	v, err = app.ApplyThumbnailOverride(ctx, s.b.Store, s.b.Store, s.b.Store, s.b.Assets,
		v.ID, bytes.NewReader(png), s.b.Now())
	if err != nil {
		return VideoDTO{}, err
	}
	return s.b.videoDTO(ctx, v)
}

// ClearOverride reverts to the rendered thumbnail; the design is kept.
func (s *ThumbnailService) ClearOverride(ctx context.Context, key string) (VideoDTO, error) {
	v, err := s.b.video(ctx, key)
	if err != nil {
		return VideoDTO{}, err
	}
	v, err = app.ClearThumbnailOverride(ctx, s.b.Store, s.b.Store, v.ID)
	if err != nil {
		return VideoDTO{}, err
	}
	return s.b.videoDTO(ctx, v)
}

// RegenerateIcon redraws one grid cell from an edited prompt.
func (s *ThumbnailService) RegenerateIcon(ctx context.Context, key string, index int, prompt string) (VideoDTO, error) {
	if index < 0 {
		return VideoDTO{}, app.Invalid("index", "must not be negative")
	}
	if prompt == "" {
		return VideoDTO{}, app.Invalid("prompt", "is required")
	}
	v, err := s.b.video(ctx, key)
	if err != nil {
		return VideoDTO{}, err
	}
	v, err = app.RegenerateThumbnailIcon(ctx, s.b.Store, s.b.Store, s.b.Store, s.b.Scheduler, s.b.Scheduler,
		v.ID, index, prompt)
	if err != nil {
		return VideoDTO{}, err
	}
	return s.b.videoDTO(ctx, v)
}

// Push sends a published video's thumbnail to YouTube again.
func (s *ThumbnailService) Push(ctx context.Context, key string) (VideoDTO, error) {
	v, err := s.b.video(ctx, key)
	if err != nil {
		return VideoDTO{}, err
	}
	v, err = app.PushVideoThumbnail(ctx, s.b.Store, s.b.Store, s.b.Uploader, v.ID,
		s.b.Settings.Bool(entity.SettingUploadDryRun))
	if err != nil {
		return VideoDTO{}, err
	}
	return s.b.videoDTO(ctx, v)
}
