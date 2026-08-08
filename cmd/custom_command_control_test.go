package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
)

// TestCustomCommandIntegration_ParallelStepWithNeeds exercises the migration-guide recipe for
// porting go-task's `deps:` (a `type: parallel` step with sibling `needs:`) inside a custom
// command. Before the control-step adapter, this failed with "parallel steps require workflow
// executor context" because custom commands never wired into pkg/workflow.ExecuteControlStep.
func TestCustomCommandIntegration_ParallelStepWithNeeds(t *testing.T) {
	if testing.Short() {
		t.Skipf("Skipping integration test in short mode")
	}

	testDir := "../tests/fixtures/scenarios/atmos-auth-mock"
	t.Setenv("ATMOS_CLI_CONFIG_PATH", testDir)
	t.Setenv("ATMOS_BASE_PATH", testDir)

	_ = NewTestKit(t)

	atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, false)
	require.NoError(t, err)

	tmpDir := t.TempDir()
	orderFile := filepath.Join(tmpDir, "order.txt")

	testCommand := schema.Command{
		Name:        "test-parallel-needs",
		Description: "Test parallel step with sibling needs works in custom commands",
		Steps: schema.Tasks{
			{
				Name: "checks",
				Type: schema.TaskTypeParallel,
				Steps: []schema.WorkflowStep{
					{
						Name:    "first",
						Type:    "shell",
						Command: customCommandAppendHelperCommand(t, orderFile, "first"),
					},
					{
						Name:    "second",
						Type:    "shell",
						Command: customCommandAppendHelperCommand(t, orderFile, "second"),
						Needs:   []string{"first"},
					},
				},
			},
		},
	}

	atmosConfig.Commands = []schema.Command{testCommand}

	err = processCustomCommands(atmosConfig, atmosConfig.Commands, RootCmd)
	require.NoError(t, err)

	var customCmd *cobra.Command
	for _, c := range RootCmd.Commands() {
		if c.Name() == "test-parallel-needs" {
			customCmd = c
			break
		}
	}
	require.NotNil(t, customCmd, "Custom command should be registered")

	customCmd.Run(customCmd, []string{})

	content, err := os.ReadFile(orderFile)
	require.NoError(t, err, "parallel step must execute instead of erroring on missing workflow executor context")
	// splitNonEmptyLines trims each line (\r and surrounding whitespace): mvdan/sh's `echo`+`>>`
	// redirect on Windows CI has been observed producing a trailing space and \r before the \n,
	// which isn't a correctness bug in the redirect itself (see the ToSlash comments above) --
	// exact byte comparison would assert on shell/OS text formatting Atmos doesn't control.
	assert.Equal(t, []string{"first", "second"}, splitNonEmptyLines(string(content)), "second must run after first per sibling needs:, even though the group runs concurrently")
}

// TestCustomCommandIntegration_MatrixStepResolvesMatrixValues verifies that a `type: matrix` step
// declared inside a custom command's steps: list can reference `{{ .matrix.* }}` in its command,
// proving the control-step engine (pkg/workflow/control.go's controlTemplateData) injects matrix
// values independently of what the custom-command adapter's TemplateData callback does with its own
// matrix argument -- ExecuteCustomCommandControlStep's callback ignores it deliberately, matching
// the workflow control adapter's identical (and already-covered) pattern.
func TestCustomCommandIntegration_MatrixStepResolvesMatrixValues(t *testing.T) {
	if testing.Short() {
		t.Skipf("Skipping integration test in short mode")
	}

	testDir := "../tests/fixtures/scenarios/atmos-auth-mock"
	t.Setenv("ATMOS_CLI_CONFIG_PATH", testDir)
	t.Setenv("ATMOS_BASE_PATH", testDir)

	_ = NewTestKit(t)

	atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, false)
	require.NoError(t, err)

	tmpDir := t.TempDir()
	rowsFile := filepath.Join(tmpDir, "rows.txt")

	showSummary := false
	testCommand := schema.Command{
		Name:        "test-matrix-values",
		Description: "Test type: matrix step resolves .matrix.* in a custom command",
		Steps: schema.Tasks{
			{
				Name: "plans",
				Type: schema.TaskTypeMatrix,
				Matrix: map[string][]string{
					"component": {"vpc", "eks"},
					"stack":     {"dev"},
				},
				ParallelOutput: &schema.ParallelOutputConfig{
					Mode:        "none",
					ShowSummary: &showSummary,
				},
				Steps: []schema.WorkflowStep{
					{
						Name:    "plan",
						Type:    "shell",
						Command: customCommandMatrixWriteHelperCommand(t, rowsFile),
					},
				},
			},
		},
	}

	atmosConfig.Commands = []schema.Command{testCommand}

	err = processCustomCommands(atmosConfig, atmosConfig.Commands, RootCmd)
	require.NoError(t, err)

	var customCmd *cobra.Command
	for _, c := range RootCmd.Commands() {
		if c.Name() == "test-matrix-values" {
			customCmd = c
			break
		}
	}
	require.NotNil(t, customCmd, "Custom command should be registered")

	customCmd.Run(customCmd, []string{})

	content, err := os.ReadFile(rowsFile)
	require.NoError(t, err, "matrix rows must execute")
	assert.ElementsMatch(t, []string{"component=vpc,stack=dev", "component=eks,stack=dev"}, splitNonEmptyLines(string(content)),
		"each matrix row's command must see its own resolved .matrix.component/.matrix.stack values")
}

