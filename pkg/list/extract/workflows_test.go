package extract

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestExtractFromManifest(t *testing.T) {
	manifest := schema.WorkflowManifest{
		Name: "deploy-workflows",
		Workflows: map[string]schema.WorkflowDefinition{
			"deploy-all": {
				Description: "Deploy all components",
				Steps: []schema.WorkflowStep{
					{Name: "step1"},
					{Name: "step2"},
				},
			},
			"destroy-all": {
				Description: "Destroy all components",
				Steps: []schema.WorkflowStep{
					{Name: "step1"},
				},
			},
		},
	}

	workflows := extractFromManifest(manifest)
	require.Len(t, workflows, 2)

	// Verify structure.
	for _, wf := range workflows {
		assert.Contains(t, wf, "file")
		assert.Contains(t, wf, "workflow")
		assert.Contains(t, wf, "description")
		assert.Contains(t, wf, "steps")
		assert.Equal(t, "deploy-workflows", wf["file"])
	}

	// Find deploy-all workflow.
	var deployAll map[string]any
	for _, wf := range workflows {
		if wf["workflow"] == "deploy-all" {
			deployAll = wf
			break
		}
	}

	require.NotNil(t, deployAll)
	assert.Equal(t, "deploy-all", deployAll["workflow"])
	assert.Equal(t, "Deploy all components", deployAll["description"])
	assert.Equal(t, 2, deployAll["steps"])
}

func TestExtractFromManifest_EmptyWorkflows(t *testing.T) {
	manifest := schema.WorkflowManifest{
		Name:      "empty-workflows",
		Workflows: nil,
	}

	workflows := extractFromManifest(manifest)
	assert.Empty(t, workflows)
}

func TestExtractFromManifest_NoDescription(t *testing.T) {
	manifest := schema.WorkflowManifest{
		Name: "test-workflows",
		Workflows: map[string]schema.WorkflowDefinition{
			"test": {
				Description: "",
				Steps:       []schema.WorkflowStep{},
			},
		},
	}

	workflows := extractFromManifest(manifest)
	require.Len(t, workflows, 1)

	assert.Equal(t, "", workflows[0]["description"])
	assert.Equal(t, 0, workflows[0]["steps"])
}

func TestExtractFromManifest_MultipleWorkflows(t *testing.T) {
	manifest := schema.WorkflowManifest{
		Name: "multi-workflows",
		Workflows: map[string]schema.WorkflowDefinition{
			"wf1": {Description: "Workflow 1", Steps: []schema.WorkflowStep{{Name: "s1"}}},
			"wf2": {Description: "Workflow 2", Steps: []schema.WorkflowStep{{Name: "s1"}, {Name: "s2"}}},
			"wf3": {Description: "Workflow 3", Steps: []schema.WorkflowStep{{Name: "s1"}, {Name: "s2"}, {Name: "s3"}}},
		},
	}

	workflows := extractFromManifest(manifest)
	assert.Len(t, workflows, 3)

	// Verify all have file field.
	for _, wf := range workflows {
		assert.Equal(t, "multi-workflows", wf["file"])
	}
}

// TestWorkflows_WithDirectory tests loading workflows from a directory.
func TestWorkflows_WithDirectory(t *testing.T) {
	// Create temporary directory with workflow files.
	tmpDir := t.TempDir()
	workflowsDir := filepath.Join(tmpDir, "workflows")
	err := os.MkdirAll(workflowsDir, 0o755)
	require.NoError(t, err)

	// Create workflow file 1.
	workflow1 := `
workflows:
  deploy:
    description: Deploy infrastructure
    steps:
      - name: step1
      - name: step2
`
	err = os.WriteFile(filepath.Join(workflowsDir, "deploy.yaml"), []byte(workflow1), 0o644)
	require.NoError(t, err)

	// Create workflow file 2.
	workflow2 := `
workflows:
  destroy:
    description: Destroy infrastructure
    steps:
      - name: step1
`
	err = os.WriteFile(filepath.Join(workflowsDir, "destroy.yaml"), []byte(workflow2), 0o644)
	require.NoError(t, err)

	// Configure atmos to use the test workflows directory.
	atmosConfig := &schema.AtmosConfiguration{
		BasePath: tmpDir,
		Workflows: schema.Workflows{
			BasePath: "workflows",
		},
	}

	// Test loading all workflows.
	workflows, err := Workflows(atmosConfig, "")
	require.NoError(t, err)
	assert.Len(t, workflows, 2)

	// Verify workflow structure.
	for _, wf := range workflows {
		assert.Contains(t, wf, "file")
		assert.Contains(t, wf, "workflow")
		assert.Contains(t, wf, "description")
		assert.Contains(t, wf, "steps")
	}
}

// TestWorkflows_WithFileFilter tests loading a specific workflow file.
func TestWorkflows_WithFileFilter(t *testing.T) {
	// Create temporary directory with workflow file.
	tmpDir := t.TempDir()
	workflowFile := filepath.Join(tmpDir, "deploy.yaml")

	workflowContent := `
workflows:
  deploy-all:
    description: Deploy all components
    steps:
      - name: step1
      - name: step2
`
	err := os.WriteFile(workflowFile, []byte(workflowContent), 0o644)
	require.NoError(t, err)

	atmosConfig := &schema.AtmosConfiguration{
		BasePath: tmpDir,
	}

	// Test loading specific file.
	workflows, err := Workflows(atmosConfig, workflowFile)
	require.NoError(t, err)
	assert.Len(t, workflows, 1)

	assert.Equal(t, workflowFile, workflows[0]["file"])
	assert.Equal(t, "deploy-all", workflows[0]["workflow"])
	assert.Equal(t, "Deploy all components", workflows[0]["description"])
	assert.Equal(t, 2, workflows[0]["steps"])
}

