package app

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/tbui/yt-studio/domain/entity"
	"github.com/tbui/yt-studio/domain/provider"
	"github.com/tbui/yt-studio/domain/repository"
	"github.com/tbui/yt-studio/domain/scheduler"
)

// BlueprintOptions are the blueprint task's settings-sourced inputs: the
// tolerance its chapter count is judged against, and the shape it expands into.
type BlueprintOptions struct {
	ChapterTolerancePercent int
	MaxAttempts             int
	UploadGate              bool
}

// Expand narrows the options to the ones the tail is built from.
func (o BlueprintOptions) Expand() ExpandOptions {
	return ExpandOptions{MaxAttempts: o.MaxAttempts, UploadGate: o.UploadGate}
}

// TaskRunner adapts the use cases below to the scheduler's Runner port. It is
// the one stateful type here, because implementing an interface needs a
// receiver, and it holds no logic: every branch forwards to a use case.
type TaskRunner struct {
	videos        repository.VideoReader
	videoFields   repository.VideoFieldWriter
	channels      repository.ChannelReader
	chapters      repository.ChapterReader
	chapterWriter repository.ChapterWriter
	chapterFields repository.ChapterFieldWriter
	assets        repository.AssetWriter
	store         provider.AssetStore
	llm           provider.LLM
	tts           provider.TTS
	slides        provider.SlideGenerator
	composer      provider.VideoComposer
	thumbnails    provider.ThumbnailRenderer
	icons         provider.IconGenerator
	uploader      provider.Uploader
	notifier      Notifier
	expander      GraphExpander
	blueprintOpts func() BlueprintOptions
	narrationOpts func() NarrationOptions
	iconOpts      func() IconOptions
	dryRun        func() bool
	now           func() time.Time
	log           *slog.Logger
}

var _ scheduler.Runner = (*TaskRunner)(nil)

// NewTaskRunner wires the runner.
//
//nolint:revive // the parameter list is the dependency list, which is the point
func NewTaskRunner(
	videos repository.VideoReader,
	videoFields repository.VideoFieldWriter,
	channels repository.ChannelReader,
	chapters repository.ChapterReader,
	chapterWriter repository.ChapterWriter,
	chapterFields repository.ChapterFieldWriter,
	assets repository.AssetWriter,
	store provider.AssetStore,
	llm provider.LLM,
	tts provider.TTS,
	slides provider.SlideGenerator,
	composer provider.VideoComposer,
	thumbnails provider.ThumbnailRenderer,
	icons provider.IconGenerator,
	uploader provider.Uploader,
	notifier Notifier,
	expander GraphExpander,
	blueprintOpts func() BlueprintOptions,
	narrationOpts func() NarrationOptions,
	iconOpts func() IconOptions,
	dryRun func() bool,
	now func() time.Time,
	log *slog.Logger,
) *TaskRunner {
	if now == nil {
		now = time.Now
	}
	return &TaskRunner{
		videos: videos, videoFields: videoFields, channels: channels,
		chapters: chapters, chapterWriter: chapterWriter, chapterFields: chapterFields,
		assets: assets, store: store, llm: llm, tts: tts, slides: slides,
		composer: composer, thumbnails: thumbnails, icons: icons,
		uploader: uploader, notifier: notifier,
		expander: expander, blueprintOpts: blueprintOpts,
		narrationOpts: narrationOpts, iconOpts: iconOpts,
		dryRun: dryRun, now: now, log: log,
	}
}

// Run executes exactly one task.
func (r *TaskRunner) Run(ctx context.Context, t entity.Task) entity.TaskOutcome {
	log := r.log.With(
		slog.String("video_id", t.VideoID.String()),
		slog.String("task", t.ID.String()),
		slog.String("kind", string(t.Kind)),
		slog.Int("ordinal", t.Ordinal),
		slog.Int("attempt", t.Attempt))
	start := r.now()

	outcome := r.dispatch(ctx, t)

	switch o := outcome.(type) {
	case entity.Success:
		log.Debug("task succeeded",
			slog.Duration("took", r.now().Sub(start)),
			slog.Int("assets", len(o.Assets)))
	case entity.AwaitingApproval:
		log.Info("task produced a gate", slog.String("gate", string(o.Gate)))
	case entity.Failed:
		log.Warn("task failed",
			slog.Duration("took", r.now().Sub(start)),
			slog.Bool("retryable", o.Retryable),
			slog.String("error", errText(o.Err)))
	default:
		panic(fmt.Sprintf("unhandled outcome %T", o))
	}
	return outcome
}

