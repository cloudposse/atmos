package step

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/castplan"
)

// cleanupRemoving declares a cleanup command that deletes target and exits 1, as the fake atmos does.
func cleanupRemoving(t *testing.T, target string) castplan.Action {
	t.Helper()
	action := fakeAtmosRun(t, "rm-glob-and-fail", map[string]string{atmosStepFakeRMGlobEnv: target})
	action.ExitCode = 1
	return action
}

func TestGenerateCastExecutesPausesAndRegisteredSteps(t *testing.T) {
	g := newDeclaredGeneration(
		t,
		simulateAction("# before"),
		castplan.Action{Kind: "pause", Fields: map[string]any{"duration": "1ms"}},
		castplan.Action{Kind: "step", Fields: map[string]any{"type": "sleep", "duration": "1ms"}},
		simulateAction("# after"),
	)
	_, err := g.generate(t.Context())
	require.NoError(t, err)
	text := g.recordedText(t)
	require.Contains(t, text, "# before")
	require.Contains(t, text, "# after")
}

func TestGenerateCastRejectsInvalidPauseDuration(t *testing.T) {
	g := newDeclaredGeneration(t, castplan.Action{Kind: "pause", Fields: map[string]any{"duration": "soon"}})
	_, err := g.generate(t.Context())
	require.ErrorContains(t, err, "cast action 1 (pause)")
	require.NoFileExists(t, g.castPath())
}

func TestGenerateCastHiddenActionsNeverAppearInTheRecording(t *testing.T) {
	g := newDeclaredGeneration(
		t,
		simulateAction("# visible narration"),
		castplan.Action{Kind: "hidden", Children: []castplan.Action{
			simulateAction("# HIDDEN-SENTINEL"),
			{Kind: "pause", Fields: map[string]any{"duration": "1ms"}},
			{Kind: "step", Fields: map[string]any{"type": "sleep", "duration": "1ms"}},
		}},
	)
	_, err := g.generate(t.Context())
	require.NoError(t, err)
	text := g.recordedText(t)
	require.Contains(t, text, "# visible narration")
	require.NotContains(t, text, "HIDDEN-SENTINEL")
}

func TestGenerateCastSetupAndCleanupDoNotTypeOrRecord(t *testing.T) {
	g := newDeclaredGeneration(t, simulateAction("# recorded"))
	g.plan.Setup = []castplan.Action{simulateAction("# SETUP-SENTINEL")}
	g.plan.Cleanup = []castplan.Action{simulateAction("# CLEANUP-SENTINEL")}
	_, err := g.generate(t.Context())
	require.NoError(t, err)
	text := g.recordedText(t)
	require.Contains(t, text, "# recorded")
	require.NotContains(t, text, "SETUP-SENTINEL")
	require.NotContains(t, text, "CLEANUP-SENTINEL")
}

