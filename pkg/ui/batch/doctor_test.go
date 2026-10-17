package batch

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDoctorManualRepairsNeverRenderAsSuccess(t *testing.T) {
	output := captureUI(t)
	r := New(0, false)
	for i, status := range []string{"manual", "verification-failed", "verification-incomplete", "findings"} {
		r.Update(&Event{ID: i, Label: "repair", Diagnostic: true, Done: true, Outcome: status})
	}
	require.NotContains(t, output.String(), "✓")
	require.Contains(t, output.String(), "verification-incomplete")
}

func TestDoctorUnknownTotalsDoNotShowProgressBar(t *testing.T) {
	output := captureUI(t)
	r := New(0, true)
	r.size = func() (int, int, error) { return 120, 24, nil }
	r.Update(&Event{ID: 1, Label: "Project inputs", Phase: "Discover", Diagnostic: true})
	r.Tick()
	r.Clear()
	require.Contains(t, output.String(), "Discover")
	require.NotContains(t, output.String(), "/0 complete")
}

func TestDoctorUntotaledEventKeepsKnownPhaseTotal(t *testing.T) {
	captureUI(t)
	r := New(0, false)
	r.Update(&Event{Reset: true, Count: 3})
	r.Update(&Event{ID: 1, Label: "first", Phase: "Verify", Diagnostic: true, Count: 3})
	r.Update(&Event{ID: 2, Label: "second", Phase: "Verify", Diagnostic: true, Count: -1})
	require.Equal(t, 3, r.total)

	fresh := New(0, false)
	fresh.Update(&Event{ID: 1, Label: "first", Phase: "Check", Diagnostic: true, Count: -1})
	require.Zero(t, fresh.total)
}

func TestDoctorWarningEventSeverityPicksIcon(t *testing.T) {
	for _, tt := range []struct {
		name, severity, want string
	}{
		{"error severity renders as error", "error", "✗ finding text\n"},
		{"warning severity renders as warning", "warning", "⚠ finding text\n"},
		{"unset severity renders as warning", "", "⚠ finding text\n"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			output := captureUI(t)
			r := New(0, false)
			r.Update(&Event{Warning: "finding text", Severity: tt.severity})
			require.Equal(t, tt.want, output.String())
		})
	}
}

func TestDoctorFailedOutcomeRendersAsFailure(t *testing.T) {
	output := captureUI(t)
	r := New(0, false)
	r.Update(&Event{ID: 1, Label: "stack validation", Diagnostic: true, Done: true, Outcome: "failed"})
	require.Equal(t, "✗ stack validation (failed)\n", output.String())
}