func (r *TaskRunner) dispatch(ctx context.Context, t entity.Task) entity.TaskOutcome {
	switch t.Kind {
	case entity.TaskKindBlueprint:
		return r.runBlueprint(ctx, t)
	case entity.TaskKindPrimeSlidePrompts:
		return PrimeSlidePrompts(ctx, t, r.llm)
	case entity.TaskKindSlidePrompts:
		return ResolveSlidePrompts(ctx, t, r.llm, r.chapters, r.chapterFields)
	case entity.TaskKindScript:
		return GenerateScript(ctx, t, r.videos, r.chapters, r.llm,
			r.chapterFields, r.assets, r.store, r.notifier, r.now())
	case entity.TaskKindTTS:
		return SynthesizeNarration(ctx, t, r.videos, r.chapters, r.tts,
			r.chapterFields, r.assets, r.store, r.notifier, r.narrationOpts(), r.now())
	case entity.TaskKindSlide:
		return GenerateSlide(ctx, t, r.videos, r.chapters, r.slides,
			r.chapterFields, r.assets, r.store, r.notifier, r.now())
	case entity.TaskKindClip:
		return ComposeChapterClip(ctx, t, r.videos, r.chapters, r.composer, r.chapterFields,
			r.assets, r.store, r.notifier, r.now())
	case entity.TaskKindConcat:
		return ComposeFinalVideo(ctx, t, r.chapters, r.composer, r.videoFields,
			r.assets, r.store, r.reporter(t), r.now())
	case entity.TaskKindMetadata:
		return GenerateMetadata(ctx, t, r.videos, r.chapters, r.llm,
			r.videoFields, r.assets, r.store, r.now())
	case entity.TaskKindThumbnailPlan:
		return GenerateThumbnailPlan(ctx, t, r.videos, r.chapters, r.llm,
			r.videoFields, r.assets, r.store, r.now())
	case entity.TaskKindThumbnailIcon:
		return GenerateThumbnailIcon(ctx, t, r.videos, r.icons,
			r.videoFields, r.assets, r.store, r.iconOpts(), r.now())
	case entity.TaskKindThumbnail:
		return BuildThumbnail(ctx, t, r.videos, r.thumbnails,
			r.videoFields, r.assets, r.store, r.now())
	case entity.TaskKindUpload:
		return PublishVideo(ctx, t, r.videos, r.channels, r.uploader, r.videoFields,
			r.dryRun, r.reporter(t))
	default:
		return entity.Failed{Err: fmt.Errorf("unhandled task kind %q", t.Kind), Retryable: false}
	}
}

// progressStep is how far a long task must advance before it reports again.
//
// ffmpeg emits a whole percent roughly once a second, and every report is an SSE
// frame that also takes a slot in the replay buffer a reconnecting client
// resumes from. Five points turns a three-minute render into twenty messages
// rather than a hundred and eighty — the difference between progress costing
// nothing and progress crowding out the deltas that actually have to arrive.
const progressStep = 5

// reporter returns the callback a long-running provider call reports through,
// or nil when there is no one to report to.
//
// The delta is built from the task rather than assembled by hand. A TaskDelta is
// merged into the client's cache field by field, so a partial one would
// overwrite the kind and state of the very task it claims to describe with zero
// values — and the snapshot handed to Run is taken after the scheduler has
// marked the task running, so `t.Delta()` is already the truth.
//
// Not safe for concurrent callers, and does not need to be: a provider reports
// from one place at a time, and every call happens before the composition
// returns.
func (r *TaskRunner) reporter(t entity.Task) func(int) {
	if r.notifier == nil {
		return nil
	}
	last := -progressStep
	return func(pct int) {
		// 100 always goes out: it is the frame that says the wait is over, and
		// rounding it away would leave the last bar short of the end.
		if pct < last+progressStep && pct < 100 {
			return
		}
		last = pct
		d := t.Delta()
		d.Percent = pct
		r.notifier.NotifyTask(d)
	}
}

// runBlueprint writes the outline and, with no gate to park on, expands the DAG
// before reporting success — chapter branches are created when the outline is
// accepted, and with no operator to ask, acceptance is the task succeeding.
// Expansion goes last, so anything that fails before it leaves a one-node DAG
// whose blueprint can simply be run again.
func (r *TaskRunner) runBlueprint(ctx context.Context, t entity.Task) entity.TaskOutcome {
	opts := r.blueprintOpts()
	outcome := GenerateBlueprint(ctx, t, r.videos, r.channels, r.llm, r.chapterWriter,
		r.videoFields, r.assets, r.store, r.notifier, opts.ChapterTolerancePercent, r.now())
	if _, ok := outcome.(entity.Success); !ok {
		return outcome
	}
	if t.Gate != entity.GateNone {
		return outcome
	}
	if err := ExpandVideoGraph(ctx, r.videos, r.chapters, r.expander, r.now(), opts.Expand(), t.VideoID); err != nil {
		return classify(err)
	}
	return outcome
}

func errText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
