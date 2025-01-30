package list

import (
	"testing"

	"github.com/stretchr/testify/assert"

	e "github.com/cloudposse/atmos/internal/exec"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

const (
	testComponent = "infra/vpc"
)

func TestListStacks(t *testing.T) {
	configAndStacksInfo := schema.ConfigAndStacksInfo{}

	atmosConfig, err := cfg.InitCliConfig(configAndStacksInfo, true)
	assert.Nil(t, err)

	stacksMap, err := e.ExecuteDescribeStacks(atmosConfig, "", nil, nil,
		nil, false, false, false)
	assert.Nil(t, err)

	// Create test list config
	listConfig := schema.ListConfig{
		Format: "",
		Columns: []schema.ListColumnConfig{
			{Name: "Stack", Value: "{{ .atmos_stack }}"},
			{Name: "File", Value: "{{ .atmos_stack_file }}"},
		},
	}

	output, err := FilterAndListStacks(stacksMap, "", listConfig, "", "\t")
	assert.Nil(t, err)
	dependentsYaml, err := u.ConvertToYAML(output)
	assert.NotEmpty(t, dependentsYaml)
}

func TestListStacksWithComponent(t *testing.T) {
	configAndStacksInfo := schema.ConfigAndStacksInfo{}

	atmosConfig, err := cfg.InitCliConfig(configAndStacksInfo, true)
	assert.Nil(t, err)
	component := testComponent

	stacksMap, err := e.ExecuteDescribeStacks(atmosConfig, component, nil, nil,
		nil, false, false, false)
	assert.Nil(t, err)

	// Create test list config
	listConfig := schema.ListConfig{
		Format: "",
		Columns: []schema.ListColumnConfig{
			{Name: "Stack", Value: "{{ .atmos_stack }}"},
			{Name: "File", Value: "{{ .atmos_stack_file }}"},
		},
	}

	output, err := FilterAndListStacks(stacksMap, component, listConfig, "", "\t")
	assert.Nil(t, err)
	dependentsYaml, err := u.ConvertToYAML(output)
	assert.Nil(t, err)

	// Verify the output structure
	assert.NotEmpty(t, dependentsYaml)
	// Verify that only stacks with the specified component are included
	assert.Contains(t, dependentsYaml, testComponent)
}

func TestFilterAndListStacks(t *testing.T) {
	// Mock context and config
	context := map[string]any{
		"components": map[string]any{},
		"stacks":     map[string]any{},
	}
	stacksBasePath := "examples/quick-start-simple/stacks"
	stackType := "deploy"
	component := ""

	tests := []struct {
		name     string
		config   schema.ListConfig
		expected []map[string]string
	}{
		{
			name:   "default columns",
			config: schema.ListConfig{},
			expected: []map[string]string{
				{
					"Stack": "dev",
					"File":  "examples/quick-start-simple/stacks/deploy/dev.yaml",
				},
				{
					"Stack": "prod",
					"File":  "examples/quick-start-simple/stacks/deploy/prod.yaml",
				},
				{
					"Stack": "staging",
					"File":  "examples/quick-start-simple/stacks/deploy/staging.yaml",
				},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result, err := FilterAndListStacks(context, stacksBasePath, test.config, stackType, component)
			assert.NoError(t, err)
			assert.Equal(t, test.expected, result)
		})
	}
}
