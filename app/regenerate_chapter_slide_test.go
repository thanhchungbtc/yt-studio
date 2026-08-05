package app_test

import (
	"context"
	"errors"
	"testing"

	"github.com/tbui/yt-studio/app"
	"github.com/tbui/yt-studio/domain/entity"
	"github.com/tbui/yt-studio/domain/repository"
)

// Editing a prompt and redrawing the slide is one operation, and these tests
// are about the two halves staying welded: the prompt reaches the row by index,
// and exactly one slide task is seeded with it.

type promptChapters struct {
	chapter entity.Chapter
	// writes records index/prompt pairs in order, so a test can tell an indexed
	// write from a whole-slice one.
	writes    [][2]any
	writeErr  error
	notified  int
	seeds     [][]entity.TaskID
	rerunErr  error
	rerunSeen bool
}

var (
	_ repository.ChapterReader      = (*promptChapters)(nil)
	_ repository.ChapterFieldWriter = (*promptChapters)(nil)
	_ app.TaskRerunner              = (*promptChapters)(nil)
	_ app.ChapterNotifier           = (*promptChapters)(nil)
)

func (p *promptChapters) ChapterByID(context.Context, entity.ChapterID) (entity.Chapter, error) {
	return p.chapter, nil
}

func (p *promptChapters) ListChaptersByVideo(context.Context, entity.VideoID) ([]entity.Chapter, error) {
	return []entity.Chapter{p.chapter}, nil
}

func (p *promptChapters) SetChapterPrompt(_ context.Context, _ entity.ChapterID, index int, prompt string) error {
	if p.writeErr != nil {
		return p.writeErr
	}
	p.writes = append(p.writes, [2]any{index, prompt})
	return nil
}

func (p *promptChapters) SetChapterScript(context.Context, entity.ChapterID, string, float64) error {
	return errors.New("not used")
}

func (p *promptChapters) SetChapterPrompts(context.Context, entity.ChapterID, []string) error {
	return errors.New("not used")
}

func (p *promptChapters) SetChapterAudio(context.Context, entity.ChapterID, entity.AssetID) error {
	return errors.New("not used")
}

func (p *promptChapters) SetChapterSlide(context.Context, entity.ChapterID, int, entity.AssetID) error {
	return errors.New("not used")
}

func (p *promptChapters) SetChapterClip(context.Context, entity.ChapterID, entity.AssetID) error {
	return errors.New("not used")
}

func (p *promptChapters) Rerun(
	_ context.Context,
	_ entity.VideoID,
	seeds []entity.TaskID,
	_ bool,
) ([]entity.TaskID, error) {
	p.rerunSeen = true
	if p.rerunErr != nil {
		return nil, p.rerunErr
	}
	p.seeds = append(p.seeds, seeds)
	return nil, nil
}

func (p *promptChapters) NotifyChapter(entity.ChapterDelta) { p.notified++ }

func newPromptChapters() *promptChapters {
	return &promptChapters{
		chapter: entity.Chapter{
			ID:           entity.NewChapterID("v1", 7),
			VideoID:      "v1",
			Ordinal:      7,
			Title:        "A chapter",
			SlidePrompts: []string{"first", "second"},
		},
	}
}

func TestRegenerateChapterSlideWritesThenSeedsOneImageTask(t *testing.T) {
	t.Parallel()
	fake := newPromptChapters()

	got, err := app.RegenerateChapterSlide(context.Background(), fake, fake, fake, fake,
		fake.chapter.ID, 1, "  a lighthouse at dusk  ")
	if err != nil {
		t.Fatalf("regenerate: %v", err)
	}

	if len(fake.writes) != 1 {
		t.Fatalf("want one indexed write, got %d", len(fake.writes))
	}
	if fake.writes[0][0] != 1 || fake.writes[0][1] != "a lighthouse at dusk" {
		t.Fatalf("wrote %v, want [1 a lighthouse at dusk]", fake.writes[0])
	}
	if len(fake.seeds) != 1 || len(fake.seeds[0]) != 1 {
		t.Fatalf("want exactly one seed, got %v", fake.seeds)
	}
	if want := entity.NewTaskID("v1", entity.TaskKindSlide, 7, 1); fake.seeds[0][0] != want {
		t.Fatalf("seeded %s, want %s", fake.seeds[0][0], want)
	}
	// The returned chapter is what the operator's editor rebinds to, so it has to
	// carry the trimmed text rather than the row as it was read.
	if got.SlidePrompts[1] != "a lighthouse at dusk" {
		t.Fatalf("returned prompt %q", got.SlidePrompts[1])
	}
	if got.SlidePrompts[0] != "first" {
		t.Fatalf("sibling prompt clobbered: %q", got.SlidePrompts[0])
	}
	if fake.notified != 1 {
		t.Fatalf("notified %d times, want 1", fake.notified)
	}
}

// An index past the end would be appended by json_set, leaving a prompt no task
// will ever read: the image width was fixed when the graph was expanded.
func TestRegenerateChapterSlideRejectsIndexPastTheEnd(t *testing.T) {
	t.Parallel()
	fake := newPromptChapters()

	_, err := app.RegenerateChapterSlide(context.Background(), fake, fake, fake, fake,
		fake.chapter.ID, 2, "a third slide")
	if !errors.Is(err, app.ErrValidation) {
		t.Fatalf("err = %v, want ErrValidation", err)
	}
	if len(fake.writes) != 0 || fake.rerunSeen {
		t.Fatalf("rejected input still wrote %v / ran %v", fake.writes, fake.rerunSeen)
	}
}

func TestRegenerateChapterSlideRejectsAnEmptyPrompt(t *testing.T) {
	t.Parallel()
	fake := newPromptChapters()

	_, err := app.RegenerateChapterSlide(context.Background(), fake, fake, fake, fake,
		fake.chapter.ID, 0, "   ")
	if !errors.Is(err, app.ErrValidation) {
		t.Fatalf("err = %v, want ErrValidation", err)
	}
	if len(fake.writes) != 0 || fake.rerunSeen {
		t.Fatalf("rejected input still wrote %v / ran %v", fake.writes, fake.rerunSeen)
	}
}

// The prompt has to be on the row before the task is admitted, because
// GenerateSlide reads it at dispatch. So a scheduler that refuses leaves the
// edit committed and nothing running — the state a second press resolves.
func TestRegenerateChapterSlideKeepsTheEditWhenTheRerunFails(t *testing.T) {
	t.Parallel()
	fake := newPromptChapters()
	fake.rerunErr = errors.New("scheduler closed")

	if _, err := app.RegenerateChapterSlide(context.Background(), fake, fake, fake, fake,
		fake.chapter.ID, 0, "a new prompt"); err == nil {
		t.Fatal("want the scheduler error surfaced")
	}
	if len(fake.writes) != 1 {
		t.Fatalf("want the edit committed anyway, got %v", fake.writes)
	}
	if fake.notified != 0 {
		t.Fatal("a failed re-run must not announce the chapter as changed")
	}
}
