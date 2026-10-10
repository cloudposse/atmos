package step

import (
	"context"
	"os"
	"testing"
	"time"

	cockroachErrors "github.com/cockroachdb/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/automation"
)

// processTreeTimeout is long enough for the helper tree to start on a loaded CI runner and short
// enough to keep the test quick.
const processTreeTimeout = "3s"

// startedStepHelpers waits until both helpers of the spawn-grandchild tree recorded their pids.
func startedStepHelpers(t *testing.T, dir string) (int, int) {
	t.Helper()
	var parent, child int
	require.Eventually(t, func() bool {
		parent, child = stepHelperPID(dir, "parent"), stepHelperPID(dir, "child")
		return parent > 0 && child > 0
	}, 10*time.Second, 25*time.Millisecond, "the helper process tree never started; the step timeout may be too short for this machine")
	return parent, child
}

func requireStepProcessesGone(t *testing.T, pids map[string]int) {
	t.Helper()
	for what, pid := range pids {
		require.Eventually(t, func() bool { return stepProcessGone(pid) }, 5*time.Second, 25*time.Millisecond,
			"%s (pid %d) is still running after the step ended", what, pid)
	}
}

// A step timeout must end the commands the step started and the commands those started. The shell
// step runs the command through the in-process interpreter; the atmos step runs a nested process.
func TestStepTimeoutEndsTheWholeProcessTree(t *testing.T) {
	if !stepProcessGroupsSupported {
		t.Skip("Windows terminates only the direct child of a step")
	}
	initShellTestIO(t)
	exe, err := os.Executable()
	require.NoError(t, err)

	for _, tc := range []struct {
		name     string
		stepType string
		fields   func(marker string) map[string]any
	}{
		{"shell step", "shell", func(marker string) map[string]any {
			return map[string]any{"command": `"$WP5_BIN" ` + marker, "output": "none"}
		}},
		{"shell step with a background job", "shell", func(marker string) map[string]any {
			return map[string]any{"command": `"$WP5_BIN" ` + marker + ` & wait`, "output": "none"}
		}},
		{"atmos step", "atmos", func(marker string) map[string]any {
			return map[string]any{"command": marker, "output": "none"}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			killStepHelpersAtCleanup(t, dir)
			marker := "wp5-orphan-marker-" + time.Now().Format("150405.000000000")
			fields := tc.fields(marker)
			fields["timeout"] = processTreeTimeout
			fields["env"] = map[string]string{
				"_ATMOS_STEP_FAKE":  "spawn-grandchild",
				stepHelperPIDDirEnv: dir,
				"WP5_BIN":           exe,
			}

			library := NewAutomationLibrary(NewVariables(), nil)
			_, runErr := library.Run(t.Context(), &automation.StepCall{Type: tc.stepType, Configuration: fields, WorkingDirectory: dir})

			require.ErrorIs(t, runErr, errUtils.ErrStepTimeout, "a step that outlives its timeout reports the step timeout")
			parent, child := startedStepHelpers(t, dir)
			requireStepProcessesGone(t, map[string]int{"command started by the step": parent, "grandchild of that command": child})
		})
	}
}

// Every step type reports a timeout as the step timeout, naming the step, its type, and the duration.
func TestDirectStepCallTimeoutIsAStepTimeout(t *testing.T) {
	initShellTestIO(t)
	exe, err := os.Executable()
	require.NoError(t, err)
	sleepEnv := map[string]string{"_ATMOS_STEP_FAKE": "sleep", atmosStepFakeSleepMSEnv: "30000", "WP5_BIN": exe}

	for _, tc := range []struct {
		name   string
		call   automation.StepCall
		wantIn []string
	}{
		{"shell", automation.StepCall{Type: "shell", Configuration: map[string]any{"name": "nap", "command": `"$WP5_BIN"`, "timeout": "300ms", "output": "none", "env": sleepEnv}}, []string{"nap", "shell", "300ms"}},
		{"atmos", automation.StepCall{Type: "atmos", Configuration: map[string]any{"name": "nested", "command": "version", "timeout": "300ms", "output": "none", "env": sleepEnv}}, []string{"nested", "atmos", "300ms"}},
		{"script", automation.StepCall{Type: "script", Configuration: map[string]any{
			"name": "scripted", "interpreter": "starlark", "timeout": "500ms", "output": "none",
			"script": `exec.run([env["WP5_BIN"]])`, "env": sleepEnv,
		}}, []string{"scripted", "script", "500ms"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			start := time.Now()
			_, runErr := NewAutomationLibrary(NewVariables(), nil).Run(t.Context(), &tc.call)

			require.ErrorIs(t, runErr, errUtils.ErrStepTimeout)
			assert.Less(t, time.Since(start), stepTimeoutBudget, "the timeout must stop the work")
			text := formattedErrorText(runErr)
			for _, want := range tc.wantIn {
				assert.Contains(t, text, want)
			}
		})
	}
}

// A parent that is cancelled is cancellation, not the step's own timeout.
func TestDirectStepCallParentCancellationIsNotAStepTimeout(t *testing.T) {
	initShellTestIO(t)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, err := NewAutomationLibrary(NewVariables(), nil).Run(ctx, &automation.StepCall{Type: "shell", Configuration: map[string]any{"command": "echo hi", "timeout": "1m"}})
	require.Error(t, err)
	assert.NotErrorIs(t, err, errUtils.ErrStepTimeout)
}

// steps.<type>(output="capture") is not a step output mode; the error says where capture lives.
func TestDirectStepCallCaptureOutputHint(t *testing.T) {
	initShellTestIO(t)
	for _, mode := range []string{"capture", "stream"} {
		t.Run(mode, func(t *testing.T) {
			_, err := NewAutomationLibrary(NewVariables(), nil).Run(t.Context(), &automation.StepCall{Type: "shell", Configuration: map[string]any{"command": "echo hi", "output": mode}})

			require.ErrorIs(t, err, errUtils.ErrStepInvalidOutputMode)
			hints := allHints(err)
			for _, mode := range []string{"raw", "log", "viewport", "none"} {
				assert.Contains(t, hints, mode, "the valid step modes are listed")
			}
			assert.Contains(t, hints, `exec.run(argv, output="capture")`)
		})
	}
}

func TestDirectStepCallOtherOutputErrorsKeepTheirHints(t *testing.T) {
	initShellTestIO(t)
	_, err := NewAutomationLibrary(NewVariables(), nil).Run(t.Context(), &automation.StepCall{Type: "shell", Configuration: map[string]any{"command": "echo hi", "output": "bogus"}})
	require.ErrorIs(t, err, errUtils.ErrStepInvalidOutputMode)
	assert.NotContains(t, allHints(err), "exec.run", "only capture and stream point at exec.run")
}

// formattedErrorText returns the message and the explanation details a user sees for err.
func formattedErrorText(err error) string {
	return err.Error() + "\n" + cockroachErrors.FlattenDetails(err)
}
