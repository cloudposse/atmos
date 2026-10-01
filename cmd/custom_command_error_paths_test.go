package cmd

import (
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
)

// registerAndRunCustomCommand registers testCommand as a top-level custom command (not anyone's
// dependency), overrides errUtils.OsExit so exitOrRecordDependencyErr's real top-level exit is
// captured instead of killing the test binary, runs it, and reports whether OsExit was reached.
// Shared by the executeCustomCommand error-path tests below: each exercises a distinct failure
// that must fail fast (real behavior: the command reports an error and stops) rather than
// silently skip or corrupt output.
func registerAndRunCustomCommand(t *testing.T, atmosConfig *schema.AtmosConfiguration, testCommand *schema.Command) bool {
	t.Helper()

	atmosConfig.Commands = []schema.Command{*testCommand}
	err := processCustomCommands(*atmosConfig, atmosConfig.Commands, RootCmd)
	require.NoError(t, err)

	var target *cobra.Command
	for _, c := range RootCmd.Commands() {
		if c.Name() == testCommand.Name {
			target = c
			break
		}
	}
	require.NotNil(t, target, "%s command should be registered", testCommand.Name)

	var mu sync.Mutex
	exited := false
	original := errUtils.OsExit
	t.Cleanup(func() { errUtils.OsExit = original })
	errUtils.OsExit = func(code int) {
		mu.Lock()
		exited = true
		mu.Unlock()
	}

	target.Run(target, []string{})

	mu.Lock()
	defer mu.Unlock()
	return exited
}

func loadAuthMockAtmosConfig(t *testing.T) schema.AtmosConfiguration {
	t.Helper()

	testDir := "../tests/fixtures/scenarios/atmos-auth-mock"
	t.Setenv("ATMOS_CLI_CONFIG_PATH", testDir)
	t.Setenv("ATMOS_BASE_PATH", testDir)

	atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, false)
	require.NoError(t, err)
	return atmosConfig
}

// TestCustomCommandIntegration_TrailingArgNulByteFailsToQuote verifies that a trailing (post --)
// argument containing a NUL byte -- which mvdan/sh's syntax.Quote cannot represent in bash syntax
// -- fails the command outright instead of silently dropping or mangling the byte.
func TestCustomCommandIntegration_TrailingArgNulByteFailsToQuote(t *testing.T) {
	if testing.Short() {
		t.Skipf("Skipping integration test in short mode")
	}

	_ = NewTestKit(t)
	atmosConfig := loadAuthMockAtmosConfig(t)

	tmpDir := t.TempDir()
	marker := filepath.Join(tmpDir, "marker.txt")

	oldArgs := os.Args
	t.Cleanup(func() { os.Args = oldArgs })
	os.Args = []string{"atmos", "ep-nul-trailing-arg", "--", "bad\x00arg"}

	exited := registerAndRunCustomCommand(t, &atmosConfig, &schema.Command{
		Name:  "ep-nul-trailing-arg",
		Steps: schema.Tasks{{Type: "shell", Command: customCommandWriteHelperCommand(t, marker, "ran")}},
	})

	assert.True(t, exited, "a NUL byte in a trailing arg must fail shell-quoting and exit")
	assert.NoFileExists(t, marker, "the step must never run once trailing-arg quoting fails")
}

// TestCustomCommandIntegration_WhenMissingEnvKeyFailsPrecheck verifies that a step whose `when:`
// CEL expression indexes an environment variable that doesn't exist is a hard failure -- not
// silently treated as false/skipped -- when it's the ONLY step, so the "does any step run"
// precheck loop is what surfaces the error before any step executes.
func TestCustomCommandIntegration_WhenMissingEnvKeyFailsPrecheck(t *testing.T) {
	if testing.Short() {
		t.Skipf("Skipping integration test in short mode")
	}

	_ = NewTestKit(t)
	atmosConfig := loadAuthMockAtmosConfig(t)

	tmpDir := t.TempDir()
	marker := filepath.Join(tmpDir, "marker.txt")

	exited := registerAndRunCustomCommand(t, &atmosConfig, &schema.Command{
		Name: "ep-when-missing-env-precheck",
		Steps: schema.Tasks{{
			Type:    "shell",
			When:    schema.MustCondition(`env.ATMOS_TEST_MISSING_KEY_PRECHECK == "x"`),
			Command: customCommandWriteHelperCommand(t, marker, "ran"),
		}},
	})

	assert.True(t, exited, "a when: referencing a missing env key must fail, not silently skip")
	assert.NoFileExists(t, marker, "the step must never run when its when: fails to evaluate")
}

