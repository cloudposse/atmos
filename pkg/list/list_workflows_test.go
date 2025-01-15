package list

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/cloudposse/atmos/pkg/schema"
)

func TestListWorkflows(t *testing.T) {
	listConfig := schema.ListConfig{
		Columns: []schema.ListColumnConfig{
			{Name: "File", Value: "{{ .workflow_file }}"},
			{Name: "Workflow", Value: "{{ .workflow_name }}"},
			{Name: "Description", Value: "{{ .workflow_description }}"},
		},
	}

	output, err := FilterAndListWorkflows("", listConfig)
	assert.Nil(t, err)
	assert.Contains(t, output, "File")
	assert.Contains(t, output, "Workflow")
	assert.Contains(t, output, "Description")
	assert.Contains(t, output, "example")
	assert.Contains(t, output, "test-1")
	assert.Contains(t, output, "Test workflow")
}

func TestListWorkflowsWithFile(t *testing.T) {
	listConfig := schema.ListConfig{
		Columns: []schema.ListColumnConfig{
			{Name: "File", Value: "{{ .workflow_file }}"},
			{Name: "Workflow", Value: "{{ .workflow_name }}"},
			{Name: "Description", Value: "{{ .workflow_description }}"},
		},
	}

	output, err := FilterAndListWorkflows("example", listConfig)
	assert.Nil(t, err)
	assert.Contains(t, output, "File")
	assert.Contains(t, output, "Workflow")
	assert.Contains(t, output, "Description")
	assert.Contains(t, output, "example")
	assert.Contains(t, output, "test-1")
	assert.Contains(t, output, "Test workflow")
}
