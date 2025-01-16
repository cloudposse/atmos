package list

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v2"

	"github.com/cloudposse/atmos/pkg/schema"
)

func TestListWorkflows(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "workflow_test")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	emptyWorkflowFile := filepath.Join(tmpDir, "empty.yaml")
	emptyWorkflow := schema.WorkflowManifest{
		Name:      "empty",
		Workflows: schema.WorkflowConfig{},
	}
	emptyWorkflowBytes, err := yaml.Marshal(emptyWorkflow)
	require.NoError(t, err)
	err = os.WriteFile(emptyWorkflowFile, emptyWorkflowBytes, 0644)
	require.NoError(t, err)

	tests := []struct {
		name        string
		fileFlag    string
		config      schema.ListConfig
		wantErr     bool
		contains    []string
		notContains []string
	}{
		{
			name:     "happy path - default config",
			fileFlag: "",
			config: schema.ListConfig{
				Columns: []schema.ListColumnConfig{
					{Name: "File", Value: "{{ .workflow_file }}"},
					{Name: "Workflow", Value: "{{ .workflow_name }}"},
					{Name: "Description", Value: "{{ .workflow_description }}"},
				},
			},
			wantErr: false,
			contains: []string{
				"File", "Workflow", "Description",
				"example", "test-1", "Test workflow",
			},
		},
		{
			name:     "empty workflows",
			fileFlag: emptyWorkflowFile,
			config: schema.ListConfig{
				Columns: []schema.ListColumnConfig{
					{Name: "File", Value: "{{ .workflow_file }}"},
					{Name: "Workflow", Value: "{{ .workflow_name }}"},
					{Name: "Description", Value: "{{ .workflow_description }}"},
				},
			},
			wantErr:  false,
			contains: []string{"No workflows found"},
		},
		{
			name:        "invalid file path",
			fileFlag:    "/invalid/path/workflows.yaml",
			config:      schema.ListConfig{},
			wantErr:     true,
			notContains: []string{"File", "Workflow", "Description"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output, err := FilterAndListWorkflows(tt.fileFlag, tt.config)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)

			// Verify expected content is present
			for _, expected := range tt.contains {
				assert.Contains(t, output, expected)
			}

			// Verify unexpected content is not present
			for _, unexpected := range tt.notContains {
				assert.NotContains(t, output, unexpected)
			}

			// Verify the order of headers
			if tt.name == "happy path - default config" {
				assert.True(t, strings.Index(output, "File") < strings.Index(output, "Workflow"))
				assert.True(t, strings.Index(output, "Workflow") < strings.Index(output, "Description"))
			}
		})
	}
}

func TestListWorkflowsWithFile(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "workflow_test")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	// Create a test workflow file
	testWorkflowFile := filepath.Join(tmpDir, "test.yaml")
	testWorkflow := schema.WorkflowManifest{
		Name: "example",
		Workflows: schema.WorkflowConfig{
			"test-1": schema.WorkflowDefinition{
				Description: "Test workflow",
				Steps: []schema.WorkflowStep{
					{Command: "echo Command 1", Name: "step1", Type: "shell"},
				},
			},
		},
	}
	testWorkflowBytes, err := yaml.Marshal(testWorkflow)
	require.NoError(t, err)
	err = os.WriteFile(testWorkflowFile, testWorkflowBytes, 0644)
	require.NoError(t, err)

	listConfig := schema.ListConfig{
		Columns: []schema.ListColumnConfig{
			{Name: "File", Value: "{{ .workflow_file }}"},
			{Name: "Workflow", Value: "{{ .workflow_name }}"},
			{Name: "Description", Value: "{{ .workflow_description }}"},
		},
	}

	output, err := FilterAndListWorkflows(testWorkflowFile, listConfig)
	assert.NoError(t, err)
	assert.Contains(t, output, "File")
	assert.Contains(t, output, "Workflow")
	assert.Contains(t, output, "Description")
	assert.Contains(t, output, "example")
	assert.Contains(t, output, "test-1")
	assert.Contains(t, output, "Test workflow")
}