// TestCustomCommandIntegration_WhenMissingEnvKeyFailsMainLoop is the main-loop counterpart to the
// precheck test above: the first step has no `when:` (always runs), so the "does any step run"
// precheck short-circuits on it before ever looking at the second step's broken `when:` -- the
// second step's CEL error can only surface once the main execution loop reaches it for real.
func TestCustomCommandIntegration_WhenMissingEnvKeyFailsMainLoop(t *testing.T) {
	if testing.Short() {
		t.Skipf("Skipping integration test in short mode")
	}

	_ = NewTestKit(t)
	atmosConfig := loadAuthMockAtmosConfig(t)

	tmpDir := t.TempDir()
	marker1 := filepath.Join(tmpDir, "first.txt")
	marker2 := filepath.Join(tmpDir, "second.txt")

	exited := registerAndRunCustomCommand(t, &atmosConfig, &schema.Command{
		Name: "ep-when-missing-env-mainloop",
		Steps: schema.Tasks{
			{Name: "first", Type: "shell", Command: customCommandWriteHelperCommand(t, marker1, "first")},
			{
				Name:    "second",
				Type:    "shell",
				When:    schema.MustCondition(`env.ATMOS_TEST_MISSING_KEY_MAINLOOP == "x"`),
				Command: customCommandWriteHelperCommand(t, marker2, "second"),
			},
		},
	})

	assert.True(t, exited, "the second step's broken when: must fail once the main loop reaches it")
	assert.FileExists(t, marker1, "the first step (unconditional) must still run before the failure")
	assert.NoFileExists(t, marker2, "the second step must never run once its own when: fails")
}

// TestCustomCommandIntegration_InvalidWorkingDirectoryExits verifies that a command-level
// working_directory pointing at a nonexistent path fails the command instead of falling back to
// process CWD.
func TestCustomCommandIntegration_InvalidWorkingDirectoryExits(t *testing.T) {
	if testing.Short() {
		t.Skipf("Skipping integration test in short mode")
	}

	_ = NewTestKit(t)
	atmosConfig := loadAuthMockAtmosConfig(t)

	tmpDir := t.TempDir()
	marker := filepath.Join(tmpDir, "marker.txt")

	exited := registerAndRunCustomCommand(t, &atmosConfig, &schema.Command{
		Name:             "ep-bad-command-workdir",
		WorkingDirectory: filepath.Join(tmpDir, "does-not-exist"),
		Steps:            schema.Tasks{{Type: "shell", Command: customCommandWriteHelperCommand(t, marker, "ran")}},
	})

	assert.True(t, exited, "a nonexistent command-level working_directory must exit, not fall back silently")
	assert.NoFileExists(t, marker, "no step should run once the working directory fails to resolve")
}

// TestCustomCommandIntegration_TwoExecStepsRejected verifies that ValidateExecTasks' "exec must
// be the last step" rule is enforced before any step runs. This must never regress into actually
// running the first exec step, since type: exec REPLACES the current process (os.Exec semantics)
// -- reaching real execution here would replace and kill this test binary itself.
func TestCustomCommandIntegration_TwoExecStepsRejected(t *testing.T) {
	if testing.Short() {
		t.Skipf("Skipping integration test in short mode")
	}

	_ = NewTestKit(t)
	atmosConfig := loadAuthMockAtmosConfig(t)

	exited := registerAndRunCustomCommand(t, &atmosConfig, &schema.Command{
		Name: "ep-two-exec-steps",
		Steps: schema.Tasks{
			{Type: schema.TaskTypeExec, Command: "true"},
			{Type: schema.TaskTypeExec, Command: "true"},
		},
	})

	assert.True(t, exited, "a second exec step must be rejected by ValidateExecTasks before any step runs")
}

