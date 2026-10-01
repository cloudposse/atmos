package exec

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	stepPkg "github.com/cloudposse/atmos/pkg/runner/step"
	"github.com/cloudposse/atmos/pkg/scheduler"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/workflow"
)

// TestExecuteCustomCommandControlStep_RunsScriptChildAndStoresResult drives
// ExecuteCustomCommandControlStep end to end through its real PrepareEnv/RunCommand closures
// (not a fake executor), the same way a `type: parallel`/`type: matrix` step declared inside a
// custom command's `steps:` list would be dispatched from cmd/cmd_utils.go. The child uses
// `type: script` with Interpreter set to the currently-running test binary's own path -- the
// cross-platform "use the test binary as the subprocess" convention (see testmain_test.go)
// instead of a Unix-only binary like `true`/`sh`, which doesn't exist on Windows. TestMain
// intercepts _ATMOS_TEST_ARGS_FILE and writes the subprocess's argv before any other logic
// runs, letting this assert the real end-to-end dispatch (PrepareEnv building stepEnv,
// RunCommand invoking ExecuteShellCommand) rather than only the constructed strings.
func TestExecuteCustomCommandControlStep_RunsScriptChildAndStoresResult(t *testing.T) {
	argsFile := filepath.Join(t.TempDir(), "args.txt")
	t.Setenv("_ATMOS_TEST_ARGS_FILE", argsFile)

	exePath, err := os.Executable()
	require.NoError(t, err)

	showSummary := false
	parent := &schema.WorkflowStep{
		Name:           "children",
		Type:           schema.TaskTypeParallel,
		Output:         "none",
		ParallelOutput: &schema.ParallelOutputConfig{ShowSummary: &showSummary},
		Steps: []schema.WorkflowStep{
			{Name: "child1", Type: schema.TaskTypeScript, Interpreter: exePath},
		},
	}

	control := &CustomCommandControlContext{
		AtmosConfig: schema.AtmosConfiguration{BasePath: t.TempDir()},
		CommandName: "test-command",
		BaseEnv:     os.Environ(),
		Executor:    stepPkg.NewStepExecutor(),
	}

	err = ExecuteCustomCommandControlStep(context.Background(), control, parent)
	require.NoError(t, err, "the script child (this test binary, intercepted by TestMain) must exit 0")

	content, readErr := os.ReadFile(argsFile)
	require.NoError(t, readErr, "TestMain must have written the subprocess's argv to _ATMOS_TEST_ARGS_FILE, proving RunCommand actually dispatched a subprocess")
	assert.Equal(t, "-", strings.TrimSpace(string(content)), "process.ScriptInvocation's default-interpreter branch invokes `<interpreter> -` with the script on stdin")

	stored, ok := control.Executor.Variables().Steps["child1"]
	require.True(t, ok, "storeCustomCommandControlResult must persist the child's result onto control.Executor, not a package-level global")
	assert.Equal(t, string(scheduler.StatusSucceeded), stored.Metadata["status"])
	assert.False(t, stored.Skipped)
}

// TestStoreCustomCommandControlResult_StoresMetadataAndErrors verifies a failed child's
// stdout/stderr/status/error all land in the *provided* executor's variables -- unlike
// storeWorkflowControlResult (workflow_control_adapter.go), which reaches for a package-level
// stepExecutorState global instead of taking one as a parameter.
func TestStoreCustomCommandControlResult_StoresMetadataAndErrors(t *testing.T) {
	executor := stepPkg.NewStepExecutor()
	errBoom := errors.New("boom")

	storeCustomCommandControlResult(executor, &scheduler.Result{
		NodeID: "child",
		Status: scheduler.StatusFailed,
		Value: &workflow.ControlResult{
			Stdout:   " output \n",
			Stderr:   "warning",
			Err:      errBoom,
			Canceled: true,
		},
	})

	stored, ok := executor.Variables().Steps["child"]
	require.True(t, ok)
	assert.Equal(t, "output", stored.Value)
	assert.Equal(t, " output \n", stored.Metadata["stdout"])
	assert.Equal(t, "warning", stored.Metadata["stderr"])
	assert.Equal(t, string(scheduler.StatusFailed), stored.Metadata["status"])
	assert.Equal(t, true, stored.Metadata["canceled"])
	assert.Equal(t, "boom", stored.Error)
	assert.False(t, stored.Skipped)
}

// TestStoreCustomCommandControlResult_MarksSkippedFallback mirrors
// TestStoreWorkflowControlResultMarksSkippedFallback: a result whose Value isn't a
// *workflow.ControlResult (e.g. a skipped node never dispatched) still stores a valid, empty
// step result marked Skipped rather than panicking on the failed type assertion.
func TestStoreCustomCommandControlResult_MarksSkippedFallback(t *testing.T) {
	executor := stepPkg.NewStepExecutor()

	storeCustomCommandControlResult(executor, &scheduler.Result{
		NodeID: "skipped",
		Status: scheduler.StatusSkipped,
		Value:  "not a control result",
	})

	stored, ok := executor.Variables().Steps["skipped"]
	require.True(t, ok)
	assert.Empty(t, stored.Value)
	assert.True(t, stored.Skipped)
}
