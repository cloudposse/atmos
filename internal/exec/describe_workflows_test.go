package exec

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
)

// setupTestWorkflowEnvironment creates a temporary test environment with the necessary directory structure and configuration files.
// It returns the temporary directory path.
func setupTestWorkflowEnvironment(t *testing.T) string {
	tmpDir := t.TempDir()

	workflowsDir := filepath.Join(tmpDir, "stacks", "workflows")
	err := os.MkdirAll(workflowsDir, 0o755)
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

	return tmpDir
}

// createTestWorkflowFile creates a workflow file in the specified directory with the given content.
func createTestWorkflowFile(t *testing.T, dir string, filename string, content string) {
	workflowPath := filepath.Join(dir, filename)
	err := os.WriteFile(workflowPath, []byte(content), 0o644)
	require.NoError(t, err)
}

// initTestConfig initializes the Atmos configuration for testing.
func initTestConfig(t *testing.T) schema.AtmosConfiguration {
	config, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, false)
	require.NoError(t, err)
	return config
}

func TestExecuteDescribeWorkflows(t *testing.T) {
	// Setup test environment
	tmpDir := setupTestWorkflowEnvironment(t)
	t.Setenv("ATMOS_CLI_CONFIG_PATH", tmpDir)

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
		wantSentinel  error
		errContains   string
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
			wantErr:      true,
			wantSentinel: errUtils.ErrWorkflowBasePathNotConfigured,
		},
		{
			name: "nonexistent workflows directory",
			config: schema.AtmosConfiguration{
				Workflows: schema.Workflows{
					BasePath: "nonexistent",
				},
			},
			wantErr:     true,
			errContains: "the workflow directory 'nonexistent' does not exist",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			listResult, mapResult, allResult, err := ExecuteDescribeWorkflows(tt.config)

			if tt.wantErr {
				assert.Error(t, err)
				if tt.wantSentinel != nil {
					assert.ErrorIs(t, err, tt.wantSentinel)
				}
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
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

func TestExecuteDescribeWorkflows_InvalidYAMLFile(t *testing.T) {
	tmpDir := setupTestWorkflowEnvironment(t)
	t.Setenv("ATMOS_CLI_CONFIG_PATH", tmpDir)

	workflowsDir := filepath.Join(tmpDir, "stacks", "workflows")

	// Create a valid workflow file.
	validWorkflow := `
workflows:
  deploy:
    description: "Valid workflow"
    steps:
      - name: "step1"
        type: "shell"
        command: "echo 'valid'"
`
	createTestWorkflowFile(t, workflowsDir, "valid.yaml", validWorkflow)

	// Create an invalid YAML file.
	invalidYAML := `this is not valid yaml: [[[`
	createTestWorkflowFile(t, workflowsDir, "invalid.yaml", invalidYAML)

	config := initTestConfig(t)
	config.BasePath = tmpDir
	config.Workflows.BasePath = "stacks/workflows"

	// The function returns an error when encountering invalid YAML files.
	_, _, _, err := ExecuteDescribeWorkflows(config)

	// Should error on invalid YAML file.
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid workflow manifest")
}

func TestExecuteDescribeWorkflows_FileWithoutWorkflowsKey(t *testing.T) {
	tmpDir := setupTestWorkflowEnvironment(t)
	t.Setenv("ATMOS_CLI_CONFIG_PATH", tmpDir)

	workflowsDir := filepath.Join(tmpDir, "stacks", "workflows")

	// Create a valid workflow file.
	validWorkflow := `
workflows:
  deploy:
    description: "Valid workflow"
    steps:
      - name: "step1"
        type: "shell"
        command: "echo 'valid'"
`
	createTestWorkflowFile(t, workflowsDir, "valid.yaml", validWorkflow)

	// Create a file without workflows key.
	noWorkflowsKey := `
some_other_key:
  value: "not a workflow file"
`
	createTestWorkflowFile(t, workflowsDir, "not-workflows.yaml", noWorkflowsKey)

	config := initTestConfig(t)
	config.BasePath = tmpDir
	config.Workflows.BasePath = "stacks/workflows"

	// The function returns an error for files without the workflows key.
	_, _, _, err := ExecuteDescribeWorkflows(config)

	// Should error on file without workflows key.
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid workflow manifest")
}

func TestExecuteDescribeWorkflows_EmptyWorkflowsDirectory(t *testing.T) {
	tmpDir := setupTestWorkflowEnvironment(t)
	t.Setenv("ATMOS_CLI_CONFIG_PATH", tmpDir)

	config := initTestConfig(t)
	config.BasePath = tmpDir
	config.Workflows.BasePath = "stacks/workflows"

	// Empty workflows directory.
	listResult, mapResult, allResult, err := ExecuteDescribeWorkflows(config)

	assert.NoError(t, err)
	assert.Len(t, listResult, 0)
	assert.Len(t, mapResult, 0)
	assert.Len(t, allResult, 0)
}
