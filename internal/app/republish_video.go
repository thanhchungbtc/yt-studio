package app

import (
	"context"
	"fmt"

	"github.com/tbui/yt-studio/internal/domain/entity"
	"github.com/tbui/yt-studio/internal/domain/repository"
)

// RepublishVideo uploads a published video to YouTube a second time.
//
// It is the one deliberate door through the guard in PublishVideo, and it is
// separate from re-running the upload task for the reason that guard exists:
// YouTube has no way to replace a video's file, so publishing again is not a
// second attempt at the same thing. It is a second video. The first stays up,
// keeps its URL and whatever it has accumulated, and this app stops knowing
// about it — which is why the caller is expected to have said so out loud
// before reaching here.
//
// Forgetting the receipt is what makes the republish possible, and it is also
// the part that cannot be undone from inside this program: nothing else records
// the old URL. It is returned so the caller can say what was disowned.
func RepublishVideo(
	ctx context.Context,
	videos repository.VideoReader,
	fields repository.VideoFieldWriter,
	tasks repository.TaskReader,
	rerunner TaskRerunner,
	resumer GraphResumer,
	key string,
) (entity.UploadRecord, error) {
	v, err := GetVideo(ctx, videos, key)
	if err != nil {
		return entity.UploadRecord{}, err
	}
	if v.Upload == nil {
		return entity.UploadRecord{}, fmt.Errorf(
			"%w: %s has not been published, so there is nothing to publish again", ErrConflict, v.Ref)
	}
	previous := *v.Upload

	// The receipt goes first. PublishVideo reads it and refuses while it is
	// there, so clearing it is not bookkeeping after the fact — it is what the
	// re-run below is permitted by.
	if err := fields.ClearVideoUpload(ctx, v.ID); err != nil {
		return entity.UploadRecord{}, err
	}

	upload := entity.NewTaskID(v.ID, entity.TaskKindUpload, -1, -1)
	// Through readmitting, because a published video is a finished one and the
	// loop does not hold graphs that finished before the last restart. Being
	// finished is not a reason the upload cannot run again — it is the only
	// state from which this is ever asked.
	if _, err := readmitting(ctx, tasks, resumer, v.ID, func() (RerunPlan, error) {
		return RerunTasks(ctx, tasks, rerunner, nil, v.ID, []entity.TaskID{upload}, false)
	}); err != nil {
		// The receipt is already gone. Saying so is the difference between a
		// video the operator knows to re-publish by hand and one that quietly
		// forgot where it lives.
		return previous, fmt.Errorf("re-run upload of %s (its upload record was cleared: %s): %w",
			v.Ref, previous.URL, err)
	}
	return previous, nil
}
