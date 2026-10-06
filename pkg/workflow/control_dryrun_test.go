package workflow

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/script"
)

// recordingEngine is a registered embedded interpreter that records every Spec it is asked to run.
type recordingEngine struct {
	mu    sync.Mutex
	specs []script.Spec
}

//nolint:gocritic // The signature is fixed by script.Engine.
func (e *recordingEngine) Execute(_ context.Context, spec script.Spec) (script.Result, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.specs = append(e.specs, spec)
	return script.Result{}, nil
}

func (e *recordingEngine) calls() []script.Spec {
	e.mu.Lock()
	defer e.mu.Unlock()
	return append([]script.Spec(nil), e.specs...)
}

// executed returns the specs that asked the engine to run code rather than only parse it.
func (e *recordingEngine) executed() []script.Spec {
	calls := e.calls()
	var executed []script.Spec
	for i := range calls {
		if !calls[i].DryRun {
			executed = append(executed, calls[i])
		}
	}
	return executed
}

// registerRecordingEngine installs a uniquely named engine so parallel tests never collide.
func registerRecordingEngine(t *testing.T, name string) *recordingEngine {
	t.Helper()
	engine := &recordingEngine{}
	script.Register(name, engine)
	return engine
}

// countingExecutor builds a ControlCommandExecutor whose runners count real executions. A command
// request that carries DryRun is the runner's cue to report instead of execute, so it is not counted.
func countingExecutor(dryRun bool) (*ControlCommandExecutor, *int) {
	var mu sync.Mutex
	calls := 0
	bump := func() {
		mu.Lock()
		defer mu.Unlock()
		calls++
	}
	return &ControlCommandExecutor{
		DryRun: dryRun,
		RunCommand: func(request *ControlCommandRequest) error {
			if !request.DryRun {
				bump()
			}
			return nil
		},
		ShellRunner: func(context.Context, *ControlShellRequest) error { bump(); return nil },
	}, &calls
}

func TestControlExecutorDryRunSkipsEveryChildType(t *testing.T) {
	engine := registerRecordingEngine(t, "recording-dryrun-all")
	children := []schema.WorkflowStep{
		{Name: "shell", Type: schema.TaskTypeShell, Command: "echo shell"},
		{Name: "atmos", Type: schema.TaskTypeAtmos, Command: "version"},
		{Name: "external", Type: schema.TaskTypeScript, Interpreter: "not-an-embedded-interpreter", Script: "echo external"},
		{Name: "embedded", Type: schema.TaskTypeScript, Interpreter: "recording-dryrun-all", Script: "ignored"},
	}

	for _, dryRun := range []bool{true, false} {
		executor, runnerCalls := countingExecutor(dryRun)
		for i := range children {
			result, err := executor.Execute(context.Background(), &ControlChild{Step: children[i]}, ControlChildOutput{})
			require.NoError(t, err, children[i].Name)
			require.NotNil(t, result, children[i].Name)
		}
		if dryRun {
			assert.Zero(t, *runnerCalls, "dry-run must not reach any command runner")
			assert.Empty(t, engine.executed(), "dry-run must only ask the embedded engine to parse")
			assert.Len(t, engine.calls(), 1, "dry-run still parses the embedded script")
			continue
		}
		// Negative path: without DryRun the same children execute.
		assert.Equal(t, 3, *runnerCalls)
		assert.Len(t, engine.executed(), 1)
	}
}

func TestControlExecutorDryRunMarksCommandRequests(t *testing.T) {
	for _, dryRun := range []bool{true, false} {
		var requests []*ControlCommandRequest
		executor := &ControlCommandExecutor{
			DryRun: dryRun,
			RunCommand: func(request *ControlCommandRequest) error {
				requests = append(requests, request)
				return nil
			},
		}
		children := []schema.WorkflowStep{
			{Name: "shell", Type: schema.TaskTypeShell, Command: "echo shell"},
			{Name: "atmos", Type: schema.TaskTypeAtmos, Command: "version"},
			{Name: "external", Type: schema.TaskTypeScript, Interpreter: "not-an-embedded-interpreter", Script: "x"},
		}
		for i := range children {
			_, err := executor.Execute(context.Background(), &ControlChild{Step: children[i]}, ControlChildOutput{})
			require.NoError(t, err)
		}
		require.Len(t, requests, 3, "the runner still sees every child so it can report them")
		for _, request := range requests {
			assert.Equal(t, dryRun, request.DryRun)
		}
	}
}

