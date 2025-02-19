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

	output, err := FilterAndListStacks(stacksMap, "eks-blue/cluster", listConfig, "", "\t")
	assert.Nil(t, err)
	dependentsYaml, err := u.ConvertToYAML(output)
	assert.Nil(t, err)

	// Verify the output structure
	assert.NotEmpty(t, dependentsYaml)
	// Verify that only stacks with the specified component are included
	assert.Contains(t, dependentsYaml, "tenant1-uw1-test-1")
	assert.Contains(t, dependentsYaml, "tenant1-uw2-test-1")
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
						"settings": map[string]any{
							"backend_type":   "s3",
							"backend_region": "us-west-2",
							"component_type": "terraform",
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
						"settings": map[string]any{
							"backend_type":   "s3",
							"backend_region": "us-west-2",
							"component_type": "terraform",
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
						"settings": map[string]any{
							"backend_type":   "s3",
							"backend_region": "us-west-2",
							"component_type": "terraform",
						},
					},
				},
			},
		},
	}

	// Add a stack with special characters to test escaping
	stacksMap["test,special"] = map[string]any{
		"vars": map[string]any{
			"stage": "test,stage",
		},
		"atmos_stack_file": "test/stack.yaml",
		"components": map[string]any{
			"terraform": map[string]any{
				"station": map[string]any{
					"vars": map[string]any{
						"location": "Test,City",
						"lang":     "test\tlang",
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
		expected  string
	}{
		{
			name:      "default format for non-TTY",
			config:    schema.ListConfig{},
			format:    "",
			delimiter: "\t",
			expected:  "Stack,Stage,File\ndev,dev,examples/quick-start-simple/stacks/deploy/dev.yaml\nprod,prod,examples/quick-start-simple/stacks/deploy/prod.yaml\nstaging,staging,examples/quick-start-simple/stacks/deploy/staging.yaml\n\"test,special\",\"test,stage\",test/stack.yaml\n",
		},
		{
			name:      "csv format with default delimiter",
			config:    schema.ListConfig{},
			format:    "csv",
			delimiter: "\t", // Should be ignored for CSV
			expected:  "Stack,Stage,File\ndev,dev,examples/quick-start-simple/stacks/deploy/dev.yaml\nprod,prod,examples/quick-start-simple/stacks/deploy/prod.yaml\nstaging,staging,examples/quick-start-simple/stacks/deploy/staging.yaml\n\"test,special\",\"test,stage\",test/stack.yaml\n",
		},
		{
			name:      "tsv format",
			config:    schema.ListConfig{},
			format:    "tsv",
			delimiter: "", // Should default to tab
			expected:  "Stack\tStage\tFile\ndev\tdev\texamples/quick-start-simple/stacks/deploy/dev.yaml\nprod\tprod\texamples/quick-start-simple/stacks/deploy/prod.yaml\nstaging\tstaging\texamples/quick-start-simple/stacks/deploy/staging.yaml\ntest,special\ttest,stage\ttest/stack.yaml\n",
		},
		{
			name: "custom columns with csv format",
			config: schema.ListConfig{
				Columns: []schema.ListColumnConfig{
					{Name: "Stack", Value: "{{ .atmos_stack }}"},
					{Name: "Stage", Value: "{{ index .vars \"stage\" }}"},
				},
			},
			format:    "csv",
			delimiter: ",",
			expected:  "Stack,Stage\ndev,dev\nprod,prod\nstaging,staging\n\"test,special\",\"test,stage\"\n",
		},
		{
			name: "access to settings and nested properties",
			config: schema.ListConfig{
				Columns: []schema.ListColumnConfig{
					{Name: "Stack", Value: "{{ .atmos_stack }}"},
					{Name: "Backend", Value: "{{ if (index .components \"terraform\" \"station\" \"settings\") }}{{ index .components \"terraform\" \"station\" \"settings\" \"backend_type\" }}{{ end }}"},
					{Name: "Region", Value: "{{ if (index .components \"terraform\" \"station\" \"settings\") }}{{ index .components \"terraform\" \"station\" \"settings\" \"backend_region\" }}{{ end }}"},
					{Name: "Type", Value: "{{ if (index .components \"terraform\" \"station\" \"settings\") }}{{ index .components \"terraform\" \"station\" \"settings\" \"component_type\" }}{{ end }}"},
				},
			},
			format:    "csv",
			delimiter: ",",
			expected:  "Stack,Backend,Region,Type\ndev,s3,us-west-2,terraform\nprod,s3,us-west-2,terraform\nstaging,s3,us-west-2,terraform\n\"test,special\",,,\n",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			output, err := FilterAndListStacks(stacksMap, "", test.config, test.format, test.delimiter)
			assert.NoError(t, err)

			// Normalize line endings for comparison
			normalizedOutput := strings.ReplaceAll(output, "\r\n", "\n")
			normalizedExpected := strings.ReplaceAll(test.expected, "\r\n", "\n")

			// Compare the actual output with expected
			assert.Equal(t, normalizedExpected, normalizedOutput, "Output format mismatch for %s", test.name)

			// Additional validation for specific formats
			switch test.format {
			case "csv":
				// Verify CSV specific formatting
				lines := strings.Split(strings.TrimSpace(normalizedOutput), "\n")
				for _, line := range lines {
					// Check if values containing commas are properly quoted
					if strings.Contains(line, ",") {
						values := strings.Split(line, ",")
						for _, value := range values {
							if strings.Contains(value, ",") {
								assert.True(t, strings.HasPrefix(value, "\"") && strings.HasSuffix(value, "\""),
									"Values containing commas should be quoted in CSV format")
							}
						}
					}
				}
			case "tsv":
				// Verify TSV specific formatting
				assert.False(t, strings.Contains(normalizedOutput, "\""),
					"TSV format should not contain quotes")
				assert.True(t, strings.Contains(normalizedOutput, "\t"),
					"TSV format should use tabs as delimiters")
			}
		})
	}
}
