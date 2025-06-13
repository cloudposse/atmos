package list

import (
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	e "github.com/cloudposse/atmos/internal/exec"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/utils"
	u "github.com/cloudposse/atmos/pkg/utils"
)

const (
	testStack = "tenant1-ue2-dev"
)

func TestListComponents(t *testing.T) {
	configAndStacksInfo := schema.ConfigAndStacksInfo{}

	atmosConfig, err := cfg.InitCliConfig(configAndStacksInfo, true)
	require.NoError(t, err)

	stacksMap, err := e.ExecuteDescribeStacks(atmosConfig, "", nil, nil,
		nil, false, true, true, false, nil)
	assert.Nil(t, err)

	output, err := FilterAndListComponents("", stacksMap)
	require.NoError(t, err)
	dependentsYaml, err := u.ConvertToYAML(output)
	require.NoError(t, err)

	// Add assertions to validate the output structure
	assert.NotNil(t, dependentsYaml)
	assert.Greater(t, len(dependentsYaml), 0)
	t.Log(dependentsYaml)
}

func TestListComponentsWithStack(t *testing.T) {
	configAndStacksInfo := schema.ConfigAndStacksInfo{}

	atmosConfig, err := cfg.InitCliConfig(configAndStacksInfo, true)
	require.NoError(t, err)

	stacksMap, err := e.ExecuteDescribeStacks(atmosConfig, testStack, nil, nil,
		nil, false, true, true, false, nil)
	assert.Nil(t, err)

	output, err := FilterAndListComponents(testStack, stacksMap)
	require.NoError(t, err)
	assert.NotNil(t, output)
	assert.Greater(t, len(output), 0)
	assert.ElementsMatch(t, []string{
		"infra/vpc", "mixin/test-1", "mixin/test-2", "test/test-component",
		"test/test-component-override", "test/test-component-override-2", "test/test-component-override-3",
		"top-level-component1", "vpc", "vpc/new",
	}, output)
}

// TestGetStackComponents tests the getStackComponents function.
func TestGetStackComponents(t *testing.T) {
	// Test successful case
	stackData := map[string]any{
		"components": map[string]any{
			"terraform": map[string]any{
				"vpc":       map[string]any{},
				"infra/vpc": map[string]any{},
			},
		},
	}

	components, err := getStackComponents(stackData)
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"vpc", "infra/vpc"}, components)

	// Test error cases
	t.Run("invalid stack data", func(t *testing.T) {
		_, err := getStackComponents("not a map")
		require.Error(t, err)
		assert.True(t, errors.Is(err, ErrParseStacks))
	})

	t.Run("missing components", func(t *testing.T) {
		_, err := getStackComponents(map[string]any{
			"not-components": map[string]any{},
		})
		require.Error(t, err)
		assert.True(t, errors.Is(err, ErrParseComponents))
	})

	t.Run("missing terraform components", func(t *testing.T) {
		_, err := getStackComponents(map[string]any{
			"components": map[string]any{
				"not-terraform": map[string]any{},
			},
		})
		require.Error(t, err)
		assert.True(t, errors.Is(err, ErrParseTerraformComponents))
	})
}

// TestGetComponentsForSpecificStack tests the getComponentsForSpecificStack function.
func TestGetComponentsForSpecificStack(t *testing.T) {
	stacksMap := map[string]any{
		"stack1": map[string]any{
			"components": map[string]any{
				"terraform": map[string]any{
					"vpc":       map[string]any{},
					"infra/vpc": map[string]any{},
				},
			},
		},
		"stack2": map[string]any{
			"components": map[string]any{
				"terraform": map[string]any{
					"eks":       map[string]any{},
					"infra/eks": map[string]any{},
				},
			},
		},
	}

	// Test successful case
	t.Run("existing stack", func(t *testing.T) {
		components, err := getComponentsForSpecificStack("stack1", stacksMap)
		require.NoError(t, err)
		assert.ElementsMatch(t, []string{"vpc", "infra/vpc"}, components)
	})

	// Test error cases
	t.Run("non-existent stack", func(t *testing.T) {
		_, err := getComponentsForSpecificStack("non-existent-stack", stacksMap)
		require.Error(t, err)
		assert.True(t, errors.Is(err, ErrStackNotFound))
		assert.Contains(t, err.Error(), "non-existent-stack")
	})

	t.Run("invalid stack structure", func(t *testing.T) {
		invalidStacksMap := map[string]any{
			"invalid-stack": "not a map",
		}
		_, err := getComponentsForSpecificStack("invalid-stack", invalidStacksMap)
		require.Error(t, err)
		assert.True(t, errors.Is(err, ErrProcessStack))
		assert.Contains(t, err.Error(), "invalid-stack")
	})
}

