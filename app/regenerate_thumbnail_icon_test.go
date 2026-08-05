package app_test

import (
	"context"
	"errors"
	"testing"

	"github.com/tbui/yt-studio/app"
	"github.com/tbui/yt-studio/domain/entity"
	"github.com/tbui/yt-studio/domain/repository"
)

// The icon counterpart of the slide tests: the edit reaches one cell, and one
// icon task is seeded with it. What differs is the shape of the target — a cell
// inside the plan, addressed by index, whose caption must survive.

type iconVideo struct {
	video     entity.Video
	writes    [][2]any
	seeds     [][]entity.TaskID
	rerunErr  error
	rerunSeen bool
}

var (
	_ repository.VideoReader      = (*iconVideo)(nil)
	_ repository.VideoFieldWriter = (*iconVideo)(nil)
	_ app.TaskRerunner            = (*iconVideo)(nil)
)

func (v *iconVideo) VideoByID(context.Context, entity.VideoID) (entity.Video, error) {
	return v.video, nil
}

func (v *iconVideo) VideoByRef(context.Context, entity.Ref) (entity.Video, error) {
	return v.video, nil
}

func (v *iconVideo) ListVideos(context.Context, repository.VideoFilter) ([]entity.Video, error) {
	return []entity.Video{v.video}, nil
}

func (v *iconVideo) CountVideos(context.Context, repository.VideoFilter) (int, error) {
	return 1, nil
}

func (v *iconVideo) SetVideoThumbnailCellPrompt(
	_ context.Context,
	_ entity.VideoID,
	index int,
	prompt string,
) error {
	v.writes = append(v.writes, [2]any{index, prompt})
	return nil
}

func (v *iconVideo) SetVideoBlueprintAsset(context.Context, entity.VideoID, entity.AssetID) error {
	return errors.New("not used")
}

func (v *iconVideo) SetVideoFinalAsset(context.Context, entity.VideoID, entity.AssetID) error {
	return errors.New("not used")
}

func (v *iconVideo) SetVideoThumbnailPlan(context.Context, entity.VideoID, entity.ThumbnailPlan) error {
	return errors.New("not used")
}

func (v *iconVideo) SetVideoThumbnailIcon(context.Context, entity.VideoID, int, entity.AssetID) error {
	return errors.New("not used")
}

func (v *iconVideo) SetVideoThumbnailAsset(context.Context, entity.VideoID, entity.AssetID) error {
	return errors.New("not used")
}

func (v *iconVideo) SetVideoMetadata(context.Context, entity.VideoID, entity.Metadata) error {
	return errors.New("not used")
}

func (v *iconVideo) SetVideoUpload(context.Context, entity.VideoID, entity.UploadRecord) error {
	return errors.New("not used")
}

func (v *iconVideo) Rerun(
	_ context.Context,
	_ entity.VideoID,
	seeds []entity.TaskID,
	_ bool,
) ([]entity.TaskID, error) {
	v.rerunSeen = true
	if v.rerunErr != nil {
		return nil, v.rerunErr
	}
	v.seeds = append(v.seeds, seeds)
	return nil, nil
}

func newIconVideo() *iconVideo {
	return &iconVideo{
		video: entity.Video{
			ID:             "v1",
			Ref:            "DSS-1",
			ThumbnailCells: 3,
			ThumbnailPlan: &entity.ThumbnailPlan{Cells: []entity.ThumbnailCell{
				{Caption: "ONE", Prompt: "an alarm clock"},
				{Caption: "TWO", Prompt: "a cold window"},
				{Caption: "THREE", Prompt: "a quiet street"},
			}},
		},
	}
}

func TestRegenerateThumbnailIconWritesOneCellAndSeedsItsTask(t *testing.T) {
	t.Parallel()
	fake := newIconVideo()

	got, err := app.RegenerateThumbnailIcon(context.Background(), fake, fake, fake,
		"v1", 1, "  a frosted window at night  ")
	if err != nil {
		t.Fatalf("regenerate: %v", err)
	}

	if len(fake.writes) != 1 || fake.writes[0][0] != 1 || fake.writes[0][1] != "a frosted window at night" {
		t.Fatalf("wrote %v", fake.writes)
	}
	if len(fake.seeds) != 1 || len(fake.seeds[0]) != 1 {
		t.Fatalf("want exactly one seed, got %v", fake.seeds)
	}
	// Icons carry no ordinal: the grid belongs to the video, not to a chapter.
	if want := entity.NewTaskID("v1", entity.TaskKindThumbnailIcon, -1, 1); fake.seeds[0][0] != want {
		t.Fatalf("seeded %s, want %s", fake.seeds[0][0], want)
	}
	// The caption is the plan's, not the operator's; editing what a cell pictures
	// must not touch what it says.
	if got.ThumbnailPlan.Cells[1].Caption != "TWO" {
		t.Fatalf("caption became %q", got.ThumbnailPlan.Cells[1].Caption)
	}
	if got.ThumbnailPlan.Cells[1].Prompt != "a frosted window at night" {
		t.Fatalf("returned prompt %q", got.ThumbnailPlan.Cells[1].Prompt)
	}
	if got.ThumbnailPlan.Cells[0].Prompt != "an alarm clock" {
		t.Fatalf("neighbouring cell clobbered: %q", got.ThumbnailPlan.Cells[0].Prompt)
	}
}

// The grid cannot grow: one icon task per cell exists from expansion onward, so
// a cell beyond the plan is one nothing would ever draw.
func TestRegenerateThumbnailIconRejectsACellTheGridDoesNotHave(t *testing.T) {
	t.Parallel()
	fake := newIconVideo()

	_, err := app.RegenerateThumbnailIcon(context.Background(), fake, fake, fake,
		"v1", 3, "a fourth tile")
	if !errors.Is(err, app.ErrValidation) {
		t.Fatalf("err = %v, want ErrValidation", err)
	}
	if len(fake.writes) != 0 || fake.rerunSeen {
		t.Fatalf("rejected input still wrote %v / ran %v", fake.writes, fake.rerunSeen)
	}
}

func TestRegenerateThumbnailIconRejectsAVideoWithNoPlan(t *testing.T) {
	t.Parallel()
	fake := newIconVideo()
	fake.video.ThumbnailPlan = nil

	_, err := app.RegenerateThumbnailIcon(context.Background(), fake, fake, fake,
		"v1", 0, "a cell of a plan that does not exist")
	if !errors.Is(err, app.ErrValidation) {
		t.Fatalf("err = %v, want ErrValidation", err)
	}
	if len(fake.writes) != 0 || fake.rerunSeen {
		t.Fatalf("rejected input still wrote %v / ran %v", fake.writes, fake.rerunSeen)
	}
}

func TestRegenerateThumbnailIconRejectsAnEmptyPrompt(t *testing.T) {
	t.Parallel()
	fake := newIconVideo()

	_, err := app.RegenerateThumbnailIcon(context.Background(), fake, fake, fake, "v1", 0, "\t\n ")
	if !errors.Is(err, app.ErrValidation) {
		t.Fatalf("err = %v, want ErrValidation", err)
	}
	if len(fake.writes) != 0 || fake.rerunSeen {
		t.Fatalf("rejected input still wrote %v / ran %v", fake.writes, fake.rerunSeen)
	}
}
