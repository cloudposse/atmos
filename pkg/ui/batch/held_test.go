package batch

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ui/theme"
)

const heldNotice = "No plugin changes were written."

func runHeld(t *testing.T, events []Event, finishWith error) string {
	t.Helper()
	output := captureUI(t)
	renderer := New(len(events), false, WithHeldResults(heldNotice))
	for index := range events {
		renderer.Update(&events[index])
	}
	renderer.Finish(finishWith)
	return ansi.Strip(output.String())
}

func TestHeldResultsAreSilentUntilFinishThenUseSemanticStatusLines(t *testing.T) {
	output := captureUI(t)
	renderer := New(3, false, WithHeldResults(heldNotice))
	// Completion order differs from job order; output must follow job order.
	renderer.Update(&Event{ID: 2, Label: "zeta", Version: "3.0.0", Done: true, Outcome: "installed"})
	renderer.Update(&Event{ID: 0, Label: "alpha", Version: "1.0.0", Done: true, Outcome: "installed"})
	renderer.Update(&Event{ID: 1, Label: "bravo", Version: "2.0.0", Done: true, Outcome: "skipped"})
	require.Empty(t, output.String(), "results are held until the batch outcome is known")
	renderer.Finish(nil)
	lines := strings.Split(strings.TrimSpace(ansi.Strip(output.String())), "\n")
	require.Len(t, lines, 3)
	assert.Equal(t, "✓ alpha (1.0.0)", lines[0])
	assert.Equal(t, theme.IconInfo+" bravo (2.0.0) (skipped)", lines[1])
	assert.Equal(t, "✓ zeta (3.0.0)", lines[2])
	assert.NotContains(t, output.String(), heldNotice)
}

func TestHeldResultsOfFailedBatchReportEveryFailureInOrderAndNeverClaimSuccess(t *testing.T) {
	hinted := errUtils.Build(errors.New("no release")).WithExplanation("Constraint was ^9.").WithHint("Run atmos plugin search.").Err()
	text := runHeld(t, []Event{
		{ID: 2, Label: "charlie", Done: true, Err: errors.New("registry unreachable")},
		{ID: 1, Label: "bravo", Version: "2.0.0", Done: true, Outcome: "installed"},
		{ID: 0, Label: "alpha", Done: true, Err: hinted},
	}, errors.Join(hinted, errors.New("registry unreachable")))
	lines := strings.Split(strings.TrimSpace(text), "\n")
	require.Len(t, lines, 5, text)
	assert.Equal(t, "✗ alpha: no release", lines[0])
	assert.Equal(t, "  Constraint was ^9.", lines[1], "the explanation follows the failure")
	assert.Contains(t, lines[2], "Run atmos plugin search.")
	assert.Equal(t, "✗ charlie: registry unreachable", lines[3])
	assert.Equal(t, "⚠ "+heldNotice, lines[4])
	assert.NotContains(t, text, "bravo", "a job that finished before the failure was not committed")
}

func TestHeldResultsCancellationPrintsNoticeWithoutPerJobCancelLines(t *testing.T) {
	text := runHeld(t, []Event{
		{ID: 0, Label: "alpha", Done: true, Err: context.Canceled},
		{ID: 1, Label: "bravo", Done: true, Outcome: "installed"},
	}, context.Canceled)
	assert.Equal(t, "⚠ "+heldNotice, strings.TrimSpace(text))
}

func TestHeldResultsDefaultRendererIsUnchanged(t *testing.T) {
	output := captureUI(t)
	renderer := New(1, false)
	renderer.Update(&Event{ID: 0, Label: "alpha", Version: "1.0.0", Done: true, Outcome: "installed"})
	assert.Contains(t, ansi.Strip(output.String()), "alpha (1.0.0)", "other batch consumers keep immediate results")
	renderer.Finish(errors.New("later failure"))
	assert.NotContains(t, output.String(), heldNotice)
}
