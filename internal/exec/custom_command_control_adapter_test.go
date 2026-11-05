package exec

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/ci"
	"github.com/cloudposse/atmos/pkg/ci/providers/generic"
	"github.com/cloudposse/atmos/pkg/ci/providers/github"
	"github.com/cloudposse/atmos/pkg/ci/providers/github/ghtest"
	stepPkg "github.com/cloudposse/atmos/pkg/runner/step"
	"github.com/cloudposse/atmos/pkg/scheduler"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/script"
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

// TestCustomCommandControlExecutor_ResolvesChildWorkingDirectory verifies parallel/matrix children
// of a custom command resolve relative working_directory against the command's working directory
// (not the project root), inherit it when unset, and let an absolute path win.
func TestCustomCommandControlExecutor_ResolvesChildWorkingDirectory(t *testing.T) {
	projectRoot := t.TempDir()
	cmdDir := filepath.Join(projectRoot, "wd")
	absDir := t.TempDir()

	tests := []struct {
		name       string
		workingDir string
		childDir   string
		wantDir    string
	}{
		{name: "relative child resolves against command dir", workingDir: cmdDir, childDir: "sub", wantDir: filepath.Join(cmdDir, "sub")},
		{name: "child without dir inherits command dir", workingDir: cmdDir, childDir: "", wantDir: cmdDir},
		{name: "absolute child wins", workingDir: cmdDir, childDir: absDir, wantDir: absDir},
		{name: "no command dir falls back to base path", workingDir: "", childDir: "sub", wantDir: filepath.Join(projectRoot, "sub")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			control := &CustomCommandControlContext{
				AtmosConfig:      schema.AtmosConfiguration{BasePath: projectRoot},
				WorkingDirectory: tt.workingDir,
			}
			executor := newCustomCommandControlExecutor(control)

			var gotDir string
			executor.RunCommand = func(request *workflow.ControlCommandRequest) error {
				gotDir = request.Dir
				return nil
			}

			for _, stepType := range []string{schema.TaskTypeShell, schema.TaskTypeScript} {
				gotDir = ""
				step := schema.WorkflowStep{Name: "child", Type: stepType, WorkingDirectory: tt.childDir, Command: "echo hi", Interpreter: "python3", Script: "print(1)"}
				_, err := executor.Execute(context.Background(), &workflow.ControlChild{Step: step}, workflow.ControlChildOutput{Mode: workflow.ControlOutputNone})
				require.NoError(t, err)
				assert.Equal(t, tt.wantDir, gotDir, "step type %s", stepType)
			}
		})
	}
}

func TestCustomCommandControlStarlarkInputs(t *testing.T) {
	for _, kind := range []string{schema.TaskTypeParallel, schema.TaskTypeMatrix} {
		t.Run(kind, func(t *testing.T) {
			executor := stepPkg.NewStepExecutor()
			executor.Variables().SetTemplateData(map[string]any{
				"Flags":     map[string]any{"enabled": true, "literal": "{{ .Env.NOT_DEFINED }}"},
				"Arguments": map[string]string{"service": "api"},
			})
			parent := &schema.WorkflowStep{Name: "group", Type: kind, Output: "none", Steps: []schema.WorkflowStep{{
				Name: "inspect", Type: schema.TaskTypeScript, Interpreter: "starlark",
				Script: `output = {"enabled": ctx.flags["enabled"], "literal": ctx.flags["literal"], "service": ctx.arguments["service"]}`,
			}}}
			expected := 1
			if kind == schema.TaskTypeMatrix {
				parent.Matrix = map[string][]string{"region": {"east", "west"}}
				expected = 2
			}
			err := ExecuteCustomCommandControlStep(t.Context(), &CustomCommandControlContext{
				AtmosConfig: schema.AtmosConfiguration{BasePath: t.TempDir()},
				Executor:    executor,
			}, parent)
			require.NoError(t, err)
			require.Len(t, executor.Variables().Steps, expected)
			for _, result := range executor.Variables().Steps {
				assert.JSONEq(t, `{"enabled":true,"literal":"{{ .Env.NOT_DEFINED }}","service":"api"}`, result.Value)
			}
		})
	}
}

// ciProbeEngine is a script engine that reports through the CI reporter its host supplied, so a
// test can tell whether the host wired the configured reporter or left the nil fallback.
type ciProbeEngine struct {
	mu       sync.Mutex
	receipts map[string]ci.Receipt
	missing  []string
}

