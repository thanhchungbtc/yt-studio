package app

import (
	"fmt"
	"time"

	"github.com/tbui/yt-studio/domain/entity"
	"github.com/tbui/yt-studio/domain/provider"
)

// The console's account of a task, which is every task rather than only the
// five that talk to a language model.
//
// The console began as a window onto the models, because a blueprint takes
// minutes and says nothing at all while it does. That is equally true of a
// four-minute concat, of a chapter's narration, and of eighty slides going
// through an image gateway one at a time — the other eight kinds produce no
// token anybody could watch, and were therefore invisible in the one place the
// machine narrates itself. So every task opens a block when it starts and
// closes it when it ends, and the block says the two things a progress dot
// cannot: how long the work actually took, and what went wrong.
//
// This is a report, not a mechanism. It hangs off TaskRunner.Run, the single
// place all thirteen kinds pass through, so no use case and no adapter knows
// the console exists — the same arrangement the LLM frames have, for the same
// reason: nothing watching may slow down what it is watching.
//
// The five LLM-backed kinds therefore produce two blocks, the envelope and the
// exchange inside it, and that is deliberate rather than tolerated. The
// envelope times the whole task — prompt, call, validation, write — so a script
// that spent forty seconds in the model and four minutes storing an asset is a
// fact only the outer block can state. The two are told apart by the second
// column, which an envelope fills with `task` where an exchange names the model
// it went to.

// taskSource is what an envelope puts in the frame's Model field.
//
// The field means "which model produced this", and a task did not go to one, so
// it says what did produce it instead. Blank would read as a model whose name
// failed to arrive, and this column is the only thing distinguishing an
// envelope from the exchange nested inside it.
const taskSource = "task"

// taskWatcher reports one task's run to the console.
//
// A nil watcher is the no-observer case and every method tolerates one, so Run
// never has to ask whether anybody is watching.
type taskWatcher struct {
	observe provider.LLMObserver
	// frame is the run's identity, copied and completed to close it.
	frame provider.LLMFrame
}

// watchTask opens a task's block, and returns nil when nothing is watching.
func (r *TaskRunner) watchTask(t entity.Task, started time.Time) *taskWatcher {
	if r.console == nil {
		return nil
	}
	w := &taskWatcher{
		observe: r.console,
		frame: provider.LLMFrame{
			Run:       provider.NextRun(),
			Video:     t.VideoID,
			Label:     taskLabel(t),
			Model:     taskSource,
			StartedAt: started,
		},
	}
	// Announced before the work begins rather than after it ends, because the
	// wait is the whole point: an envelope that appeared on completion would be
	// a log of what has already stopped mattering.
	w.observe(w.frame)
	return w
}

// close reports how the task ended. Exactly one of these follows every
// watchTask, whatever the outcome — an envelope left open shows a caret
// forever, which the console means as "still running".
func (w *taskWatcher) close(outcome entity.TaskOutcome) {
	if w == nil {
		return
	}
	f := w.frame
	f.Done = true
	switch o := outcome.(type) {
	case entity.Success:
		f.Text = successText(len(o.Assets))
	case entity.AwaitingApproval:
		f.Text = fmt.Sprintf("succeeded, holding the %s gate", o.Gate)
	case entity.Failed:
		// No text, because the error is a field of its own and the console
		// draws it in the colour a failure is drawn in everywhere else.
		// Retryable is deliberately not reported: whether another attempt
		// happens depends on the budget the scheduler is counting, and a line
		// here reading "retrying" would be a guess printed as a fact.
		f.Err = o.Err
	}
	w.observe(f)
}

// successText says what a task produced, when it produced anything.
//
// Spelled out only past one asset. Most kinds write exactly one and the count
// there would be noise next to the duration; the batches are the case worth
// reading, and a bare `succeeded` is the whole report for everything else.
func successText(assets int) string {
	switch assets {
	case 0:
		return "succeeded"
	case 1:
		return "succeeded, 1 asset"
	default:
		return fmt.Sprintf("succeeded, %d assets", assets)
	}
}

// taskLabel names a task the way an exchange and a transcript file are already
// named — kind, then chapter, then slot.
//
// The same components as the task id, in the same order, minus the video, and
// with `ch` in front of the ordinal: the console shows every video at once, and
// `script-12` sitting beside `slide-3-2` gives no clue which number means what.
// A `script-ch12` envelope then lines up by eye with the `script-ch12` exchange
// inside it, which is the only reason the adapters' spelling is worth copying.
func taskLabel(t entity.Task) string {
	label := string(t.Kind)
	if t.Ordinal >= 0 {
		label = fmt.Sprintf("%s-ch%d", label, t.Ordinal)
	}
	if t.Index >= 0 {
		label = fmt.Sprintf("%s-%d", label, t.Index)
	}
	return label
}
