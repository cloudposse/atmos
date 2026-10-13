package doctor

import (
	"os"
	"path/filepath"
	"time"

	diagnostic "github.com/cloudposse/atmos/pkg/doctor"
	"github.com/cloudposse/atmos/pkg/ui/batch"
	"github.com/cloudposse/atmos/pkg/validation"
)

const (
	phaseCheck = "Check"
	// Row outcomes understood by the batch renderer.
	outcomeFindings = "findings"
	outcomeFailed   = "failed"
	// The checked outcome marks a file with no findings known when its row completes.
	// Project-wide checks report later, so "passed" would overstate what was verified.
	outcomeChecked = "checked"
)

// progressObserver translates engine events into progress rows.
//
// Findings are deliberately not streamed: the final report lists every finding once,
// in deterministic order and with its reason and remediation. Streaming them as well
// printed each finding twice. Per-file rows instead carry an outcome derived from the
// findings already known for that file, so a malformed file never reads as passed.
func progressObserver(observe batch.Observer) func(diagnostic.Event) {
	phase := ""
	ids := map[string]int{}
	nextID := 0
	started := time.Now()
	worst := map[string]validation.Severity{}
	return func(event diagnostic.Event) {
		if event.Finding != nil {
			recordFinding(worst, event.Finding)
			return
		}
		if phase != event.Phase {
			phase = event.Phase
			observe(batch.Event{Reset: true, Count: event.Total})
		}
		if event.Status == "warning" {
			observe(batch.Event{Warning: event.Label})
			return
		}
		key := event.Phase + "/" + event.Label
		id, ok := ids[key]
		if !ok {
			nextID++
			id = nextID
			ids[key] = id
		}
		count := event.Total
		if count == 0 {
			count = -1
		}
		outcome := event.Status
		if event.Done && event.Phase == phaseCheck {
			outcome = fileOutcome(worst, &event)
		}
		observe(batch.Event{Count: count, ID: id, Label: event.Label, Phase: event.Phase, Done: event.Done, Outcome: outcome, Elapsed: time.Since(started), Diagnostic: true})
	}
}

// recordFinding remembers the most severe finding seen for each file, keyed by project-relative path.
func recordFinding(worst map[string]validation.Severity, f *diagnostic.Finding) {
	if f.File == "" {
		return
	}
	key := progressPath(f.File)
	if _, seen := worst[key]; !seen || f.Severity == validation.SeverityError {
		worst[key] = f.Severity
	}
}

// Per-file row outcomes derive from the findings known when the row completes.
// Aggregate rows such as static stacks and GitHub Actions keep the engine's own status.
func fileOutcome(worst map[string]validation.Severity, event *diagnostic.Event) string {
	severity, seen := worst[progressPath(event.Label)]
	switch {
	case seen && severity == validation.SeverityError:
		return outcomeFailed
	case seen:
		return outcomeFindings
	case event.Index > 0 && event.Status == "passed":
		return outcomeChecked
	default:
		return event.Status
	}
}

// progressPath normalizes engine paths to slash-separated, working-directory-relative form.
func progressPath(path string) string {
	if root, err := os.Getwd(); err == nil && filepath.IsAbs(path) {
		if rel, err := filepath.Rel(root, path); err == nil {
			path = rel
		}
	}
	return filepath.ToSlash(path)
}