// TestCustomCommandIntegration_FreshnessComputeBadGlobExits verifies that a malformed
// inputs.sources glob pattern (unterminated "[") is a hard failure at freshness-fact computation
// time -- surfaced to the user, not silently treated as "no matches".
func TestCustomCommandIntegration_FreshnessComputeBadGlobExits(t *testing.T) {
	if testing.Short() {
		t.Skipf("Skipping integration test in short mode")
	}

	_ = NewTestKit(t)
	atmosConfig := loadAuthMockAtmosConfig(t)

	tmpDir := t.TempDir()
	marker := filepath.Join(tmpDir, "marker.txt")

	exited := registerAndRunCustomCommand(t, &atmosConfig, &schema.Command{
		Name: "ep-freshness-bad-glob",
		Steps: schema.Tasks{{
			Type:    "shell",
			Inputs:  &schema.Inputs{Sources: []string{"["}},
			Command: customCommandWriteHelperCommand(t, marker, "ran"),
		}},
	})

	assert.True(t, exited, "a malformed inputs.sources glob pattern must fail freshness computation")
	assert.NoFileExists(t, marker, "the step must never run once freshness computation fails")
}

// TestCustomCommandIntegration_LegacyComponentConfigComponentEmpty verifies that a legacy
// component_config.component whose template renders to an empty string (a non-empty raw
// template, e.g. a conditional that resolves to nothing) is rejected as invalid, rather than
// silently used as an empty component name.
func TestCustomCommandIntegration_LegacyComponentConfigComponentEmpty(t *testing.T) {
	if testing.Short() {
		t.Skipf("Skipping integration test in short mode")
	}

	_ = NewTestKit(t)
	atmosConfig := loadAuthMockAtmosConfig(t)

	tmpDir := t.TempDir()
	marker := filepath.Join(tmpDir, "marker.txt")

	exited := registerAndRunCustomCommand(t, &atmosConfig, &schema.Command{
		Name: "ep-component-config-empty-component",
		ComponentConfig: schema.CommandComponentConfig{
			Component: "{{ if false }}unused{{ end }}",
			Stack:     "irrelevant-stack",
		},
		Steps: schema.Tasks{{Type: "shell", Command: customCommandWriteHelperCommand(t, marker, "ran")}},
	})

	assert.True(t, exited, "a component_config.component template that renders empty must exit")
	assert.NoFileExists(t, marker, "no step should run once component_config.component resolves empty")
}

// TestCustomCommandIntegration_LegacyComponentConfigStackEmpty is the `stack:` sibling of the
// component test above: a valid, literal `component:` but a `stack:` template that renders to an
// empty string must still be rejected.
func TestCustomCommandIntegration_LegacyComponentConfigStackEmpty(t *testing.T) {
	if testing.Short() {
		t.Skipf("Skipping integration test in short mode")
	}

	_ = NewTestKit(t)
	atmosConfig := loadAuthMockAtmosConfig(t)

	tmpDir := t.TempDir()
	marker := filepath.Join(tmpDir, "marker.txt")

	exited := registerAndRunCustomCommand(t, &atmosConfig, &schema.Command{
		Name: "ep-component-config-empty-stack",
		ComponentConfig: schema.CommandComponentConfig{
			Component: "literal-component",
			Stack:     "{{ if false }}unused{{ end }}",
		},
		Steps: schema.Tasks{{Type: "shell", Command: customCommandWriteHelperCommand(t, marker, "ran")}},
	})

	assert.True(t, exited, "a component_config.stack template that renders empty must exit")
	assert.NoFileExists(t, marker, "no step should run once component_config.stack resolves empty")
}

// TestCustomCommandIntegration_LegacyComponentConfigComponentTemplateError verifies that a
// legacy component_config.component template that fails to EXECUTE (as opposed to rendering
// empty -- see TestCustomCommandIntegration_LegacyComponentConfigComponentEmpty) -- here, a
// reference to an undefined template data field -- is surfaced as a command failure, distinct
// from the "rendered empty" case: this exercises e.ProcessTmpl's own error return, not the
// post-render emptiness check.
func TestCustomCommandIntegration_LegacyComponentConfigComponentTemplateError(t *testing.T) {
	if testing.Short() {
		t.Skipf("Skipping integration test in short mode")
	}

	_ = NewTestKit(t)
	atmosConfig := loadAuthMockAtmosConfig(t)

	tmpDir := t.TempDir()
	marker := filepath.Join(tmpDir, "marker.txt")

	exited := registerAndRunCustomCommand(t, &atmosConfig, &schema.Command{
		Name: "ep-component-config-component-tmpl-error",
		ComponentConfig: schema.CommandComponentConfig{
			Component: "{{ .NoSuchField }}",
			Stack:     "irrelevant-stack",
		},
		Steps: schema.Tasks{{Type: "shell", Command: customCommandWriteHelperCommand(t, marker, "ran")}},
	})

	assert.True(t, exited, "a component_config.component template that fails to execute must exit")
	assert.NoFileExists(t, marker, "no step should run once component_config.component fails to render")
}