// TestCustomCommandIntegration_ContinueAlwaysForgivesFailure verifies GitHub-Actions-
// continue-on-error semantics for custom commands: a step with `continue: always` that fails
// still lets subsequent steps run, and the overall command does not exit with an error.
func TestCustomCommandIntegration_ContinueAlwaysForgivesFailure(t *testing.T) {
	if testing.Short() {
		t.Skipf("Skipping integration test in short mode")
	}

	testDir := "../tests/fixtures/scenarios/atmos-auth-mock"
	t.Setenv("ATMOS_CLI_CONFIG_PATH", testDir)
	t.Setenv("ATMOS_BASE_PATH", testDir)

	_ = NewTestKit(t)

	atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, false)
	require.NoError(t, err)

	tmpDir := t.TempDir()
	failFile := filepath.Join(tmpDir, "fail.txt")
	ranFile := filepath.Join(tmpDir, "ran.txt")
	handlerFile := filepath.Join(tmpDir, "failure-handler.txt")

	testCommand := schema.Command{
		Name:        "test-continue-always",
		Description: "Test continue: always forgives a step failure",
		Steps: schema.Tasks{
			{
				Command:  customCommandWriteAndExitHelperCommand(t, failFile, "fail", 7),
				Type:     "shell",
				Continue: schema.MustCondition(schema.ConditionPredicateAlways),
			},
			{
				Command: customCommandWriteHelperCommand(t, ranFile, "ran"),
				Type:    "shell",
			},
			{
				Command: customCommandWriteHelperCommand(t, handlerFile, "failure-handler"),
				Type:    "shell",
				When:    schema.MustCondition(schema.ConditionPredicateFailure),
			},
		},
	}
	atmosConfig.Commands = []schema.Command{testCommand}

	err = processCustomCommands(atmosConfig, atmosConfig.Commands, RootCmd)
	require.NoError(t, err)

	var customCmd *cobra.Command
	for _, c := range RootCmd.Commands() {
		if c.Name() == "test-continue-always" {
			customCmd = c
			break
		}
	}
	require.NotNil(t, customCmd, "Custom command should be registered")

	customCmd.Run(customCmd, []string{})

	assert.FileExists(t, failFile)
	assert.FileExists(t, ranFile, "step after a forgiven failure must still run")
	assert.NoFileExists(t, handlerFile, "a forgiven failure must not satisfy a later when: failure")
}

