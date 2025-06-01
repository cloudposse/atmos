package workflow

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
)

// TestExitHandler implements the ExitHandler interface for testing
type TestExitHandler struct {
	LastError error
}

func (h *TestExitHandler) PrintErrorAndExit(title string, err error, message string) {
	h.LastError = err
}

// setupTestWorkflowEnvironment creates a temporary test environment with the necessary directory structure and configuration files.
// It returns the temporary directory path and a cleanup function.
func setupTestWorkflowEnvironment(t *testing.T) (string, func()) {
	tmpDir, err := os.MkdirTemp("", "workflow_test")
	require.NoError(t, err)

	workflowsDir := filepath.Join(tmpDir, "stacks", "workflows")
	err = os.MkdirAll(workflowsDir, 0o755)
	require.NoError(t, err)

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

	// Set environment variables
	err = os.Setenv("ATMOS_CLI_CONFIG_PATH", tmpDir)
	require.NoError(t, err)
	err = os.Setenv("ATMOS_BASE_PATH", tmpDir)
	require.NoError(t, err)

	cleanup := func() { os.RemoveAll(tmpDir) }
	return tmpDir, cleanup
}

// createTestWorkflowFile creates a workflow file in the specified directory with the given content.
func createTestWorkflowFile(t *testing.T, dir string, filename string, content string) string {
	workflowPath := filepath.Join(dir, filename)
	err := os.WriteFile(workflowPath, []byte(content), 0o644)
	require.NoError(t, err)
	return workflowPath
}

// initTestConfig initializes the Atmos configuration for testing.
func initTestConfig(t *testing.T) schema.AtmosConfiguration {
	config, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, false)
	require.NoError(t, err)
	return config
}

func TestExecuteWorkflow(t *testing.T) {
	stacksPath := "../../tests/fixtures/scenarios/workflows"

	err := os.Setenv("ATMOS_CLI_CONFIG_PATH", stacksPath)
	assert.NoError(t, err, "Setting 'ATMOS_CLI_CONFIG_PATH' environment variable should execute without error")

	err = os.Setenv("ATMOS_BASE_PATH", stacksPath)
	assert.NoError(t, err, "Setting 'ATMOS_BASE_PATH' environment variable should execute without error")

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
			testExitHandler := &TestExitHandler{}
			err := ExecuteWorkflow(
				config,
				tt.workflow,
				tt.workflowPath,
				tt.workflowDef,
				tt.dryRun,
				tt.commandLineStack,
				tt.fromStep,
				testExitHandler,
			)

			if tt.wantErr {
				assert.Error(t, err)
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
				if testExitHandler.LastError != nil {
					assert.Contains(t, testExitHandler.LastError.Error(), tt.errMsg)
				}
			} else {
				assert.NoError(t, err)
				assert.Nil(t, testExitHandler.LastError)
			}
		})
	}
}

func TestExecuteDescribeWorkflows(t *testing.T) {
	// Setup test environment
	tmpDir, cleanup := setupTestWorkflowEnvironment(t)
	defer cleanup()

	workflowsDir := filepath.Join(tmpDir, "stacks", "workflows")

	// Create test workflow files
	workflow1Content := `
workflows:
  test-workflow-1:
    description: "Test workflow 1"
    steps:
      - name: "step1"
        type: "shell"
        command: "echo 'Step 1'"
`
	createTestWorkflowFile(t, workflowsDir, "workflow1.yaml", workflow1Content)

	workflow2Content := `
workflows:
  test-workflow-2:
    description: "Test workflow 2"
    steps:
      - name: "step1"
        type: "shell"
        command: "echo 'Step 1'"
`
	createTestWorkflowFile(t, workflowsDir, "workflow2.yaml", workflow2Content)

	// Initialize Atmos config
	config := initTestConfig(t)

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