// TestCustomCommandIntegration_LegacyComponentConfigStackTemplateError is the `stack:` sibling of
// the component template-execution-error test above.
func TestCustomCommandIntegration_LegacyComponentConfigStackTemplateError(t *testing.T) {
	if testing.Short() {
		t.Skipf("Skipping integration test in short mode")
	}

	_ = NewTestKit(t)
	atmosConfig := loadAuthMockAtmosConfig(t)

	tmpDir := t.TempDir()
	marker := filepath.Join(tmpDir, "marker.txt")

	exited := registerAndRunCustomCommand(t, &atmosConfig, &schema.Command{
		Name: "ep-component-config-stack-tmpl-error",
		ComponentConfig: schema.CommandComponentConfig{
			Component: "literal-component",
			Stack:     "{{ .NoSuchField }}",
		},
		Steps: schema.Tasks{{Type: "shell", Command: customCommandWriteHelperCommand(t, marker, "ran")}},
	})

	assert.True(t, exited, "a component_config.stack template that fails to execute must exit")
	assert.NoFileExists(t, marker, "no step should run once component_config.stack fails to render")
}

// TestCustomCommandIntegration_LegacyComponentConfigDescribeComponentError verifies that once
// component_config.component/.stack both resolve successfully (non-empty, no template error),
// a genuinely nonexistent component/stack pair fails at ExecuteDescribeComponent -- not silently
// producing an empty/zero-value {{ .ComponentConfig }} for later template steps.
func TestCustomCommandIntegration_LegacyComponentConfigDescribeComponentError(t *testing.T) {
	if testing.Short() {
		t.Skipf("Skipping integration test in short mode")
	}

	_ = NewTestKit(t)
	atmosConfig := loadAuthMockAtmosConfig(t)

	tmpDir := t.TempDir()
	marker := filepath.Join(tmpDir, "marker.txt")

	exited := registerAndRunCustomCommand(t, &atmosConfig, &schema.Command{
		Name: "ep-component-config-describe-error",
		ComponentConfig: schema.CommandComponentConfig{
			Component: "definitely-not-a-real-component",
			Stack:     "definitely-not-a-real-stack",
		},
		Steps: schema.Tasks{{Type: "shell", Command: customCommandWriteHelperCommand(t, marker, "ran")}},
	})

	assert.True(t, exited, "a nonexistent component/stack pair must fail at ExecuteDescribeComponent")
	assert.NoFileExists(t, marker, "no step should run once ExecuteDescribeComponent fails")
}

// TestCustomCommandIntegration_EnvValueAndValueCommandConflict verifies that declaring both
// `value:` and `valueCommand:` on the same command-level env var is rejected -- the two are
// mutually exclusive, and silently preferring one would be surprising.
func TestCustomCommandIntegration_EnvValueAndValueCommandConflict(t *testing.T) {
	if testing.Short() {
		t.Skipf("Skipping integration test in short mode")
	}

	_ = NewTestKit(t)
	atmosConfig := loadAuthMockAtmosConfig(t)

	tmpDir := t.TempDir()
	marker := filepath.Join(tmpDir, "marker.txt")

	exited := registerAndRunCustomCommand(t, &atmosConfig, &schema.Command{
		Name:  "ep-env-value-conflict",
		Env:   []schema.CommandEnv{{Key: "FOO", Value: "bar", ValueCommand: "echo baz"}},
		Steps: schema.Tasks{{Type: "shell", Command: customCommandWriteHelperCommand(t, marker, "ran")}},
	})

	assert.True(t, exited, "value: and valueCommand: set together must be rejected")
	assert.NoFileExists(t, marker, "no step should run once env var resolution fails")
}

