package batch

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/charmbracelet/x/ansi"

	"github.com/cloudposse/atmos/pkg/ui"
	"github.com/cloudposse/atmos/pkg/ui/theme"
)

// WithPhaseSummaries retains one summary per diagnostic phase instead of a row
// for every job. Live jobs remain visible; other batch consumers are unaffected.
func WithPhaseSummaries() Option {
	return func(r *Renderer) { r.phaseSummaries = true }
}

// Finish clears active progress and flushes the final phase, including partial
// work on cancellation or error. Run calls it after draining all events.
func (r *Renderer) Finish(err error) {
	r.Clear()
	r.flushPhase(err)
}

func (r *Renderer) flushPhase(err error) {
	if !r.phaseSummaries || r.phase == "" {
		return
	}
	r.Clear()
	styles := theme.GetCurrentStyles()
	style := styles.Body
	parts := []string{fmt.Sprintf("%d completed", r.completed)}
	problems := r.problemOutcomes()
	parts = append(parts, problems...)
	if len(problems) > 0 {
		style = styles.Warning
	}
	if r.outcomes["failed"] > 0 || r.outcomes["verification-failed"] > 0 {
		style = styles.Error
	}
	switch {
	case errors.Is(err, context.Canceled):
		parts = append(parts, "canceled")
		style = styles.Warning
	case err != nil:
		parts = append(parts, "failed")
		style = styles.Error
	case len(r.active) > 0 || r.total > r.completed:
		parts = append(parts, "incomplete")
		style = styles.Warning
	}
	width, _ := r.dimensions()
	ui.Writeln(ansi.Wrap(styles.Heading.Render(r.phase)+" · "+style.Render(strings.Join(parts, " · ")), max(1, width), ""))
	r.phase = ""
	r.outcomes = nil
}

// Completed jobs do not imply successful validation. Retain explicit outcomes.
func (r *Renderer) problemOutcomes() []string {
	keys := make([]string, 0, len(r.outcomes))
	for outcome := range r.outcomes {
		switch outcome {
		case "passed", "checked", "completed", "fixed", "":
			continue
		}
		keys = append(keys, outcome)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, outcome := range keys {
		label := strings.ReplaceAll(outcome, "-", " ")
		if outcome == "findings" {
			label = "with findings"
		}
		parts = append(parts, fmt.Sprintf("%d %s", r.outcomes[outcome], label))
	}
	return parts
}
