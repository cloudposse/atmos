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
	"github.com/cloudposse/atmos/pkg/utils"
)

func TestValidateFormat(t *testing.T) {
	tests := []struct {
		name    string
		format  string
		wantErr bool
	}{
		{
			name:    "empty format",
			format:  "",
			wantErr: false,
		},
		{
			name:    "valid table format",
			format:  "table",
			wantErr: false,
		},
		{
			name:    "valid json format",
			format:  "json",
			wantErr: false,
		},
		{
			name:    "valid csv format",
			format:  "csv",
			wantErr: false,
		},
		{
			name:    "valid yaml format",
			format:  "yaml",
			wantErr: false,
		},
		{
			name:    "invalid format",
			format:  "invalid",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateFormat(tt.format)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestListWorkflows(t *testing.T) {
	// Create a temporary directory for test files
	tmpDir := t.TempDir()

	// Create workflow directory structure
	workflowsDir := filepath.Join(tmpDir, "stacks", "workflows")
	err := os.MkdirAll(workflowsDir, 0o755)
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

	// Create an empty workflow file
	emptyWorkflowFile := filepath.Join(workflowsDir, "empty.yaml")
	emptyWorkflow := schema.WorkflowManifest{
		Name:      "empty",
		Workflows: schema.WorkflowConfig{},
	}
	emptyWorkflowBytes, err := yaml.Marshal(emptyWorkflow)
	require.NoError(t, err)
	err = os.WriteFile(emptyWorkflowFile, emptyWorkflowBytes, 0o644)
	require.NoError(t, err)

	// Create a networking workflow file
	networkingWorkflowFile := filepath.Join(workflowsDir, "networking.yaml")
	networkingWorkflow := schema.WorkflowManifest{
		Name: "Networking",
		Workflows: schema.WorkflowConfig{
			"plan-all-vpc": schema.WorkflowDefinition{
				Description: "Run terraform plan on all vpc components",
				Steps: []schema.WorkflowStep{
					{Command: "terraform plan vpc -s test", Type: "shell"},
				},
			},
		},
	}
	networkingWorkflowBytes, err := yaml.Marshal(networkingWorkflow)
	require.NoError(t, err)
	err = os.WriteFile(networkingWorkflowFile, networkingWorkflowBytes, 0o644)
	require.NoError(t, err)

	// Create a validation workflow file
	validationWorkflowFile := filepath.Join(workflowsDir, "validation.yaml")
	validationWorkflow := schema.WorkflowManifest{
		Name: "Validation",
		Workflows: schema.WorkflowConfig{
			"validate-all": schema.WorkflowDefinition{
				Description: "Validate all components",
				Steps: []schema.WorkflowStep{
					{Command: "validate component vpc", Type: "shell"},
				},
			},
		},
	}
	validationWorkflowBytes, err := yaml.Marshal(validationWorkflow)
	require.NoError(t, err)
	err = os.WriteFile(validationWorkflowFile, validationWorkflowBytes, 0o644)
	require.NoError(t, err)

	// Change to the temporary directory for testing
	t.Chdir(tmpDir)

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
			name:     "discover all workflows",
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
				"Networking", "plan-all-vpc", "Run terraform plan on all vpc components",
				"Validation", "validate-all", "Validate all components",
			},
		},
		{
			name:     "empty workflows",
			fileFlag: "stacks/workflows/empty.yaml",
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
			name:     "json format with multiple workflows",
			fileFlag: "",
			config:   schema.ListConfig{},
			format:   "json",
			wantErr:  false,
			validate: func(t *testing.T, output string) {
				var workflows []map[string]interface{}
				err := json.Unmarshal([]byte(output), &workflows)
				assert.NoError(t, err)
				assert.GreaterOrEqual(t, len(workflows), 2)

				// Find and validate networking workflow
				var foundNetworking bool
				var foundValidation bool
				for _, w := range workflows {
					if w["file"] == "Networking" && w["name"] == "plan-all-vpc" {
						foundNetworking = true
						assert.Equal(t, "Run terraform plan on all vpc components", w["description"])
					}
					if w["file"] == "Validation" && w["name"] == "validate-all" {
						foundValidation = true
						assert.Equal(t, "Validate all components", w["description"])
					}
				}
				assert.True(t, foundNetworking, "Networking workflow not found")
				assert.True(t, foundValidation, "Validation workflow not found")
			},
		},
		{
			name:      "csv format with multiple workflows",
			fileFlag:  "",
			config:    schema.ListConfig{},
			format:    "csv",
			delimiter: ",",
			wantErr:   false,
			validate: func(t *testing.T, output string) {
				lines := strings.Split(strings.TrimSpace(output), utils.GetLineEnding())
				assert.GreaterOrEqual(t, len(lines), 3) // Header + at least 2 workflows
				assert.Equal(t, "File,Workflow,Description", lines[0])

				var foundNetworking bool
				var foundValidation bool
				for _, line := range lines[1:] {
					fields := strings.Split(line, ",")
					if len(fields) == 3 {
						if fields[0] == "Networking" && fields[1] == "plan-all-vpc" {
							foundNetworking = true
							assert.Equal(t, "Run terraform plan on all vpc components", fields[2])
						}
						if fields[0] == "Validation" && fields[1] == "validate-all" {
							foundValidation = true
							assert.Equal(t, "Validate all components", fields[2])
						}
					}
				}
				assert.True(t, foundNetworking, "Networking workflow not found")
				assert.True(t, foundValidation, "Validation workflow not found")
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
		})
	}
}

func TestListWorkflowsWithFile(t *testing.T) {
	// Create a temporary directory for test files
	tmpDir := t.TempDir()

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
	err = os.WriteFile(testWorkflowFile, testWorkflowBytes, 0o644)
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
				lines := strings.Split(strings.TrimSpace(output), utils.GetLineEnding())
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
