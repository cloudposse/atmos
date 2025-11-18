package exec

import (
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/workflow"
)

func TestExecuteWorkflow(t *testing.T) {
	stacksPath := "../../../tests/fixtures/scenarios/workflows"

	t.Setenv("ATMOS_CLI_CONFIG_PATH", stacksPath)
	t.Setenv("ATMOS_BASE_PATH", stacksPath)

	workflowsDir := stacksPath + "/stacks/workflows"
	workflowPath := workflowsDir + "/test.yaml"

	config, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, false)
	assert.NoError(t, err, "'InitCliConfig' should execute without error")

	tests := []struct {
		name             string
		workflow         string
		workflowPath     string
		workflowDef      *schema.WorkflowDefinition
		dryRun           bool
		commandLineStack string
		fromStep         string
		wantErr          bool
		errMsg           string
	}{
		{
			name:         "valid workflow execution",
			workflow:     "test-workflow",
			workflowPath: workflowPath,
			workflowDef: &schema.WorkflowDefinition{
				Steps: []schema.WorkflowStep{
					{
						Name:    "step1",
						Type:    "shell",
						Command: "echo 'Step 1'",
					},
					{
						Name:    "step2",
						Type:    "shell",
						Command: "echo 'Step 2'",
					},
				},
			},
			dryRun:           false,
			commandLineStack: "",
			fromStep:         "",
			wantErr:          false,
		},
		{
			name:         "empty workflow",
			workflow:     "no-steps",
			workflowPath: workflowPath,
			workflowDef: &schema.WorkflowDefinition{
				Steps: []schema.WorkflowStep{},
			},
			dryRun:           false,
			commandLineStack: "",
			fromStep:         "",
			wantErr:          true,
			errMsg:           "workflow has no steps defined",
		},
		{
			name:         "invalid step type",
			workflow:     "invalid-step",
			workflowPath: workflowPath,
			workflowDef: &schema.WorkflowDefinition{
				Steps: []schema.WorkflowStep{
					{
						Name:    "step1",
						Type:    "invalid",
						Command: "echo 'Step 1'",
					},
				},
			},
			dryRun:           false,
			commandLineStack: "",
			fromStep:         "",
			wantErr:          true,
			errMsg:           "invalid workflow step type",
		},
		{
			name:         "invalid from-step",
			workflow:     "test-workflow",
			workflowPath: workflowPath,
			workflowDef: &schema.WorkflowDefinition{
				Steps: []schema.WorkflowStep{
					{
						Name:    "step1",
						Type:    "shell",
						Command: "echo 'Step 1'",
					},
				},
			},
			dryRun:           false,
			commandLineStack: "",
			fromStep:         "nonexistent-step",
			wantErr:          true,
			errMsg:           "invalid from-step flag",
		},
		{
			name:         "failing shell command",
			workflow:     "failing-workflow",
			workflowPath: workflowPath,
			workflowDef: &schema.WorkflowDefinition{
				Steps: []schema.WorkflowStep{
					{
						Name:    "step1",
						Type:    "shell",
						Command: "exit 1",
					},
				},
			},
			dryRun:           false,
			commandLineStack: "",
			fromStep:         "",
			wantErr:          true,
			errMsg:           "subcommand exited with code 1",
		},
		{
			name:         "failing atmos command",
			workflow:     "failing-atmos-workflow",
			workflowPath: workflowPath,
			workflowDef: &schema.WorkflowDefinition{
				Steps: []schema.WorkflowStep{
					{
						Name:    "step1",
						Type:    "atmos",
						Command: "invalid-command",
					},
				},
			},
			dryRun:           false,
			commandLineStack: "",
			fromStep:         "",
			wantErr:          true,
			errMsg:           "subcommand exited with code",
		},
		{
			name:         "workflow with stack override",
			workflow:     "stack-workflow",
			workflowPath: workflowPath,
			workflowDef: &schema.WorkflowDefinition{
				Stack: "prod",
				Steps: []schema.WorkflowStep{
					{
						Name:    "step1",
						Type:    "shell",
						Command: "echo 'Step 1'",
					},
				},
			},
			dryRun:           false,
			commandLineStack: "dev",
			fromStep:         "",
			wantErr:          false,
		},
		{
			name:         "failing atmos command with stack",
			workflow:     "failing-atmos-with-stack",
			workflowPath: workflowPath,
			workflowDef: &schema.WorkflowDefinition{
				Stack: "prod",
				Steps: []schema.WorkflowStep{
					{
						Name:    "step1",
						Type:    "atmos",
						Command: "terraform plan mock -s idontexist",
					},
				},
			},
			dryRun:           false,
			commandLineStack: "",
			fromStep:         "",
			wantErr:          true,
			errMsg:           "subcommand exited with code",
		},
		{
			name:         "failing atmos command with command line stack override",
			workflow:     "failing-atmos-with-cli-stack",
			workflowPath: workflowPath,
			workflowDef: &schema.WorkflowDefinition{
				Stack: "prod",
				Steps: []schema.WorkflowStep{
					{
						Name:    "step1",
						Type:    "atmos",
						Command: "terraform plan mock -s idontexist",
					},
				},
			},
			dryRun:           false,
			commandLineStack: "dev",
			fromStep:         "",
			wantErr:          true,
			errMsg:           "subcommand exited with code",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ExecuteWorkflow(
				config,
				tt.workflow,
				tt.workflowPath,
				tt.workflowDef,
				tt.dryRun,
				tt.commandLineStack,
				tt.fromStep,
				"", // No command-line identity for these tests
			)

			if tt.wantErr {
				assert.Error(t, err)
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestCheckAndGenerateWorkflowStepNames(t *testing.T) {
	tests := []struct {
		name     string
		input    *schema.WorkflowDefinition
		expected *schema.WorkflowDefinition
	}{
		{
			name: "steps with names",
			input: &schema.WorkflowDefinition{
				Steps: []schema.WorkflowStep{
					{
						Name:    "step1",
						Type:    "shell",
						Command: "echo 'Step 1'",
					},
					{
						Name:    "step2",
						Type:    "shell",
						Command: "echo 'Step 2'",
					},
				},
			},
			expected: &schema.WorkflowDefinition{
				Steps: []schema.WorkflowStep{
					{
						Name:    "step1",
						Type:    "shell",
						Command: "echo 'Step 1'",
					},
					{
						Name:    "step2",
						Type:    "shell",
						Command: "echo 'Step 2'",
					},
				},
			},
		},
		{
			name: "steps without names",
			input: &schema.WorkflowDefinition{
				Steps: []schema.WorkflowStep{
					{
						Type:    "shell",
						Command: "echo 'Step 1'",
					},
					{
						Type:    "shell",
						Command: "echo 'Step 2'",
					},
				},
			},
			expected: &schema.WorkflowDefinition{
				Steps: []schema.WorkflowStep{
					{
						Name:    "step1",
						Type:    "shell",
						Command: "echo 'Step 1'",
					},
					{
						Name:    "step2",
						Type:    "shell",
						Command: "echo 'Step 2'",
					},
				},
			},
		},
		{
			name: "mixed steps",
			input: &schema.WorkflowDefinition{
				Steps: []schema.WorkflowStep{
					{
						Name:    "custom-step",
						Type:    "shell",
						Command: "echo 'Step 1'",
					},
					{
						Type:    "shell",
						Command: "echo 'Step 2'",
					},
				},
			},
			expected: &schema.WorkflowDefinition{
				Steps: []schema.WorkflowStep{
					{
						Name:    "custom-step",
						Type:    "shell",
						Command: "echo 'Step 1'",
					},
					{
						Name:    "step2",
						Type:    "shell",
						Command: "echo 'Step 2'",
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			workflow.CheckAndGenerateWorkflowStepNames(tt.input)
			assert.Equal(t, tt.expected, tt.input)
		})
	}
}

// TestExecuteWorkflowCmd tests the ExecuteWorkflowCmd function.
func TestExecuteWorkflowCmd(t *testing.T) {
	// Create a helper to set up cobra command with workflow flags.
	createWorkflowCmd := func() *cobra.Command {
		cmd := &cobra.Command{
			Use: "workflow",
		}
		// Workflow-specific flags.
		cmd.PersistentFlags().StringP("file", "f", "", "Workflow file")
		cmd.PersistentFlags().Bool("dry-run", false, "Dry run")
		cmd.PersistentFlags().StringP("stack", "s", "", "Stack")
		cmd.PersistentFlags().String("from-step", "", "From step")
		cmd.PersistentFlags().String("identity", "", "Identity")

		// Flags expected by ProcessCommandLineArgs.
		cmd.PersistentFlags().String("base-path", "", "Base path")
		cmd.PersistentFlags().StringSlice("config", []string{}, "Config files")
		cmd.PersistentFlags().StringSlice("config-path", []string{}, "Config paths")
		cmd.PersistentFlags().StringSlice("profile", []string{}, "Configuration profile")

		return cmd
	}

	t.Run("successful workflow execution", func(t *testing.T) {
		// Note: These tests are run from the module root, so use paths relative to module root.
		stacksPath := "../../tests/fixtures/scenarios/workflows"

		t.Setenv("ATMOS_CLI_CONFIG_PATH", stacksPath)
		t.Setenv("ATMOS_BASE_PATH", stacksPath)

		cmd := createWorkflowCmd()
		err := cmd.ParseFlags([]string{"--file", "test.yaml"})
		require.NoError(t, err)

		// Execute with workflow name.
		args := []string{"shell-pass"}
		err = ExecuteWorkflowCmd(cmd, args)

		// Should succeed.
		assert.NoError(t, err)
	})

	t.Run("auto-discovery with no file flag - workflow not found", func(t *testing.T) {
		stacksPath := "../../tests/fixtures/scenarios/workflows"

		t.Setenv("ATMOS_CLI_CONFIG_PATH", stacksPath)
		t.Setenv("ATMOS_BASE_PATH", stacksPath)

		cmd := createWorkflowCmd()
		// Don't set --file flag - should auto-discover workflow.

		// Use a workflow name that doesn't exist.
		args := []string{"nonexistent-workflow"}
		err := ExecuteWorkflowCmd(cmd, args)

		// Should error with "no workflow found" message.
		assert.Error(t, err)
		assert.ErrorIs(t, err, ErrWorkflowNoWorkflow)
	})

	t.Run("file not found", func(t *testing.T) {
		// ExecuteWorkflowCmd calls CheckErrorPrintAndExit which exits the process.
		// We can't test this directly without mocking. Skip for now or refactor.
		// This test would require dependency injection to avoid the exit.
		t.Skip("Requires refactoring to avoid CheckErrorPrintAndExit")
	})

	t.Run("absolute file path", func(t *testing.T) {
		stacksPath := "../../tests/fixtures/scenarios/workflows"

		t.Setenv("ATMOS_CLI_CONFIG_PATH", stacksPath)
		t.Setenv("ATMOS_BASE_PATH", stacksPath)

		// Use absolute path to workflow file.
		absPath, err := filepath.Abs("../../tests/fixtures/scenarios/workflows/stacks/workflows/test.yaml")
		require.NoError(t, err)

		cmd := createWorkflowCmd()
		err = cmd.ParseFlags([]string{"--file", absPath})
		require.NoError(t, err)

		args := []string{"shell-pass"}
		err = ExecuteWorkflowCmd(cmd, args)

		assert.NoError(t, err)
	})

	t.Run("file without extension", func(t *testing.T) {
		stacksPath := "../../tests/fixtures/scenarios/workflows"

		t.Setenv("ATMOS_CLI_CONFIG_PATH", stacksPath)
		t.Setenv("ATMOS_BASE_PATH", stacksPath)

		cmd := createWorkflowCmd()
		// Specify file without .yaml extension - should auto-add it.
		err := cmd.ParseFlags([]string{"--file", "test"})
		require.NoError(t, err)

		args := []string{"shell-pass"}
		err = ExecuteWorkflowCmd(cmd, args)

		assert.NoError(t, err)
	})

	t.Run("dry-run flag", func(t *testing.T) {
		stacksPath := "../../tests/fixtures/scenarios/workflows"

		t.Setenv("ATMOS_CLI_CONFIG_PATH", stacksPath)
		t.Setenv("ATMOS_BASE_PATH", stacksPath)

		cmd := createWorkflowCmd()
		err := cmd.ParseFlags([]string{"--file", "test.yaml", "--dry-run"})
		require.NoError(t, err)

		args := []string{"shell-pass"}
		err = ExecuteWorkflowCmd(cmd, args)

		// Dry run should not error.
		assert.NoError(t, err)
	})

	t.Run("stack override", func(t *testing.T) {
		stacksPath := "../../tests/fixtures/scenarios/workflows"

		t.Setenv("ATMOS_CLI_CONFIG_PATH", stacksPath)
		t.Setenv("ATMOS_BASE_PATH", stacksPath)

		cmd := createWorkflowCmd()
		err := cmd.ParseFlags([]string{"--file", "test.yaml", "--stack", "dev"})
		require.NoError(t, err)

		// Use a workflow.
		args := []string{"shell-pass"}
		err = ExecuteWorkflowCmd(cmd, args)

		// Should succeed with stack override.
		assert.NoError(t, err)
	})

	t.Run("from-step flag", func(t *testing.T) {
		stacksPath := "../../tests/fixtures/scenarios/workflows"

		t.Setenv("ATMOS_CLI_CONFIG_PATH", stacksPath)
		t.Setenv("ATMOS_BASE_PATH", stacksPath)

		cmd := createWorkflowCmd()
		err := cmd.ParseFlags([]string{"--file", "test.yaml", "--from-step", "step1"})
		require.NoError(t, err)

		args := []string{"shell-pass"}
		err = ExecuteWorkflowCmd(cmd, args)

		// Should start from step1 (the only step in shell-pass workflow).
		assert.NoError(t, err)
	})

	t.Run("identity flag", func(t *testing.T) {
		stacksPath := "../../tests/fixtures/scenarios/workflows"

		t.Setenv("ATMOS_CLI_CONFIG_PATH", stacksPath)
		t.Setenv("ATMOS_BASE_PATH", stacksPath)

		cmd := createWorkflowCmd()
		err := cmd.ParseFlags([]string{"--file", "test.yaml", "--identity", "test-identity"})
		require.NoError(t, err)

		args := []string{"shell-pass"}
		err = ExecuteWorkflowCmd(cmd, args)

		// Should error because identity doesn't exist (but flag was passed through correctly).
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "test-identity")
	})

	t.Run("invalid workflow manifest - no workflows key", func(t *testing.T) {
		// This will call CheckErrorPrintAndExit which exits the process.
		// Skip for now without dependency injection.
		t.Skip("Requires refactoring to avoid CheckErrorPrintAndExit")
	})

	t.Run("workflow name not found in manifest", func(t *testing.T) {
		// This will call CheckErrorPrintAndExit which exits the process.
		// Skip for now without dependency injection.
		t.Skip("Requires refactoring to avoid CheckErrorPrintAndExit")
	})
}