// TestProcessAllStacks tests the processAllStacks function.
func TestProcessAllStacks(t *testing.T) {
	// Test with valid stacks
	t.Run("valid stacks", func(t *testing.T) {
		stacksMap := map[string]any{
			"stack1": map[string]any{
				"components": map[string]any{
					"terraform": map[string]any{
						"vpc": map[string]any{},
					},
				},
			},
			"stack2": map[string]any{
				"components": map[string]any{
					"terraform": map[string]any{
						"eks": map[string]any{},
					},
				},
			},
		}

		components := processAllStacks(stacksMap)
		assert.ElementsMatch(t, []string{"vpc", "eks"}, components)
	})

	// Test with mix of valid and invalid stacks
	t.Run("mixed valid and invalid stacks", func(t *testing.T) {
		stacksMap := map[string]any{
			"valid-stack": map[string]any{
				"components": map[string]any{
					"terraform": map[string]any{
						"vpc": map[string]any{},
					},
				},
			},
			"invalid-stack": "not a map",
			"empty-stack": map[string]any{
				"components": map[string]any{},
			},
		}

		components := processAllStacks(stacksMap)
		assert.ElementsMatch(t, []string{"vpc"}, components)
	})

	// Test with all invalid stacks
	t.Run("all invalid stacks", func(t *testing.T) {
		stacksMap := map[string]any{
			"invalid1": "not a map",
			"invalid2": map[string]any{
				"not-components": map[string]any{},
			},
		}

		components := processAllStacks(stacksMap)
		assert.Empty(t, components)
	})
}

// TestFilterAndListComponents tests the FilterAndListComponents function.
func TestFilterAndListComponents(t *testing.T) {
	stacksMap := map[string]any{
		"stack1": map[string]any{
			"components": map[string]any{
				"terraform": map[string]any{
					"vpc":  map[string]any{},
					"eks":  map[string]any{},
					"rds":  map[string]any{},
					"s3":   map[string]any{},
					"test": map[string]any{},
				},
			},
		},
		"stack2": map[string]any{
			"components": map[string]any{
				"terraform": map[string]any{
					"vpc":         map[string]any{},
					"eks":         map[string]any{},
					"elasticache": map[string]any{},
				},
			},
		},
	}

	// Test specific stack case
	t.Run("specific stack", func(t *testing.T) {
		components, err := FilterAndListComponents("stack1", stacksMap)
		require.NoError(t, err)
		assert.ElementsMatch(t, []string{"vpc", "eks", "rds", "s3", "test"}, components)
	})

	// Test all stacks case
	t.Run("all stacks", func(t *testing.T) {
		components, err := FilterAndListComponents("", stacksMap)
		require.NoError(t, err)
		assert.ElementsMatch(t, []string{"vpc", "eks", "rds", "s3", "test", "elasticache"}, components)
	})

	// Test error cases
	t.Run("non-existent stack", func(t *testing.T) {
		_, err := FilterAndListComponents("non-existent-stack", stacksMap)
		require.Error(t, err)
		assert.True(t, errors.Is(err, ErrStackNotFound))
	})

	// Test with nil stacks map
	t.Run("nil stacks map", func(t *testing.T) {
		_, err := FilterAndListComponents("stack1", nil)
		require.Error(t, err)
		assert.True(t, errors.Is(err, ErrStackNotFound))
	})

	// Test with empty stacks map
	t.Run("empty stacks map", func(t *testing.T) {
		_, err := FilterAndListComponents("stack1", map[string]any{})
		require.Error(t, err)
		assert.True(t, errors.Is(err, ErrStackNotFound))
	})
}

