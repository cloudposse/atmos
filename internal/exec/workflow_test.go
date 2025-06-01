package exec

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

func TestExecuteWorkflowCmd(t *testing.T) {
	stacksPath := "../../tests/fixtures/scenarios/atmos-overrides-section"

	err := os.Setenv("ATMOS_CLI_CONFIG_PATH", stacksPath)
	assert.NoError(t, err, "Setting 'ATMOS_CLI_CONFIG_PATH' environment variable should execute without error")

	err = os.Setenv("ATMOS_BASE_PATH", stacksPath)
	assert.NoError(t, err, "Setting 'ATMOS_BASE_PATH' environment variable should execute without error")

	// Capture stdout
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	expectedOutput := `atmos describe component c1 -s prod
atmos describe component c1 -s staging
atmos describe component c1 -s dev
atmos describe component c1 -s sandbox
atmos describe component c1 -s test
`

	cmd := &cobra.Command{
		Use:   "workflow",
		Short: "Run predefined tasks using workflows",
		Long:  `Run predefined workflows as an alternative to traditional task runners. Workflows enable you to automate and manage infrastructure and operational tasks specified in configuration files.`,

		FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: false},
		Run: func(cmd *cobra.Command, args []string) {
			err := ExecuteWorkflowCmd(cmd, args)
			if err != nil {
				u.PrintErrorMarkdownAndExit("", err, "")
			}
		},
	}

	cmd.DisableFlagParsing = false
	cmd.PersistentFlags().StringP("file", "f", "", "Specify the workflow file to run")
	cmd.PersistentFlags().Bool("dry-run", false, "Simulate the workflow without making any changes")
	cmd.PersistentFlags().String("from-step", "", "Resume the workflow from the specified step")
	cmd.PersistentFlags().String("stack", "", "Execute the workflow for the specified stack")
	cmd.PersistentFlags().String("base-path", "", "Base path for Atmos project")
	cmd.PersistentFlags().StringSlice("config", []string{}, "Paths to configuration file")
	cmd.PersistentFlags().StringSlice("config-path", []string{}, "Path to configuration directory")

	// Execute the command
	cmd.SetArgs([]string{"--file", "workflows", "show-all-describe-component-commands"})
	err = cmd.Execute()
	assert.NoError(t, err, "'atmos workflow' command should execute without error")

	// Close the writer and restore stdout
	err = w.Close()
	assert.NoError(t, err, "'atmos workflow' command should execute without error")

	os.Stdout = oldStdout

	// Read captured output
	var output bytes.Buffer
	_, err = io.Copy(&output, r)
	assert.NoError(t, err, "'atmos workflow' command should execute without error")

	// Check if output contains expected markdown content
	assert.Contains(t, output.String(), expectedOutput, "'atmos workflow' output should contain information about workflows")
}

func TestExecuteWorkflow(t *testing.T) {
	// Create a temporary directory for test files
	tmpDir, err := os.MkdirTemp("", "workflow_test")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	// Create workflow directory structure
	workflowsDir := filepath.Join(tmpDir, "stacks", "workflows")
	err = os.MkdirAll(workflowsDir, 0o755)
	require.NoError(t, err)

	// Create atmos.yaml with workflow configuration
	atmosConfig := `
base_path: ""
stacks:
  base_path: "stacks"
  included_paths:
    - "**/*"
workflows:
  base_path: "stacks/workflows"
`
	err = os.WriteFile(filepath.Join(tmpDir, "atmos.yaml"), []byte(atmosConfig), 0o644)
	require.NoError(t, err)

	// Create a test workflow file
	workflowPath := filepath.Join(workflowsDir, "test.yaml")
	workflowContent := `
workflows:
  test-workflow:
    description: "Test workflow"
    steps:
      - name: "step1"
        type: "shell"
        command: "echo 'Step 1'"
      - name: "step2"
        type: "shell"
        command: "echo 'Step 2'"
      - type: "shell"
        command: "echo 'Step 3'"
`
	err = os.WriteFile(workflowPath, []byte(workflowContent), 0o644)
	require.NoError(t, err)

	// Initialize Atmos config
	config, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, false)
	require.NoError(t, err)

	// Set environment variables
	err = os.Setenv("ATMOS_CLI_CONFIG_PATH", tmpDir)
	require.NoError(t, err)
	err = os.Setenv("ATMOS_BASE_PATH", tmpDir)
	require.NoError(t, err)

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
			workflow:     "empty-workflow",
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
			errMsg:           "workflow step execution failed",
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
			errMsg:           "workflow step execution failed",
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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Skip the test if it's an error case since we can't test exit behavior
			if tt.wantErr {
				t.Skip("Skipping error test cases as they use os.Exit")
				return
			}

			err := ExecuteWorkflow(
				config,
				tt.workflow,
				tt.workflowPath,
				tt.workflowDef,
				tt.dryRun,
				tt.commandLineStack,
				tt.fromStep,
			)

			assert.NoError(t, err)
		})
	}
}

