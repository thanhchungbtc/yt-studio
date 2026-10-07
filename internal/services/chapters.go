package services

import (
	"context"

	"github.com/tbui/yt-studio/internal/app"
	"github.com/tbui/yt-studio/internal/domain/entity"
)

// ChapterService reads and edits a video's chapters.
type ChapterService struct{ b *Backend }

// NewChapterService creates a ChapterService.
func NewChapterService(b *Backend) *ChapterService { return &ChapterService{b: b} }

// ChapterPlan is what the blueprint plans for a chapter.
type ChapterPlan struct {
	Title   string `json:"title"`
	Summary string `json:"summary"`
	// EstimatedWords is the spoken-word budget; 0 leaves it unset.
	EstimatedWords int `json:"estimatedWords"`
}

// List returns a video's chapters in order.
func (s *ChapterService) List(ctx context.Context, video string) ([]ChapterDTO, error) {
	v, err := s.b.video(ctx, video)
	if err != nil {
		return nil, err
	}
	rows, err := app.ListChapters(ctx, s.b.Store, v.ID)
	if err != nil {
		return nil, err
	}
	out := make([]ChapterDTO, 0, len(rows))
	for _, c := range rows {
		out = append(out, chapterFrom(c))
	}
	return out, nil
}

// UpdatePlan edits a chapter's plan. Nothing re-runs.
func (s *ChapterService) UpdatePlan(ctx context.Context, id string, plan ChapterPlan) (ChapterDTO, error) {
	if plan.Title == "" {
		return ChapterDTO{}, app.Invalid("title", "is required")
	}
	if plan.EstimatedWords < 0 {
		return ChapterDTO{}, app.Invalid("estimatedWords", "must not be negative")
	}
	c, err := app.UpdateChapterPlan(ctx, s.b.Store, s.b.Store, s.b.Notifier, entity.ChapterID(id), app.ChapterPlan{
		Title:          plan.Title,
		Summary:        plan.Summary,
		EstimatedWords: plan.EstimatedWords,
	})
	if err != nil {
		return ChapterDTO{}, err
	}
	return chapterFrom(c), nil
}

// UpdateScript replaces a chapter's narration, flagging what it produced stale.
func (s *ChapterService) UpdateScript(ctx context.Context, id, script string) (ChapterDTO, error) {
	if script == "" {
		return ChapterDTO{}, app.Invalid("script", "is required")
	}
	c, err := app.UpdateChapterScript(ctx, s.b.Store, s.b.Store, s.b.Store, s.b.Notifier, s.b.Scheduler,
		s.b.Scheduler, entity.ChapterID(id), script)
	if err != nil {
		return ChapterDTO{}, err
	}
	return chapterFrom(c), nil
}

// RegenerateSlide redraws one slide from an edited prompt.
func (s *ChapterService) RegenerateSlide(ctx context.Context, id string, index int, prompt string) (ChapterDTO, error) {
	if index < 0 {
		return ChapterDTO{}, app.Invalid("index", "must not be negative")
	}
	if prompt == "" {
		return ChapterDTO{}, app.Invalid("prompt", "is required")
	}
	c, err := app.RegenerateChapterSlide(ctx, s.b.Store, s.b.Store, s.b.Store, s.b.Scheduler, s.b.Scheduler,
		s.b.Notifier, entity.ChapterID(id), index, prompt)
	if err != nil {
		return ChapterDTO{}, err
	}
	return chapterFrom(c), nil
}