// TestCustomCommandIntegration_UnforgivenFailureSkipsPlainStepsButRunsFailureHandler verifies the
// no-`continue:` (default) case: an unforgiven step failure does NOT literally halt the step loop
// (there is deliberately no `break` -- see the loop's own comments); instead it flips the running
// status to failure, which the implicit `when: success` gate on later plain steps evaluates false
// (skipping them), while a later step with an explicit `when: failure` still gets evaluated and
// runs -- exactly mirroring GitHub Actions' `if: success()` default / `if: failure()` override
// semantics, and the pkg/workflow executor's identical, pre-existing loop shape.
func TestCustomCommandIntegration_UnforgivenFailureSkipsPlainStepsButRunsFailureHandler(t *testing.T) {
	if testing.Short() {
		t.Skipf("Skipping integration test in short mode")
	}

	testDir := "../tests/fixtures/scenarios/atmos-auth-mock"
	t.Setenv("ATMOS_CLI_CONFIG_PATH", testDir)
	t.Setenv("ATMOS_BASE_PATH", testDir)

	_ = NewTestKit(t)

	atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, false)
	require.NoError(t, err)

	tmpDir := t.TempDir()
	failFile := filepath.Join(tmpDir, "fail.txt")
	skippedFile := filepath.Join(tmpDir, "skipped.txt")
	handlerFile := filepath.Join(tmpDir, "failure-handler.txt")

	testCommand := schema.Command{
		Name:        "test-unforgiven-failure",
		Description: "Test an unforgiven step failure skips plain steps but still runs a when: failure handler",
		Steps: schema.Tasks{
			{
				Command: customCommandWriteAndExitHelperCommand(t, failFile, "fail", 1),
				Type:    "shell",
			},
			{
				Command: customCommandWriteHelperCommand(t, skippedFile, "skipped"),
				Type:    "shell",
			},
			{
				Command: customCommandWriteHelperCommand(t, handlerFile, "failure-handler"),
				Type:    "shell",
				When:    schema.MustCondition(schema.ConditionPredicateFailure),
			},
		},
	}
	atmosConfig.Commands = []schema.Command{testCommand}

	err = processCustomCommands(atmosConfig, atmosConfig.Commands, RootCmd)
	require.NoError(t, err)

	var customCmd *cobra.Command
	for _, c := range RootCmd.Commands() {
		if c.Name() == "test-unforgiven-failure" {
			customCmd = c
			break
		}
	}
	require.NotNil(t, customCmd, "Custom command should be registered")

	originalOsExit := errUtils.OsExit
	t.Cleanup(func() { errUtils.OsExit = originalOsExit })
	errUtils.OsExit = func(code int) { panic(code) }

	assert.Panics(t, func() {
		customCmd.Run(customCmd, []string{})
	}, "an unforgiven step failure must exit non-zero")

	assert.FileExists(t, failFile, "the failing step itself must still run")
	assert.NoFileExists(t, skippedFile, "a plain step (implicit when: success) after an unforgiven failure must be skipped")
	assert.FileExists(t, handlerFile, "a when: failure step must still run after an unforgiven failure")
}

// TestCustomCommandIntegration_ParallelStepWithUnknownNeedsFailsValidation verifies that
// executeCustomCommand now runs commandConfig.Steps through schema.ValidateWorkflowSteps -- the
// same needs:-graph validator workflows already use -- before doing anything else (including
// resolving dependencies.commands/dependencies.workflows or running any step). A `needs:`
// reference to a sibling step name that does not exist inside a `type: parallel` group must be
// rejected up front, and none of the command's steps must ever run.
func TestCustomCommandIntegration_ParallelStepWithUnknownNeedsFailsValidation(t *testing.T) {
	if testing.Short() {
		t.Skipf("Skipping integration test in short mode")
	}

	testDir := "../tests/fixtures/scenarios/atmos-auth-mock"
	t.Setenv("ATMOS_CLI_CONFIG_PATH", testDir)
	t.Setenv("ATMOS_BASE_PATH", testDir)

	_ = NewTestKit(t)

	atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, false)
	require.NoError(t, err)

	tmpDir := t.TempDir()
	ranFile := filepath.Join(tmpDir, "ran.txt")

	testCommand := schema.Command{
		Name:        "test-parallel-unknown-needs",
		Description: "Test a parallel step's needs: referencing an unknown sibling fails validation before any step runs",
		Steps: schema.Tasks{
			{
				Name: "checks",
				Type: schema.TaskTypeParallel,
				Steps: []schema.WorkflowStep{
					{
						Name:    "first",
						Type:    "shell",
						Command: customCommandWriteHelperCommand(t, ranFile, "first"),
						Needs:   []string{"does-not-exist"},
					},
				},
			},
		},
	}
	atmosConfig.Commands = []schema.Command{testCommand}

	err = processCustomCommands(atmosConfig, atmosConfig.Commands, RootCmd)
	require.NoError(t, err)

	var customCmd *cobra.Command
	for _, c := range RootCmd.Commands() {
		if c.Name() == "test-parallel-unknown-needs" {
			customCmd = c
			break
		}
	}
	require.NotNil(t, customCmd, "Custom command should be registered")

	originalOsExit := errUtils.OsExit
	t.Cleanup(func() { errUtils.OsExit = originalOsExit })
	errUtils.OsExit = func(code int) { panic(code) }

	assert.Panics(t, func() {
		customCmd.Run(customCmd, []string{})
	}, "an unknown needs: reference must fail validation and exit non-zero")

	assert.NoFileExists(t, ranFile, "no step may run once needs:-graph validation rejects the command")
}
