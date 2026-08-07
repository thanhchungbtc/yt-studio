package app

import (
	"context"
	"fmt"
	"strings"

	"github.com/tbui/yt-studio/domain/entity"
	"github.com/tbui/yt-studio/domain/repository"
)

// RegenerateThumbnailIcon rewrites one cell's prompt and redraws that cell —
// the icon counterpart of RegenerateChapterSlide, and welded together for the
// same reason. It is the narrow alternative to re-running thumbnail_plan, which
// would rewrite every caption to fix one tile.
//
// Only the subject is edited; the grid's shared style clause is appended at
// generation, so editing one cell cannot make it the odd one out.
//
//nolint:revive // the parameter list is the dependency list
func RegenerateThumbnailIcon(
	ctx context.Context,
	videos repository.VideoReader,
	fields repository.VideoFieldWriter,
	rerunner TaskRerunner,
	videoID entity.VideoID,
	index int,
	prompt string,
) (entity.Video, error) {
	prompt = strings.TrimSpace(prompt)
	if prompt == "" {
		return entity.Video{}, Invalid("prompt", "must not be empty")
	}
	if index < 0 {
		return entity.Video{}, Invalid("index", "must not be negative")
	}
	v, err := videos.VideoByID(ctx, videoID)
	if err != nil {
		return entity.Video{}, err
	}
	if v.ThumbnailPlan == nil {
		return entity.Video{}, Invalid("index", "this video has no thumbnail plan yet")
	}
	// The grid cannot grow: one icon task exists per cell from expansion onward,
	// so a cell the plan does not have is a cell nothing would ever draw.
	if index >= len(v.ThumbnailPlan.Cells) {
		return entity.Video{}, Invalid("index", fmt.Sprintf(
			"the plan has %d cells", len(v.ThumbnailPlan.Cells)))
	}

	if err := fields.SetVideoThumbnailCellPrompt(ctx, videoID, index, prompt); err != nil {
		return entity.Video{}, err
	}
	v.ThumbnailPlan.Cells[index].Prompt = prompt

	// The tail below an icon is short, but it is what the operator judges at the
	// upload gate, so it is flagged rather than silently rebuilt.
	seed := entity.NewTaskID(videoID, entity.TaskKindThumbnailIcon, -1, index)
	if _, err := rerunner.Rerun(ctx, videoID, []entity.TaskID{seed}, false); err != nil {
		return entity.Video{}, err
	}
	return v, nil
}