func TestPlainControlRunCommandHonorsDryRun(t *testing.T) {
	// A dry-run request never starts the (nonexistent) program.
	require.NoError(t, plainControlRunCommand(&ControlCommandRequest{
		DryRun: true, Context: context.Background(), Program: "this-program-does-not-exist",
	}))
	require.Error(t, plainControlRunCommand(&ControlCommandRequest{
		Context: context.Background(), Program: "this-program-does-not-exist",
	}))
}

func TestControlExecutorDryRunStillRejectsEmbeddedInContainer(t *testing.T) {
	engine := registerRecordingEngine(t, "recording-dryrun-container")
	executor, _ := countingExecutor(true)
	executor.WorkflowDefinition = &schema.WorkflowDefinition{Container: &schema.WorkflowContainer{Image: "example"}}

	_, err := executor.Execute(context.Background(), &ControlChild{Step: schema.WorkflowStep{
		Name: "embedded", Type: schema.TaskTypeScript, Interpreter: "recording-dryrun-container", Script: "x",
	}}, ControlChildOutput{})

	require.ErrorIs(t, err, errUtils.ErrScript)
	assert.ErrorContains(t, err, "recording-dryrun-container")
	assert.NotErrorIs(t, err, errUtils.ErrStarlark)
	assert.Empty(t, engine.calls())
}

func TestControlExecutorDryRunMatrixChildren(t *testing.T) {
	initControlTestIO(t)
	engine := registerRecordingEngine(t, "recording-dryrun-matrix")
	executor, runnerCalls := countingExecutor(true)
	parent := &schema.WorkflowStep{
		Name:   "plans",
		Type:   schema.TaskTypeMatrix,
		Matrix: map[string][]string{"stack": {"dev", "prod"}},
		Steps: []schema.WorkflowStep{
			{Name: "embedded", Type: schema.TaskTypeScript, Interpreter: "recording-dryrun-matrix", Script: "{{ .matrix.stack }}"},
			{Name: "shell", Type: schema.TaskTypeShell, Command: "echo {{ .matrix.stack }}"},
		},
	}

	err := ExecuteControlStep(context.Background(), parent, executor.Execute, ControlExecutionOptions{})

	require.NoError(t, err)
	assert.Empty(t, engine.executed(), "matrix script children must not run under dry-run")
	assert.Zero(t, *runnerCalls)
}

func TestControlExecutorDryRunParallelChildren(t *testing.T) {
	initControlTestIO(t)
	engine := registerRecordingEngine(t, "recording-dryrun-parallel")
	executor, runnerCalls := countingExecutor(true)
	parent := &schema.WorkflowStep{
		Name: "group",
		Type: schema.TaskTypeParallel,
		Steps: []schema.WorkflowStep{
			{Name: "embedded", Type: schema.TaskTypeScript, Interpreter: "recording-dryrun-parallel", Script: "x"},
			{Name: "shell", Type: schema.TaskTypeShell, Command: "echo shell"},
		},
	}

	require.NoError(t, ExecuteControlStep(context.Background(), parent, executor.Execute, ControlExecutionOptions{}))

	assert.Empty(t, engine.executed())
	assert.Zero(t, *runnerCalls)
}

func TestControlExecutorDryRunParsesEmbeddedStarlark(t *testing.T) {
	executor, _ := countingExecutor(true)
	executor.BasePath = t.TempDir()
	run := func(source string) error {
		_, err := executor.Execute(context.Background(), &ControlChild{Step: schema.WorkflowStep{
			Name: "embedded", Type: schema.TaskTypeScript, Interpreter: "starlark", Script: source,
		}}, ControlChildOutput{})
		return err
	}

	// A syntax error surfaces even though nothing executes.
	require.ErrorIs(t, run("def broken(:\n    pass\n"), errUtils.ErrStarlark)
	// A valid script that would fail at runtime is not executed under dry-run.
	require.NoError(t, run("fail(\"must not execute\")\n"))
}

