package list

import (
	"encoding/json"
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
	// Create a temporary directory for test files
	tmpDir, err := os.MkdirTemp("", "workflow_test")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	// Create an empty workflow file
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
		format      string
		delimiter   string
		wantErr     bool
		contains    []string
		notContains []string
		validate    func(t *testing.T, output string)
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
			format:    "",
			delimiter: "\t",
			wantErr:   false,
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
			format:    "",
			delimiter: "\t",
			wantErr:   false,
			contains:  []string{"No workflows found"},
		},
		{
			name:        "invalid file path",
			fileFlag:    "/invalid/path/workflows.yaml",
			config:      schema.ListConfig{},
			format:      "",
			delimiter:   "\t",
			wantErr:     true,
			notContains: []string{"File", "Workflow", "Description"},
		},
		{
			name:     "json format",
			fileFlag: "",
			config:   schema.ListConfig{},
			format:   "json",
			wantErr:  false,
			validate: func(t *testing.T, output string) {
				var workflows []map[string]interface{}
				err := json.Unmarshal([]byte(output), &workflows)
				assert.NoError(t, err)
				assert.Len(t, workflows, 1)
				assert.Equal(t, "example", workflows[0]["file"])
				assert.Equal(t, "test-1", workflows[0]["name"])
				assert.Equal(t, "Test workflow", workflows[0]["description"])
			},
		},
		{
			name:      "csv format with custom delimiter",
			fileFlag:  "",
			config:    schema.ListConfig{},
			format:    "csv",
			delimiter: ",",
			wantErr:   false,
			validate: func(t *testing.T, output string) {
				lines := strings.Split(strings.TrimSpace(output), "\n")
				assert.Len(t, lines, 2) // Header + 1 workflow
				assert.Equal(t, "File,Workflow,Description", lines[0])
				assert.Equal(t, "example,test-1,Test workflow", lines[1])
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output, err := FilterAndListWorkflows(tt.fileFlag, tt.config, tt.format, tt.delimiter)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)

			// Run custom validation if provided
			if tt.validate != nil {
				tt.validate(t, output)
				return
			}

			// Verify expected content is present
			for _, expected := range tt.contains {
				assert.Contains(t, output, expected)
			}

			// Verify unexpected content is not present
			for _, unexpected := range tt.notContains {
				assert.NotContains(t, output, unexpected)
			}

			// For the happy path, verify the order of headers
			if tt.name == "happy path - default config" {
				assert.True(t, strings.Index(output, "File") < strings.Index(output, "Workflow"))
				assert.True(t, strings.Index(output, "Workflow") < strings.Index(output, "Description"))
			}
		})
	}
}

func TestListWorkflowsWithFile(t *testing.T) {
	// Create a temporary directory for test files
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

	tests := []struct {
		name      string
		format    string
		delimiter string
		validate  func(t *testing.T, output string)
	}{
		{
			name:      "default format",
			format:    "",
			delimiter: "\t",
			validate: func(t *testing.T, output string) {
				assert.Contains(t, output, "File")
				assert.Contains(t, output, "Workflow")
				assert.Contains(t, output, "Description")
				assert.Contains(t, output, "example")
				assert.Contains(t, output, "test-1")
				assert.Contains(t, output, "Test workflow")
			},
		},
		{
			name:      "json format",
			format:    "json",
			delimiter: "\t",
			validate: func(t *testing.T, output string) {
				var workflows []map[string]interface{}
				err := json.Unmarshal([]byte(output), &workflows)
				assert.NoError(t, err)
				assert.Len(t, workflows, 1)
				assert.Equal(t, "example", workflows[0]["file"])
				assert.Equal(t, "test-1", workflows[0]["name"])
				assert.Equal(t, "Test workflow", workflows[0]["description"])
			},
		},
		{
			name:      "csv format with custom delimiter",
			format:    "csv",
			delimiter: ",",
			validate: func(t *testing.T, output string) {
				lines := strings.Split(strings.TrimSpace(output), "\n")
				assert.Len(t, lines, 2) // Header + 1 workflow
				assert.Equal(t, "File,Workflow,Description", lines[0])
				assert.Equal(t, "example,test-1,Test workflow", lines[1])
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output, err := FilterAndListWorkflows(testWorkflowFile, listConfig, tt.format, tt.delimiter)
			assert.NoError(t, err)
			tt.validate(t, output)
		})
	}
}