// TestCustomCommandIntegration_EnvValueCommandFailureExits verifies that a command-level env
// var's valueCommand failing (non-zero exit) fails the whole command, rather than proceeding with
// an empty/stale value for that env var.
func TestCustomCommandIntegration_EnvValueCommandFailureExits(t *testing.T) {
	if testing.Short() {
		t.Skipf("Skipping integration test in short mode")
	}

	_ = NewTestKit(t)
	atmosConfig := loadAuthMockAtmosConfig(t)

	tmpDir := t.TempDir()
	marker := filepath.Join(tmpDir, "marker.txt")

	exited := registerAndRunCustomCommand(t, &atmosConfig, &schema.Command{
		Name:  "ep-env-valuecommand-fails",
		Env:   []schema.CommandEnv{{Key: "FOO", ValueCommand: "exit 1"}},
		Steps: schema.Tasks{{Type: "shell", Command: customCommandWriteHelperCommand(t, marker, "ran")}},
	})

	assert.True(t, exited, "a failing valueCommand must exit the command")
	assert.NoFileExists(t, marker, "no step should run once a valueCommand fails")
}

// TestCustomCommandIntegration_EnvValueTemplateErrorExits verifies that a command-level env
// var's `value:` containing an unparsable Go template fails the command instead of passing the
// broken template text through literally.
func TestCustomCommandIntegration_EnvValueTemplateErrorExits(t *testing.T) {
	if testing.Short() {
		t.Skipf("Skipping integration test in short mode")
	}

	_ = NewTestKit(t)
	atmosConfig := loadAuthMockAtmosConfig(t)

	tmpDir := t.TempDir()
	marker := filepath.Join(tmpDir, "marker.txt")

	exited := registerAndRunCustomCommand(t, &atmosConfig, &schema.Command{
		Name:  "ep-env-value-template-error",
		Env:   []schema.CommandEnv{{Key: "FOO", Value: "{{ .Unclosed"}},
		Steps: schema.Tasks{{Type: "shell", Command: customCommandWriteHelperCommand(t, marker, "ran")}},
	})

	assert.True(t, exited, "an unparsable env value template must exit the command")
	assert.NoFileExists(t, marker, "no step should run once an env value template fails to resolve")
}

// TestCustomCommandIntegration_StepEnvTemplateErrorExits is the step-level sibling of the
// command-level env template test above: a per-step `env:` entry with an unparsable Go template
// must fail that step's execution, not silently run with a broken value.
func TestCustomCommandIntegration_StepEnvTemplateErrorExits(t *testing.T) {
	if testing.Short() {
		t.Skipf("Skipping integration test in short mode")
	}

	_ = NewTestKit(t)
	atmosConfig := loadAuthMockAtmosConfig(t)

	exited := registerAndRunCustomCommand(t, &atmosConfig, &schema.Command{
		Name: "ep-step-env-template-error",
		Steps: schema.Tasks{{
			Type:    "shell",
			Env:     map[string]string{"BAR": "{{ .Unclosed"},
			Command: "exit 0",
		}},
	})

	assert.True(t, exited, "an unparsable step-level env template must exit the command")
}

// TestCustomCommandIntegration_StepCommandTemplateErrorExits verifies that a step's own
// `command:` containing an unparsable Go template fails that step, instead of running the
// literal, unrendered template text as a shell command.
func TestCustomCommandIntegration_StepCommandTemplateErrorExits(t *testing.T) {
	if testing.Short() {
		t.Skipf("Skipping integration test in short mode")
	}

	_ = NewTestKit(t)
	atmosConfig := loadAuthMockAtmosConfig(t)

	exited := registerAndRunCustomCommand(t, &atmosConfig, &schema.Command{
		Name:  "ep-step-command-template-error",
		Steps: schema.Tasks{{Type: "shell", Command: "{{ .Unclosed"}},
	})

	assert.True(t, exited, "an unparsable step command template must exit the command")
}

// TestCustomCommandIntegration_StepWorkingDirectoryInvalidExits verifies that a step-level
// working_directory pointing at a nonexistent path fails that step, instead of silently running
// in process CWD or the command's own working directory.
func TestCustomCommandIntegration_StepWorkingDirectoryInvalidExits(t *testing.T) {
	if testing.Short() {
		t.Skipf("Skipping integration test in short mode")
	}

	_ = NewTestKit(t)
	atmosConfig := loadAuthMockAtmosConfig(t)

	tmpDir := t.TempDir()

	exited := registerAndRunCustomCommand(t, &atmosConfig, &schema.Command{
		Name: "ep-step-workdir-invalid",
		Steps: schema.Tasks{{
			Type:             "shell",
			WorkingDirectory: filepath.Join(tmpDir, "does-not-exist"),
			Command:          "exit 0",
		}},
	})

	assert.True(t, exited, "a nonexistent step-level working_directory must exit the command")
}

