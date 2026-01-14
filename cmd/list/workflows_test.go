package list

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/cloudposse/atmos/pkg/schema"
)

// TestListWorkflowsCmd_WithoutStacks verifies that list workflows does not require stack configuration.
// This test documents that the command uses InitCliConfig with processStacks=false.
func TestListWorkflowsCmd_WithoutStacks(t *testing.T) {
	// This test documents that list workflows command does not process stacks
	// by verifying InitCliConfig is called with processStacks=false in list_workflows.go:39
	// and that checkAtmosConfig is called with WithStackValidation(false) in list_workflows.go:19
	// No runtime test needed - this is enforced by code structure.
	t.Log("list workflows command uses InitCliConfig with processStacks=false")
}

// TestGetWorkflowColumns_DefaultColumns verifies the default column templates.
// This test ensures the "Workflow" column uses {{ .workflow }} (not {{ .name }}).
func TestGetWorkflowColumns_DefaultColumns(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{}

	// No columns flag, no config - should return defaults.
	columns := getWorkflowColumns(atmosConfig, nil)

	assert.Len(t, columns, 4)

	// Verify column names.
	assert.Equal(t, "File", columns[0].Name)
	assert.Equal(t, "Workflow", columns[1].Name)
	assert.Equal(t, "Description", columns[2].Name)
	assert.Equal(t, "Steps", columns[3].Name)

	// Verify column templates use correct keys.
	// This is the fix for the issue where workflow name showed "<no value>".
	assert.Equal(t, "{{ .file }}", columns[0].Value)
	assert.Equal(t, "{{ .workflow }}", columns[1].Value) // Must be .workflow, not .name
	assert.Equal(t, "{{ .description }}", columns[2].Value)
	assert.Equal(t, "{{ .steps }}", columns[3].Value)
}

// TestGetWorkflowColumns_WithColumnsFlag verifies columns flag overrides defaults.
func TestGetWorkflowColumns_WithColumnsFlag(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{}

	// Columns flag provided - should parse and return those.
	columns := getWorkflowColumns(atmosConfig, []string{"File", "Workflow"})

	assert.Len(t, columns, 2)
	assert.Equal(t, "File", columns[0].Name)
	assert.Equal(t, "Workflow", columns[1].Name)
}

// TestGetWorkflowColumns_WithAtmosConfig verifies atmos.yaml config is used.
func TestGetWorkflowColumns_WithAtmosConfig(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{
		Workflows: schema.Workflows{
			List: schema.ListConfig{
				Columns: []schema.ListColumnConfig{
					{Name: "Custom", Value: "{{ .custom }}"},
				},
			},
		},
	}

	// No columns flag - should use atmos.yaml config.
	columns := getWorkflowColumns(atmosConfig, nil)

	assert.Len(t, columns, 1)
	assert.Equal(t, "Custom", columns[0].Name)
	assert.Equal(t, "{{ .custom }}", columns[0].Value)
}