func TestGenerateCastCommandFailuresSurfaceBeforePublishing(t *testing.T) {
	tests := []struct {
		name     string
		action   func(*testing.T) castplan.Action
		contains string
		// assertion marks failures reported as declaration assertions rather than step errors.
		assertion bool
	}{
		{
			name: "command that cannot be rendered",
			action: func(*testing.T) castplan.Action {
				return castplan.Action{Kind: "run", Fields: map[string]any{"command": "{{ .nope", "interval": "0s"}}
			},
			contains: "failed to parse template",
		},
		{
			name: "command that renders only when executed",
			action: func(*testing.T) castplan.Action {
				// The displayed text types fine; the real command fails before producing a result.
				return castplan.Action{Kind: "run", Fields: map[string]any{"command": "{{ .nope", "display": "atmos plan", "interval": "0s"}}
			},
			contains: "failed to parse template",
		},
		{
			name:      "unexpected exit status",
			action:    func(t *testing.T) castplan.Action { return fakeAtmosRun(t, "fail", nil) },
			contains:  "exit status 3, expected 0",
			assertion: true,
		},
		{
			name: "expected output is missing",
			action: func(t *testing.T) castplan.Action {
				action := fakeAtmosRun(t, "ok", nil)
				action.Expect = []castplan.Matcher{{Text: "not-in-output"}}
				return action
			},
			contains:  `missing "not-in-output"`,
			assertion: true,
		},
		{
			name: "rejected output is present",
			action: func(t *testing.T) castplan.Action {
				action := fakeAtmosRun(t, "ok", nil)
				action.Reject = []castplan.Matcher{{Text: "fake-atmos-[a-z]+", Regex: true}}
				return action
			},
			contains:  `rejected "fake-atmos-[a-z]+"`,
			assertion: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := newDeclaredGeneration(t, tt.action(t))
			require.NoError(t, os.WriteFile(g.castPath(), []byte("previous recording"), 0o600))
			_, err := g.generate(t.Context())
			if tt.assertion {
				require.ErrorIs(t, err, errUtils.ErrAutomation)
			}
			require.ErrorContains(t, err, tt.contains)
			content, readErr := os.ReadFile(g.castPath())
			require.NoError(t, readErr)
			require.Equal(t, "previous recording", string(content))
		})
	}
}

func TestGenerateCastSessionOptionErrorsRunCleanup(t *testing.T) {
	tests := []struct {
		name     string
		fields   map[string]any
		contains string
	}{
		{name: "invalid write rate", fields: map[string]any{"write_rate": "soon"}, contains: `invalid duration "soon"`},
		{name: "invalid key interval", fields: map[string]any{"key_interval": "soon"}, contains: `invalid duration "soon"`},
		{name: "unresolvable env template", fields: map[string]any{"env": map[string]any{"BROKEN": "{{ .nope"}}, contains: "failed to resolve env var BROKEN"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := newDeclaredGeneration(t, castplan.Action{
				Kind:     "session",
				Fields:   tt.fields,
				Children: []castplan.Action{{Kind: "hide"}},
			})
			target := filepath.Join(g.dir, "fixture")
			require.NoError(t, os.WriteFile(target, []byte("fixture"), 0o600))
			g.plan.Cleanup = []castplan.Action{cleanupRemoving(t, target)}
			_, err := g.generate(t.Context())
			require.ErrorContains(t, err, tt.contains)
			require.NoFileExists(t, target, "cleanup must run after a failed session")
			require.NoFileExists(t, g.castPath())
		})
	}
}

func TestGenerateCastDeadlineStopsRunningCommandAndRunsCleanup(t *testing.T) {
	g := newDeclaredGeneration(t, fakeAtmosRun(t, "sleep", map[string]string{atmosStepFakeSleepMSEnv: "30000"}))
	target := filepath.Join(g.dir, "fixture")
	require.NoError(t, os.WriteFile(target, []byte("fixture"), 0o600))
	g.plan.Cleanup = []castplan.Action{cleanupRemoving(t, target)}
	ctx, cancel := context.WithTimeout(t.Context(), 500*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, err := g.generate(ctx)
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.Less(t, time.Since(start), 20*time.Second, "the running command must stop at the deadline")
	require.NoFileExists(t, target, "cleanup must run even though the run context expired")
	require.NoFileExists(t, g.castPath())
}

func TestGenerateCastCancelledBeforeStartStillRunsCleanup(t *testing.T) {
	g := newDeclaredGeneration(t, simulateAction("# never recorded"))
	target := filepath.Join(g.dir, "fixture")
	require.NoError(t, os.WriteFile(target, []byte("fixture"), 0o600))
	g.plan.Setup = []castplan.Action{simulateAction("# setup")}
	g.plan.Cleanup = []castplan.Action{cleanupRemoving(t, target)}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, err := g.generate(ctx)
	require.ErrorIs(t, err, context.Canceled)
	require.NoFileExists(t, target, "cleanup must run for a run that was cancelled before it started")
	require.NoFileExists(t, g.castPath())
}
