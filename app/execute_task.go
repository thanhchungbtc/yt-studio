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

// BlueprintOptions are the settings-sourced inputs of the blueprint task: the
// tolerance its chapter count is judged against, and the shape of the DAG that
// count expands into when no gate stands in the way.
type BlueprintOptions struct {
	ChapterTolerancePercent int
	MaxAttempts             int
	UploadGate              bool
}

// Expand narrows the options to the ones the tail is built from.
func (o BlueprintOptions) Expand() ExpandOptions {
	return ExpandOptions{MaxAttempts: o.MaxAttempts, UploadGate: o.UploadGate}
}

// TaskRunner adapts the use cases below to the scheduler's Runner port.
//
// It is the one place in this package that holds state, because implementing an
// interface requires a receiver. It contains no logic of its own: every branch
// forwards to an exported use case whose signature names exactly the narrow
// dependencies that branch touches.
type TaskRunner struct {
	videos        repository.VideoReader
	videoFields   repository.VideoFieldWriter
	channels      repository.ChannelReader
	chapters      repository.ChapterReader
	chapterWriter repository.ChapterWriter
	chapterFields repository.ChapterFieldWriter
	assets        repository.AssetWriter
	store         provider.AssetStore
	llm           provider.LLMProvider
	tts           provider.TTSProvider
	images        provider.ImageProvider
	composer      provider.VideoComposer
	uploader      provider.Uploader
	notifier      ChapterNotifier
	expander      GraphExpander
	blueprintOpts func() BlueprintOptions
	dryRun        func() bool
	now           func() time.Time
	log           *slog.Logger
}

var _ scheduler.Runner = (*TaskRunner)(nil)

// NewTaskRunner wires the runner. Every collaborator is an explicit parameter.
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
	llm provider.LLMProvider,
	tts provider.TTSProvider,
	images provider.ImageProvider,
	composer provider.VideoComposer,
	uploader provider.Uploader,
	notifier ChapterNotifier,
	expander GraphExpander,
	blueprintOpts func() BlueprintOptions,
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
		assets: assets, store: store, llm: llm, tts: tts, images: images,
		composer: composer, uploader: uploader, notifier: notifier,
		expander: expander, blueprintOpts: blueprintOpts,
		dryRun: dryRun, now: now, log: log,
	}
}

// Run executes exactly one task. The switch is exhaustive over TaskKind and
// ends in a panic, so adding a kind without handling it fails loudly rather
// than silently succeeding.
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
	case entity.TaskKindPrimeImagePrompts:
		return PrimeImagePrompts(ctx, t, r.llm)
	case entity.TaskKindImagePrompts:
		return ResolveImagePrompts(ctx, t, r.llm, r.chapters, r.chapterFields)
	case entity.TaskKindScript:
		return GenerateScript(ctx, t, r.videos, r.channels, r.chapters, r.llm,
			r.chapterFields, r.assets, r.store, r.notifier, r.now())
	case entity.TaskKindTTS:
		return SynthesizeNarration(ctx, t, r.videos, r.channels, r.chapters, r.tts,
			r.chapterFields, r.assets, r.store, r.notifier, r.now())
	case entity.TaskKindImage:
		return GenerateStill(ctx, t, r.videos, r.channels, r.chapters, r.images,
			r.chapterFields, r.assets, r.store, r.notifier, r.now())
	case entity.TaskKindClip:
		return ComposeChapterClip(ctx, t, r.videos, r.chapters, r.composer, r.chapterFields,
			r.assets, r.store, r.notifier, r.now())
	case entity.TaskKindConcat:
		return ComposeFinalVideo(ctx, t, r.chapters, r.composer, r.videoFields,
			r.assets, r.store, r.now())
	case entity.TaskKindMetadata:
		return GenerateMetadata(ctx, t, r.videos, r.channels, r.chapters, r.llm,
			r.videoFields, r.assets, r.store, r.now())
	case entity.TaskKindUpload:
		return PublishVideo(ctx, t, r.videos, r.channels, r.uploader, r.videoFields, r.dryRun)
	default:
		return entity.Failed{Err: fmt.Errorf("unhandled task kind %q", t.Kind), Retryable: false}
	}
}

// runBlueprint writes the outline and, when no gate will park the pipeline,
// builds the DAG that outline shapes before reporting success.
//
// The two branches are the same rule seen from either side: a video's chapter
// branches are created when its outline is accepted. With the gate on that is
// the operator's approval, and ApproveGate expands. With the gate off there is
// nobody to ask, so acceptance is the task succeeding and the expansion happens
// here — before the outcome is reported, so the blueprint never releases
// dependents that do not exist yet.
//
// Expansion is deliberately the last thing to happen. Anything that fails
// before it leaves a video whose DAG is still a single node and whose blueprint
// can simply be run again.
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