// TestCustomCommandIntegration_ContinueMissingEnvKeyExits verifies that a malformed `continue:`
// CEL expression (indexing a missing env key) on a failed step is a hard failure -- never
// silently forgiven -- distinguishing "continue: didn't match" (forgiveness withheld) from
// "continue: couldn't even be evaluated" (which must propagate as an error).
func TestCustomCommandIntegration_ContinueMissingEnvKeyExits(t *testing.T) {
	if testing.Short() {
		t.Skipf("Skipping integration test in short mode")
	}

	_ = NewTestKit(t)
	atmosConfig := loadAuthMockAtmosConfig(t)

	exited := registerAndRunCustomCommand(t, &atmosConfig, &schema.Command{
		Name: "ep-continue-missing-env",
		Steps: schema.Tasks{{
			Type:     "shell",
			Command:  "exit 1",
			Continue: schema.MustCondition(`env.ATMOS_TEST_MISSING_KEY_CONTINUE == "x"`),
		}},
	})

	assert.True(t, exited, "a continue: that fails to evaluate must exit, not be silently forgiven")
}

// TestCustomCommandIntegration_ParallelStepThreadsStackFlag verifies that a custom command
// declaring its own `stack` flag has that flag's value threaded into the parallel/matrix control
// step's CommandLineStack -- exercising the `if s, ok := flagsData["stack"].(string)` assignment
// that wires flagsData into the control-step context.
func TestCustomCommandIntegration_ParallelStepThreadsStackFlag(t *testing.T) {
	if testing.Short() {
		t.Skipf("Skipping integration test in short mode")
	}

	_ = NewTestKit(t)
	atmosConfig := loadAuthMockAtmosConfig(t)

	tmpDir := t.TempDir()
	marker := filepath.Join(tmpDir, "marker.txt")

	atmosConfig.Commands = []schema.Command{{
		Name:  "ep-parallel-stack-flag",
		Flags: []schema.CommandFlag{{Name: "stack", Type: "string", Default: "dev"}},
		Steps: schema.Tasks{{
			Name: "checks",
			Type: schema.TaskTypeParallel,
			Steps: []schema.WorkflowStep{
				{Name: "only", Type: "shell", Command: customCommandWriteHelperCommand(t, marker, "ran")},
			},
		}},
	}}
	err := processCustomCommands(atmosConfig, atmosConfig.Commands, RootCmd)
	require.NoError(t, err)

	var target *cobra.Command
	for _, c := range RootCmd.Commands() {
		if c.Name() == "ep-parallel-stack-flag" {
			target = c
			break
		}
	}
	require.NotNil(t, target, "ep-parallel-stack-flag command should be registered")

	target.Run(target, []string{})

	assert.FileExists(t, marker, "the parallel child step must still run when the command declares its own stack flag")
}

// TestCustomCommandIntegration_RecordSuccessBadGlobIsNonFatal verifies the documented contract on
// freshness.Checker.RecordSuccess's call site: a malformed inputs.sources glob pattern that only
// causes RecordSuccess (not Compute) to fail must be logged, not propagated -- an otherwise-
// successful step must not be turned into a command failure by a freshness bookkeeping error.
// Achieved via an explicit `when: always` (bypassing the implicit `when: checksum.changed` that
// Inputs would otherwise imply), so Compute's needs.sourceGlob() is false and only the
// unconditional post-success RecordSuccess call ever globs the bad pattern.
func TestCustomCommandIntegration_RecordSuccessBadGlobIsNonFatal(t *testing.T) {
	if testing.Short() {
		t.Skipf("Skipping integration test in short mode")
	}

	_ = NewTestKit(t)
	atmosConfig := loadAuthMockAtmosConfig(t)

	tmpDir := t.TempDir()
	marker := filepath.Join(tmpDir, "marker.txt")

	exited := registerAndRunCustomCommand(t, &atmosConfig, &schema.Command{
		Name: "ep-recordsuccess-bad-glob",
		Steps: schema.Tasks{{
			Type:    "shell",
			When:    schema.MustCondition("always"),
			Inputs:  &schema.Inputs{Sources: []string{"["}},
			Command: customCommandWriteHelperCommand(t, marker, "ran"),
		}},
	})

	assert.False(t, exited, "a RecordSuccess-only glob failure must not fail an otherwise-successful step")
	assert.FileExists(t, marker, "the step itself must have run and succeeded")
}
