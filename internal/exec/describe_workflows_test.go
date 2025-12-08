package exec

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

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