// TestFilterAndListComponentsIntegration performs integration tests with actual stack data.
func TestFilterAndListComponentsIntegration(t *testing.T) {
	configAndStacksInfo := schema.ConfigAndStacksInfo{}

	atmosConfig, err := cfg.InitCliConfig(configAndStacksInfo, true)
	require.NoError(t, err)

	// Test with invalid stack name
	t.Run("invalid stack name", func(t *testing.T) {
		stacksMap, err := e.ExecuteDescribeStacks(atmosConfig, "", nil, nil,
			nil, false, false, false, false, nil)
		require.NoError(t, err)

		_, err = FilterAndListComponents("non-existent-stack", stacksMap)
		require.Error(t, err)
		assert.True(t, errors.Is(err, ErrStackNotFound))
		assert.Contains(t, err.Error(), "non-existent-stack")
	})
}

// TestListComponentsWithColumns tests the custom columns functionality for components
func TestListComponentsWithColumns(t *testing.T) {
	stacksMap := map[string]any{
		"test-stack": map[string]any{
			"components": map[string]any{
				"terraform": map[string]any{
					"vpc": map[string]any{
						"metadata": map[string]any{
							"component": "vpc",
						},
					},
					"eks": map[string]any{
						"metadata": map[string]any{
							"component": "eks",
						},
					},
					"rds": map[string]any{
						"metadata": map[string]any{
							"component": "rds",
						},
					},
				},
			},
		},
	}

	atmosConfig := schema.AtmosConfiguration{
		Components: schema.Components{
			Terraform: schema.Terraform{
				BasePath: "components/terraform",
			},
		},
	}

	tests := []struct {
		name        string
		stackFlag   string
		listConfig  schema.ListConfig
		format      string
		delimiter   string
		validate    func(t *testing.T, output string)
		description string
	}{
		{
			name:      "default columns with JSON format",
			stackFlag: "",
			listConfig: schema.ListConfig{
				Columns: []schema.ListColumnConfig{
					{Name: "Component", Value: "{{ .component_name }}"},
					{Name: "Type", Value: "{{ .component_type }}"},
					{Name: "Path", Value: "{{ .component_path }}"},
				},
			},
			format:    "json",
			delimiter: "\t",
			validate: func(t *testing.T, output string) {
				assert.Contains(t, output, "\"Component\":")
				assert.Contains(t, output, "\"Type\":")
				assert.Contains(t, output, "\"Path\":")
				assert.Contains(t, output, "vpc")
				assert.Contains(t, output, "eks")
				assert.Contains(t, output, "rds")
				assert.Contains(t, output, "terraform")
				assert.Contains(t, output, "components/terraform")
			},
			description: "should output JSON with default columns",
		},
		{
			name:      "custom columns with templates",
			stackFlag: "test-stack",
			listConfig: schema.ListConfig{
				Columns: []schema.ListColumnConfig{
					{Name: "Name", Value: "{{ .component_name }}"},
					{Name: "FullPath", Value: "{{ .component_path }}/main.tf"},
					{Name: "Status", Value: "Active"},
				},
			},
			format:    "csv",
			delimiter: ",",
			validate: func(t *testing.T, output string) {
				lines := strings.Split(strings.TrimSpace(output), utils.GetLineEnding())
				assert.GreaterOrEqual(t, len(lines), 2)
				assert.Equal(t, "Name,FullPath,Status", lines[0])
				for i := 1; i < len(lines); i++ {
					fields := strings.Split(lines[i], ",")
					if len(fields) == 3 {
						assert.Equal(t, "Active", fields[2])
						assert.Contains(t, fields[1], "/main.tf")
					}
				}
			},
			description: "should handle custom columns with templates and static values",
		},
		{
			name:      "default simple list format",
			stackFlag: "",
			listConfig: schema.ListConfig{},
			format:    "",
			delimiter: "\t",
			validate: func(t *testing.T, output string) {
				lines := strings.Split(strings.TrimSpace(output), utils.GetLineEnding())
				assert.Equal(t, 3, len(lines))
				components := []string{"eks", "rds", "vpc"}
				for _, comp := range components {
					assert.Contains(t, output, comp)
				}
			},
			description: "should return simple list format when no format specified",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output, err := FilterAndListComponentsWithColumns(tt.stackFlag, stacksMap, tt.listConfig, tt.format, tt.delimiter, atmosConfig)
			assert.NoError(t, err)
			tt.validate(t, output)
		})
	}
}
