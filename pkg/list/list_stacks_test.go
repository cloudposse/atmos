package list

import (
	"strings"
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
		nil, false, false, false, false, nil)
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
		nil, false, false, false, false, nil)
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
	// Mock stacks map with actual configurations from examples/quick-start-simple
	stacksMap := map[string]any{
		"dev": map[string]any{
			"vars": map[string]any{
				"stage": "dev",
			},
			"atmos_stack_file": "examples/quick-start-simple/stacks/deploy/dev.yaml",
			"components": map[string]any{
				"terraform": map[string]any{
					"station": map[string]any{
						"vars": map[string]any{
							"location": "Stockholm",
							"lang":     "se",
						},
					},
				},
			},
		},
		"staging": map[string]any{
			"vars": map[string]any{
				"stage": "staging",
			},
			"atmos_stack_file": "examples/quick-start-simple/stacks/deploy/staging.yaml",
			"components": map[string]any{
				"terraform": map[string]any{
					"station": map[string]any{
						"vars": map[string]any{
							"location": "Los Angeles",
							"lang":     "en",
						},
					},
				},
			},
		},
		"prod": map[string]any{
			"vars": map[string]any{
				"stage": "prod",
			},
			"atmos_stack_file": "examples/quick-start-simple/stacks/deploy/prod.yaml",
			"components": map[string]any{
				"terraform": map[string]any{
					"station": map[string]any{
						"vars": map[string]any{
							"location": "Los Angeles",
							"lang":     "en",
						},
					},
				},
			},
		},
	}

	tests := []struct {
		name      string
		config    schema.ListConfig
		format    string
		delimiter string
		expected  []map[string]string
	}{
		{
			name:      "default columns",
			config:    schema.ListConfig{},
			format:    "",
			delimiter: "\t",
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
			output, err := FilterAndListStacks(stacksMap, "", test.config, test.format, test.delimiter)
			assert.NoError(t, err)

			// Parse the output into a slice of maps for comparison
			var result []map[string]string
			lines := strings.Split(strings.TrimSpace(output), u.GetLineEnding())
			if len(lines) > 1 { // Skip header row
				headers := strings.Split(lines[0], test.delimiter)
				for _, line := range lines[1:] {
					values := strings.Split(line, test.delimiter)
					row := make(map[string]string)
					for i, header := range headers {
						row[header] = values[i]
					}
					result = append(result, row)
				}
			}

			assert.Equal(t, test.expected, result)
		})
	}
}