//nolint:gocritic // The signature is fixed by script.Engine.
func (e *ciProbeEngine) Execute(_ context.Context, spec script.Spec) (script.Result, error) {
	if spec.DryRun {
		return script.Result{}, nil
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if spec.CI == nil {
		e.missing = append(e.missing, spec.Name)
		return script.Result{}, nil
	}
	rc, err := spec.CI.Summary("probe " + spec.Name + "\n")
	if err != nil {
		return script.Result{}, err
	}
	e.receipts[spec.Name] = rc
	return script.Result{}, nil
}

// ciHostHarness registers a probe interpreter and a fake GitHub Actions environment. The config
// enables ci.enabled, so a configured reporter writes the summary through the detected GitHub
// provider, while the nil-config fallback reports the ci.enabled gate and renders locally.
type ciHostHarness struct {
	interpreter string
	engine      *ciProbeEngine
	env         ghtest.Env
	config      schema.AtmosConfiguration
}

func newCIHostHarness(t *testing.T) *ciHostHarness {
	t.Helper()
	server := ghtest.NewServer(t)
	env := ghtest.SetEnv(t, server)
	t.Chdir(t.TempDir())
	ghtest.RegisterProvider(t, github.NewProvider())
	ci.Register(generic.NewProvider())

	h := &ciHostHarness{
		interpreter: "ci-host-probe-" + strings.ReplaceAll(strings.ToLower(t.Name()), "/", "-"),
		engine:      &ciProbeEngine{receipts: map[string]ci.Receipt{}},
		env:         env,
		config:      schema.AtmosConfiguration{BasePath: t.TempDir(), CI: schema.CIConfig{Enabled: true}},
	}
	script.Register(h.interpreter, h.engine)
	return h
}

// child returns a script child step that runs on the probe interpreter.
func (h *ciHostHarness) child(name string) schema.WorkflowStep {
	return schema.WorkflowStep{Name: name, Type: schema.TaskTypeScript, Interpreter: h.interpreter, Script: "ignored"}
}

// receiptFor finds the receipt of the script whose name is, or ends with, name. Matrix children
// are named after their row. The caller holds the engine lock.
func (h *ciHostHarness) receiptFor(name string) (ci.Receipt, bool) {
	for key, rc := range h.engine.receipts {
		if key == name || strings.HasSuffix(key, "_"+name) {
			return rc, true
		}
	}
	return ci.Receipt{}, false
}

// assertConfigured verifies the named script child received the configured reporter.
func (h *ciHostHarness) assertConfigured(t *testing.T, names ...string) {
	t.Helper()
	h.engine.mu.Lock()
	defer h.engine.mu.Unlock()
	assert.Empty(t, h.engine.missing, "the host must supply a CI reporter, not leave the nil fallback")
	for _, name := range names {
		rc, ok := h.receiptFor(name)
		require.True(t, ok, "script %q must have run", name)
		assert.Equal(t, github.ProviderName, rc.Provider, "script %q must report through the detected provider", name)
		assert.Empty(t, rc.Gate, "script %q must not see a gate: the configured reporter has ci.enabled", name)
		assert.False(t, rc.Local, "script %q must not render locally", name)
	}
	assert.Contains(t, ghtest.ReadFile(t, h.env.Summary), "probe ")
}

func TestCustomCommandControlStep_ParallelChildGetsConfiguredCIReporter(t *testing.T) {
	h := newCIHostHarness(t)
	showSummary := false
	parent := &schema.WorkflowStep{
		Name: "fanout", Type: schema.TaskTypeParallel, Output: "none",
		ParallelOutput: &schema.ParallelOutputConfig{ShowSummary: &showSummary},
		Steps:          []schema.WorkflowStep{h.child("child")},
	}
	control := &CustomCommandControlContext{
		AtmosConfig: h.config,
		CommandName: "ft-ci-parallel",
		BaseEnv:     os.Environ(),
		Executor:    stepPkg.NewStepExecutor(),
	}

	require.NoError(t, ExecuteCustomCommandControlStep(context.Background(), control, parent))

	h.assertConfigured(t, "child")
}

func TestCustomCommandControlStep_MatrixChildGetsConfiguredCIReporter(t *testing.T) {
	h := newCIHostHarness(t)
	showSummary := false
	parent := &schema.WorkflowStep{
		Name: "fanout", Type: schema.TaskTypeMatrix, Output: "none",
		Matrix:         map[string][]string{"env": {"dev"}},
		ParallelOutput: &schema.ParallelOutputConfig{ShowSummary: &showSummary},
		Steps:          []schema.WorkflowStep{h.child("child")},
	}
	control := &CustomCommandControlContext{
		AtmosConfig: h.config,
		CommandName: "ft-ci-matrix",
		BaseEnv:     os.Environ(),
		Executor:    stepPkg.NewStepExecutor(),
	}

	require.NoError(t, ExecuteCustomCommandControlStep(context.Background(), control, parent))

	h.assertConfigured(t, "child")
}

// TestCustomCommandControlStep_ReporterCarriesTheCommandConfig is the negative path: with ci.enabled
// off the same wiring reports the ci.enabled gate, which is exactly what the nil fallback hid.
func TestCustomCommandControlStep_ReporterCarriesTheCommandConfig(t *testing.T) {
	h := newCIHostHarness(t)
	h.config.CI.Enabled = false
	showSummary := false
	parent := &schema.WorkflowStep{
		Name: "fanout", Type: schema.TaskTypeParallel, Output: "none",
		ParallelOutput: &schema.ParallelOutputConfig{ShowSummary: &showSummary},
		Steps:          []schema.WorkflowStep{h.child("child")},
	}
	control := &CustomCommandControlContext{AtmosConfig: h.config, BaseEnv: os.Environ(), Executor: stepPkg.NewStepExecutor()}

	require.NoError(t, ExecuteCustomCommandControlStep(context.Background(), control, parent))

	h.engine.mu.Lock()
	defer h.engine.mu.Unlock()
	assert.Empty(t, h.engine.missing)
	rc, ok := h.receiptFor("child")
	require.True(t, ok)
	assert.NotEmpty(t, rc.Gate, "the reporter must be built from the command's config, where ci.enabled is off")
	assert.True(t, rc.Local)
}

// TestScriptStepHandlerGetsConfiguredCIReporter covers every host that runs a script through the
// step registry: a direct custom-command step, a workflow step, and a lifecycle hook step all
// build their Variables with the active Atmos configuration, which the handler turns into the
// reporter.
func TestScriptStepHandlerGetsConfiguredCIReporter(t *testing.T) {
	h := newCIHostHarness(t)
	handler, ok := stepPkg.Get(schema.TaskTypeScript)
	require.True(t, ok)

	vars := stepPkg.NewVariables()
	vars.SetAtmosConfig(&h.config)
	step := h.child("direct")
	_, err := handler.Execute(context.Background(), &step, vars)
	require.NoError(t, err)

	h.assertConfigured(t, "direct")
}
