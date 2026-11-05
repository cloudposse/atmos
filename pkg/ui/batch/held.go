package batch

import (
	"context"
	"errors"
	"sort"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ui"
)

// WithHeldResults makes the batch transactional for presentation. Completed jobs are not
// reported while work is running, because a later failure can still invalidate them. When the
// batch finishes, a successful run prints one semantic status line per job in job (ID) order.
// A failed run prints only the failed jobs, each as an error line followed by its hints, and
// then one warning with failureNotice (when non-empty) instead of claiming that anything was
// committed. Cancellation is reported by the notice alone, never once per job.
func WithHeldResults(failureNotice string) Option {
	return func(r *Renderer) {
		r.hold = true
		r.holdNotice = failureNotice
	}
}

// holdResult remembers a terminal event for the batch outcome without printing it.
func (r *Renderer) holdResult(e *Event) {
	r.held = append(r.held, *e)
	r.Complete(e.ID, func() {})
}

// flushHeld reports held results after the batch outcome is known.
func (r *Renderer) flushHeld(err error) {
	if !r.hold {
		return
	}
	held := r.held
	r.held = nil
	sort.SliceStable(held, func(a, b int) bool { return held[a].ID < held[b].ID })
	failed := err != nil
	for index := range held {
		failed = failed || held[index].Err != nil
	}
	if !failed {
		for index := range held {
			held[index].printStatus()
		}
		return
	}
	for index := range held {
		held[index].printFailure()
	}
	if r.holdNotice != "" {
		ui.Warning(r.holdNotice)
	}
}

// printStatus renders a successful job with the semantic status functions.
func (e *Event) printStatus() {
	text := e.Label
	if e.Version != "" {
		text += " (" + e.Version + ")"
	}
	switch {
	case e.Outcome == "unchanged", e.Outcome == "skipped", e.Outcome == "canceled":
		ui.Info(text + " (" + e.Outcome + ")")
	case e.Outcome != "" && e.Outcome != "installed":
		ui.Success(text + " (" + e.Outcome + ")")
	default:
		ui.Success(text)
	}
}

// printFailure renders a failed job and its hints. Cancellations are summarized by the caller.
func (e *Event) printFailure() {
	if e.Err == nil || errors.Is(e.Err, context.Canceled) {
		return
	}
	text := e.Label
	if e.Version != "" {
		text += " (" + e.Version + ")"
	}
	ui.Error(text + ": " + e.Err.Error())
	for _, detail := range errUtils.AllDetails(e.Err) {
		ui.Writeln("  " + ui.FormatInline(detail))
	}
	for _, hint := range errUtils.AllHints(e.Err) {
		ui.Hint(hint)
	}
}