func TestControlExecutorPassesScriptContextToEmbeddedChild(t *testing.T) {
	engine := registerRecordingEngine(t, "recording-context")
	ref := &script.ComponentRef{Name: "api", Stack: "dev", Type: "terraform"}
	resolveErr := errors.New("resolver reached")
	hook := &script.HookContext{Name: "hook", Event: "before.terraform.plan"}
	executor := &ControlCommandExecutor{
		ScriptHook:      hook,
		ScriptComponent: ref,
		ResolveComponent: func(context.Context, script.ComponentRef) (*script.Component, error) {
			return nil, resolveErr
		},
		ScriptProcessOverrides: map[string]string{"FROM_COMMAND": "command", "OVERRIDDEN": "command"},
		BaseEnv:                []string{"FROM_COMMAND=command", "OVERRIDDEN=command"},
		PrepareEnv: func(base []string, _, _ string, _, stepEnv map[string]string) ([]string, error) {
			env := append([]string(nil), base...)
			for key, value := range stepEnv {
				env = append(env, key+"="+value)
			}
			return env, nil
		},
	}

	_, err := executor.Execute(context.Background(), &ControlChild{Step: schema.WorkflowStep{
		Name: "child", Type: schema.TaskTypeScript, Interpreter: "recording-context", Script: "x",
		Env: map[string]string{"OVERRIDDEN": "child"},
	}}, ControlChildOutput{})
	require.NoError(t, err)

	calls := engine.calls()
	require.Len(t, calls, 1)
	spec := calls[0]
	assert.Same(t, ref, spec.Component)
	assert.Same(t, hook, spec.Hook)
	require.NotNil(t, spec.ResolveComponent)
	_, resolved := spec.ResolveComponent(context.Background(), *ref)
	assert.ErrorIs(t, resolved, resolveErr)
	assert.Equal(t, "command", spec.ProcessOverrides["FROM_COMMAND"])
	assert.Equal(t, "child", spec.ProcessOverrides["OVERRIDDEN"], "a child's own env keeps precedence over the command-level value")

	// The executor's own map is never mutated by a child's refresh.
	assert.Equal(t, "command", executor.ScriptProcessOverrides["OVERRIDDEN"])
}

func TestControlExecutorWithoutScriptContextHasNoOverrides(t *testing.T) {
	engine := registerRecordingEngine(t, "recording-no-context")
	executor := &ControlCommandExecutor{}

	_, err := executor.Execute(context.Background(), &ControlChild{Step: schema.WorkflowStep{
		Name: "child", Type: schema.TaskTypeScript, Interpreter: "recording-no-context", Script: "x",
	}}, ControlChildOutput{})
	require.NoError(t, err)

	calls := engine.calls()
	require.Len(t, calls, 1)
	assert.Nil(t, calls[0].Component)
	assert.Nil(t, calls[0].ResolveComponent)
	assert.Nil(t, calls[0].ProcessOverrides)
}

func TestResolveControlStepRendersScriptAndInterpreter(t *testing.T) {
	step := &schema.WorkflowStep{
		Name:        "child",
		Type:        schema.TaskTypeScript,
		Interpreter: "{{ .matrix.interp }}",
		Script:      "print('{{ .matrix.word }}')",
	}

	resolved, err := resolveControlStep(step, map[string]string{"interp": "starlark", "word": "rendered"}, nil)

	require.NoError(t, err)
	assert.Equal(t, "starlark", resolved.Interpreter)
	assert.Equal(t, "print('rendered')", resolved.Script)
	// The input step is never mutated.
	assert.Equal(t, "{{ .matrix.interp }}", step.Interpreter)
}

func TestResolveControlStepScriptTemplateError(t *testing.T) {
	_, err := resolveControlStep(&schema.WorkflowStep{Name: "child", Script: "{{ .broken"}, nil, nil)
	require.Error(t, err)
}