// TestWorkflows_InvalidFileExtension tests error handling for non-YAML files.
func TestWorkflows_InvalidFileExtension(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{
		BasePath: "/tmp",
	}

	workflows, err := Workflows(atmosConfig, "test.txt")
	assert.Error(t, err)
	assert.Nil(t, workflows)
	assert.ErrorIs(t, err, errUtils.ErrParseFile)
	assert.Contains(t, err.Error(), "invalid workflow file extension")
}

// TestWorkflows_FileNotFound tests error handling for missing files.
func TestWorkflows_FileNotFound(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{
		BasePath: "/tmp",
	}

	workflows, err := Workflows(atmosConfig, "/nonexistent/file.yaml")
	assert.Error(t, err)
	assert.Nil(t, workflows)
	assert.ErrorIs(t, err, errUtils.ErrParseFile)
	assert.Contains(t, err.Error(), "workflow file not found")
}

// TestWorkflows_InvalidYAML tests error handling for malformed YAML.
func TestWorkflows_InvalidYAML(t *testing.T) {
	tmpDir := t.TempDir()
	workflowFile := filepath.Join(tmpDir, "invalid.yaml")

	// Write invalid YAML.
	err := os.WriteFile(workflowFile, []byte("invalid: yaml: content: ["), 0o644)
	require.NoError(t, err)

	atmosConfig := &schema.AtmosConfiguration{
		BasePath: tmpDir,
	}

	workflows, err := Workflows(atmosConfig, workflowFile)
	assert.Error(t, err)
	assert.Nil(t, workflows)
	assert.ErrorIs(t, err, errUtils.ErrParseFile)
}

// TestWorkflows_DirectoryNotFound tests error handling for missing workflow directory.
func TestWorkflows_DirectoryNotFound(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{
		BasePath: "/tmp",
		Workflows: schema.Workflows{
			BasePath: "nonexistent-workflows",
		},
	}

	workflows, err := Workflows(atmosConfig, "")
	assert.Error(t, err)
	assert.Nil(t, workflows)
	assert.ErrorIs(t, err, errUtils.ErrWorkflowDirectoryDoesNotExist)
}

// TestWorkflows_EmptyDirectory tests loading from empty directory.
func TestWorkflows_EmptyDirectory(t *testing.T) {
	tmpDir := t.TempDir()
	workflowsDir := filepath.Join(tmpDir, "workflows")
	err := os.MkdirAll(workflowsDir, 0o755)
	require.NoError(t, err)

	atmosConfig := &schema.AtmosConfiguration{
		BasePath: tmpDir,
		Workflows: schema.Workflows{
			BasePath: "workflows",
		},
	}

	workflows, err := Workflows(atmosConfig, "")
	require.NoError(t, err)
	assert.Empty(t, workflows)
}

// TestWorkflows_SkipsInvalidFiles tests that invalid files in directory are skipped.
func TestWorkflows_SkipsInvalidFiles(t *testing.T) {
	tmpDir := t.TempDir()
	workflowsDir := filepath.Join(tmpDir, "workflows")
	err := os.MkdirAll(workflowsDir, 0o755)
	require.NoError(t, err)

	// Create valid workflow.
	validWorkflow := `
workflows:
  deploy:
    description: Deploy
    steps:
      - name: step1
`
	err = os.WriteFile(filepath.Join(workflowsDir, "valid.yaml"), []byte(validWorkflow), 0o644)
	require.NoError(t, err)

	// Create invalid workflow (malformed YAML).
	err = os.WriteFile(filepath.Join(workflowsDir, "invalid.yaml"), []byte("invalid: yaml: ["), 0o644)
	require.NoError(t, err)

	atmosConfig := &schema.AtmosConfiguration{
		BasePath: tmpDir,
		Workflows: schema.Workflows{
			BasePath: "workflows",
		},
	}

	// Should load only the valid workflow.
	workflows, err := Workflows(atmosConfig, "")
	require.NoError(t, err)
	assert.Len(t, workflows, 1)
	assert.Equal(t, "valid.yaml", workflows[0]["file"])
}

// TestWorkflows_AbsoluteWorkflowsPath tests loading with absolute workflows path.
func TestWorkflows_AbsoluteWorkflowsPath(t *testing.T) {
	tmpDir := t.TempDir()
	workflowsDir := filepath.Join(tmpDir, "workflows")
	err := os.MkdirAll(workflowsDir, 0o755)
	require.NoError(t, err)

	workflow := `
workflows:
  test:
    description: Test workflow
    steps:
      - name: step1
`
	err = os.WriteFile(filepath.Join(workflowsDir, "test.yaml"), []byte(workflow), 0o644)
	require.NoError(t, err)

	// Use absolute path for workflows.
	atmosConfig := &schema.AtmosConfiguration{
		BasePath: tmpDir,
		Workflows: schema.Workflows{
			BasePath: workflowsDir, // Absolute path.
		},
	}

	workflows, err := Workflows(atmosConfig, "")
	require.NoError(t, err)
	assert.Len(t, workflows, 1)
	assert.Equal(t, "test.yaml", workflows[0]["file"])
}