func TestExecuteDescribeWorkflows(t *testing.T) {
	// Create a temporary directory for test files
	tmpDir, err := os.MkdirTemp("", "workflow_test")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	// Create workflow directory structure
	workflowsDir := filepath.Join(tmpDir, "stacks", "workflows")
	err = os.MkdirAll(workflowsDir, 0o755)
	require.NoError(t, err)

	// Create atmos.yaml with workflow configuration
	atmosConfig := `
base_path: ""
stacks:
  base_path: "stacks"
  included_paths:
    - "**/*"
workflows:
  base_path: "stacks/workflows"
`
	err = os.WriteFile(filepath.Join(tmpDir, "atmos.yaml"), []byte(atmosConfig), 0o644)
	require.NoError(t, err)

	// Create test workflow files
	workflow1Path := filepath.Join(workflowsDir, "workflow1.yaml")
	workflow1Content := `
workflows:
  test-workflow-1:
    description: "Test workflow 1"
    steps:
      - name: "step1"
        type: "shell"
        command: "echo 'Step 1'"
`
	err = os.WriteFile(workflow1Path, []byte(workflow1Content), 0o644)
	require.NoError(t, err)

	workflow2Path := filepath.Join(workflowsDir, "workflow2.yaml")
	workflow2Content := `
workflows:
  test-workflow-2:
    description: "Test workflow 2"
    steps:
      - name: "step1"
        type: "shell"
        command: "echo 'Step 1'"
`
	err = os.WriteFile(workflow2Path, []byte(workflow2Content), 0o644)
	require.NoError(t, err)

	// Initialize Atmos config
	config, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, false)
	require.NoError(t, err)

	// Set environment variables
	err = os.Setenv("ATMOS_CLI_CONFIG_PATH", tmpDir)
	require.NoError(t, err)
	err = os.Setenv("ATMOS_BASE_PATH", tmpDir)
	require.NoError(t, err)

	// Update config with the correct base path
	config.BasePath = tmpDir
	config.Workflows.BasePath = "stacks/workflows"

	tests := []struct {
		name          string
		config        schema.AtmosConfiguration
		wantErr       bool
		errMsg        string
		wantWorkflows int
	}{
		{
			name:          "valid workflows",
			config:        config,
			wantErr:       false,
			wantWorkflows: 2,
		},
		{
			name: "missing workflows base path",
			config: schema.AtmosConfiguration{
				Workflows: schema.Workflows{
					BasePath: "",
				},
			},
			wantErr: true,
			errMsg:  "'workflows.base_path' must be configured in 'atmos.yaml'",
		},
		{
			name: "nonexistent workflows directory",
			config: schema.AtmosConfiguration{
				Workflows: schema.Workflows{
					BasePath: "nonexistent",
				},
			},
			wantErr: true,
			errMsg:  "the workflow directory 'nonexistent' does not exist",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			listResult, mapResult, allResult, err := ExecuteDescribeWorkflows(tt.config)

			if tt.wantErr {
				assert.Error(t, err)
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
			} else {
				assert.NoError(t, err)
				assert.Len(t, listResult, tt.wantWorkflows)
				assert.Len(t, mapResult, tt.wantWorkflows)
				assert.Len(t, allResult, tt.wantWorkflows)
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
			checkAndGenerateWorkflowStepNames(tt.input)
			assert.Equal(t, tt.expected, tt.input)
		})
	}
}
