package exec

import (
	"context"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	stepPkg "github.com/cloudposse/atmos/pkg/runner/step"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/script"
)

// countingScriptEngine is a registered embedded interpreter that counts executions.
// Dry-run calls only parse the script, so they are not counted as executions.
type countingScriptEngine struct {
	mu    sync.Mutex
	count int
}

//nolint:gocritic // The signature is fixed by script.Engine.
func (e *countingScriptEngine) Execute(_ context.Context, spec script.Spec) (script.Result, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if !spec.DryRun {
		e.count++
	}
	return script.Result{}, nil
}

func (e *countingScriptEngine) runs() int {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.count
}

// scriptChildParent wraps one embedded script child in a parallel step. Shell children are covered
// by the pkg/workflow control executor tests with fake runners, so no shell binary is needed here.
func scriptChildParent(interpreter string) *schema.WorkflowStep {
	showSummary := false
	return &schema.WorkflowStep{
		Name:           "group",
		Type:           schema.TaskTypeParallel,
		Output:         "none",
		ParallelOutput: &schema.ParallelOutputConfig{ShowSummary: &showSummary},
		Steps: []schema.WorkflowStep{
			{Name: "embedded", Type: schema.TaskTypeScript, Interpreter: interpreter, Script: "x"},
		},
	}
}

// TestExecuteWorkflowControlStepDryRunSkipsEmbeddedScriptChild is the regression test for the
// dry-run leak: a Starlark child of a parallel step used to execute for real under --dry-run while
// its shell sibling was skipped.
func TestExecuteWorkflowControlStepDryRunSkipsEmbeddedScriptChild(t *testing.T) {
	engine := &countingScriptEngine{}
	script.Register("counting-workflow-dryrun", engine)
	t.Cleanup(func() { stepExecutorState = nil })
	stepExecutorState = nil

	run := func(dryRun bool) {
		t.Helper()
		err := executeWorkflowControlStep(context.Background(), &workflowControlContext{
			workflowDefinition: &schema.WorkflowDefinition{},
			dryRun:             dryRun,
			baseEnv:            []string{"BASE=1"},
		}, scriptChildParent("counting-workflow-dryrun"))
		require.NoError(t, err)
	}

	run(true)
	assert.Zero(t, engine.runs(), "dry-run must not run the embedded child")

	// Negative path: the same step runs the embedded child when not a dry-run.
	run(false)
	assert.Equal(t, 1, engine.runs())
}

func TestWorkflowControlVariablesInstallsComponentResolution(t *testing.T) {
	t.Cleanup(func() { stepExecutorState = nil })
	stepExecutorState = nil

	atmosConfig := schema.AtmosConfiguration{BasePath: t.TempDir()}
	vars := workflowControlVariables(&workflowControlContext{atmosConfig: atmosConfig})

	require.NotNil(t, vars)
	require.NotNil(t, vars.AtmosConfig)
	assert.Same(t, stepExecutorState.Variables(), vars)

	// With a resolver installed a canceled context surfaces as cancellation; without one the
	// resolver would report that component resolution is unavailable instead.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	resolver := stepPkg.ScriptComponentResolver(vars)
	require.NotNil(t, resolver)
	_, err := resolver(ctx, script.ComponentRef{Name: "api", Stack: "dev", Type: "terraform"})
	assert.ErrorIs(t, err, context.Canceled)
}

func TestWorkflowControlVariablesKeepsExistingAtmosConfig(t *testing.T) {
	t.Cleanup(func() { stepExecutorState = nil })
	stepExecutorState = stepPkg.NewStepExecutor()
	existing := &schema.AtmosConfiguration{BasePath: "existing"}
	stepExecutorState.Variables().SetAtmosConfig(existing)

	vars := workflowControlVariables(&workflowControlContext{atmosConfig: schema.AtmosConfiguration{BasePath: "other"}})

	assert.Same(t, existing, vars.AtmosConfig)
}

func TestCommandEnvOverrides(t *testing.T) {
	assert.Nil(t, commandEnvOverrides(&CustomCommandControlContext{BaseEnv: []string{"A=1"}}))

	overrides := commandEnvOverrides(&CustomCommandControlContext{
		CommandEnv: map[string]string{"A": "declared", "MISSING": "declared"},
		BaseEnv:    []string{"A=resolved", "OTHER=x"},
	})
	assert.Equal(t, map[string]string{"A": "resolved", "MISSING": ""}, overrides)
}

func TestNewCustomCommandControlExecutorWiresScriptContext(t *testing.T) {
	control := &CustomCommandControlContext{
		AtmosConfig: schema.AtmosConfiguration{BasePath: t.TempDir()},
		CommandEnv:  map[string]string{"A": "declared"},
		BaseEnv:     []string{"A=resolved"},
		Executor:    stepPkg.NewStepExecutor(),
	}

	executor := newCustomCommandControlExecutor(control)

	assert.False(t, executor.DryRun, "custom commands have no dry-run mode")
	assert.NotNil(t, executor.ResolveComponent)
	assert.Equal(t, map[string]string{"A": "resolved"}, executor.ScriptProcessOverrides)
	assert.Nil(t, executor.ScriptComponent, "no component is in scope for this command")
}
