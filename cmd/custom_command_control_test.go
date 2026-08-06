package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

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
						Name: "first",
						Type: "shell",
						// filepath.ToSlash: an unquoted Windows backslash path reaching mvdan/sh
						// (pkg/utils/shell_utils.go) would have its backslashes consumed as shell
						// escapes; forward slashes are valid path separators on Windows too.
						Command: "echo first >> " + filepath.ToSlash(orderFile),
					},
					{
						Name:    "second",
						Type:    "shell",
						Command: "echo second >> " + filepath.ToSlash(orderFile),
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
	assert.Equal(t, "first\nsecond\n", string(content), "second must run after first per sibling needs:, even though the group runs concurrently")
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
				// filepath.ToSlash: see the comment on the parallel-needs test above.
				Command:  "echo fail >> " + filepath.ToSlash(failFile) + " && exit 7",
				Type:     "shell",
				Continue: schema.MustCondition(schema.ConditionPredicateAlways),
			},
			{
				Command: "echo ran >> " + filepath.ToSlash(ranFile),
				Type:    "shell",
			},
			{
				Command: "echo failure-handler >> " + filepath.ToSlash(handlerFile),
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
