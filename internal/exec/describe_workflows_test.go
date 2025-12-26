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

func TestFindWorkflowAcrossFiles(t *testing.T) {
	tmpDir := setupTestWorkflowEnvironment(t)
	t.Setenv("ATMOS_CLI_CONFIG_PATH", tmpDir)

	workflowsDir := filepath.Join(tmpDir, "stacks", "workflows")

	// Create workflow files with duplicate workflow names.
	workflow1Content := `
workflows:
  deploy:
    description: "Deploy infrastructure from file 1"
    steps:
      - name: "step1"
        type: "shell"
        command: "echo 'Deploying from file 1'"
  test:
    description: "Run tests"
    steps:
      - name: "test"
        type: "shell"
        command: "echo 'Testing'"
`
	createTestWorkflowFile(t, workflowsDir, "infrastructure.yaml", workflow1Content)

	workflow2Content := `
workflows:
  deploy:
    description: "Deploy infrastructure from file 2"
    steps:
      - name: "step1"
        type: "shell"
        command: "echo 'Deploying from file 2'"
  cleanup:
    description: "Cleanup resources"
    steps:
      - name: "cleanup"
        type: "shell"
        command: "echo 'Cleaning up'"
`
	createTestWorkflowFile(t, workflowsDir, "maintenance.yaml", workflow2Content)

	config := initTestConfig(t)
	config.BasePath = tmpDir
	config.Workflows.BasePath = "stacks/workflows"

	tests := []struct {
		name              string
		workflowName      string
		wantErr           bool
		expectedMatches   int
		checkDescriptions bool
	}{
		{
			name:              "find workflow with multiple matches",
			workflowName:      "deploy",
			wantErr:           false,
			expectedMatches:   2,
			checkDescriptions: true,
		},
		{
			name:            "find workflow with single match",
			workflowName:    "test",
			wantErr:         false,
			expectedMatches: 1,
		},
		{
			name:            "workflow not found",
			workflowName:    "nonexistent",
			wantErr:         false,
			expectedMatches: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matches, err := findWorkflowAcrossFiles(tt.workflowName, &config)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Len(t, matches, tt.expectedMatches)

				if tt.checkDescriptions && len(matches) > 0 {
					// Verify descriptions are populated.
					for _, match := range matches {
						assert.NotEmpty(t, match.Description)
						assert.Contains(t, match.Description, "Deploy infrastructure")
					}
				}

				// Verify all matches have the correct workflow name.
				for _, match := range matches {
					assert.Equal(t, tt.workflowName, match.Name)
					assert.NotEmpty(t, match.File)
				}
			}
		})
	}
}

func TestFindWorkflowAcrossFiles_ExecuteDescribeWorkflowsError(t *testing.T) {
	// Config with invalid workflows base path.
	config := schema.AtmosConfiguration{
		Workflows: schema.Workflows{
			BasePath: "",
		},
	}

	matches, err := findWorkflowAcrossFiles("deploy", &config)

	assert.Error(t, err)
	assert.Nil(t, matches)
	assert.ErrorIs(t, err, errUtils.ErrWorkflowBasePathNotConfigured)
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

	// Should continue processing and return valid workflows despite invalid file.
	listResult, _, _, err := ExecuteDescribeWorkflows(config)

	// Should not error - invalid files are logged and skipped.
	assert.NoError(t, err)
	// Should still find the valid workflow.
	assert.Len(t, listResult, 1)
	assert.Equal(t, "deploy", listResult[0].Workflow)
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

	// Should continue processing and return valid workflows.
	listResult, _, _, err := ExecuteDescribeWorkflows(config)

	// Should not error - files without workflows key are logged and skipped.
	assert.NoError(t, err)
	// Should still find the valid workflow.
	assert.Len(t, listResult, 1)
	assert.Equal(t, "deploy", listResult[0].Workflow)
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
