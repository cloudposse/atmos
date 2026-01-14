package cmd

import (
	"errors"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/cloudposse/atmos/pkg/auth"
	"github.com/cloudposse/atmos/pkg/schema"
)

// Test stackHasAnySource - checks if a stack has any terraform component with source configured.
func TestStackHasAnySource(t *testing.T) {
	tests := []struct {
		name      string
		stackData any
		expected  bool
	}{
		{
			name:      "nil stack data",
			stackData: nil,
			expected:  false,
		},
		{
			name:      "non-map stack data",
			stackData: "invalid",
			expected:  false,
		},
		{
			name:      "empty map",
			stackData: map[string]any{},
			expected:  false,
		},
		{
			name: "missing components key",
			stackData: map[string]any{
				"vars": map[string]any{"foo": "bar"},
			},
			expected: false,
		},
		{
			name: "components not a map",
			stackData: map[string]any{
				"components": "invalid",
			},
			expected: false,
		},
		{
			name: "missing terraform key",
			stackData: map[string]any{
				"components": map[string]any{
					"helmfile": map[string]any{},
				},
			},
			expected: false,
		},
		{
			name: "terraform not a map",
			stackData: map[string]any{
				"components": map[string]any{
					"terraform": "invalid",
				},
			},
			expected: false,
		},
		{
			name: "empty terraform components",
			stackData: map[string]any{
				"components": map[string]any{
					"terraform": map[string]any{},
				},
			},
			expected: false,
		},
		{
			name: "component without source",
			stackData: map[string]any{
				"components": map[string]any{
					"terraform": map[string]any{
						"vpc": map[string]any{
							"vars": map[string]any{"cidr": "10.0.0.0/16"},
						},
					},
				},
			},
			expected: false,
		},
		{
			name: "component with empty source string",
			stackData: map[string]any{
				"components": map[string]any{
					"terraform": map[string]any{
						"vpc": map[string]any{
							"source": "",
						},
					},
				},
			},
			expected: false,
		},
		{
			name: "component with source string",
			stackData: map[string]any{
				"components": map[string]any{
					"terraform": map[string]any{
						"vpc": map[string]any{
							"source": "github.com/example/terraform-aws-vpc",
						},
					},
				},
			},
			expected: true,
		},
		{
			name: "component with source map containing uri",
			stackData: map[string]any{
				"components": map[string]any{
					"terraform": map[string]any{
						"vpc": map[string]any{
							"source": map[string]any{
								"uri": "github.com/example/terraform-aws-vpc",
							},
						},
					},
				},
			},
			expected: true,
		},
		{
			name: "component with source map with empty uri",
			stackData: map[string]any{
				"components": map[string]any{
					"terraform": map[string]any{
						"vpc": map[string]any{
							"source": map[string]any{
								"uri": "",
							},
						},
					},
				},
			},
			expected: false,
		},
		{
			name: "component data not a map",
			stackData: map[string]any{
				"components": map[string]any{
					"terraform": map[string]any{
						"vpc": "invalid",
					},
				},
			},
			expected: false,
		},
		{
			name: "multiple components one with source",
			stackData: map[string]any{
				"components": map[string]any{
					"terraform": map[string]any{
						"vpc": map[string]any{
							"vars": map[string]any{"cidr": "10.0.0.0/16"},
						},
						"eks": map[string]any{
							"source": "github.com/example/terraform-aws-eks",
						},
					},
				},
			},
			expected: true,
		},
		{
			name: "multiple components none with source",
			stackData: map[string]any{
				"components": map[string]any{
					"terraform": map[string]any{
						"vpc": map[string]any{
							"vars": map[string]any{"cidr": "10.0.0.0/16"},
						},
						"eks": map[string]any{
							"vars": map[string]any{"cluster_name": "test"},
						},
					},
				},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := stackHasAnySource(tt.stackData)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// Test stackContainsComponentWithSource - checks if a stack contains a specific component with source.
func TestStackContainsComponentWithSource(t *testing.T) {
	tests := []struct {
		name      string
		stackData any
		component string
		expected  bool
	}{
		{
			name:      "nil stack data",
			stackData: nil,
			component: "vpc",
			expected:  false,
		},
		{
			name:      "non-map stack data",
			stackData: "invalid",
			component: "vpc",
			expected:  false,
		},
		{
			name:      "empty map",
			stackData: map[string]any{},
			component: "vpc",
			expected:  false,
		},
		{
			name: "missing components key",
			stackData: map[string]any{
				"vars": map[string]any{},
			},
			component: "vpc",
			expected:  false,
		},
		{
			name: "components not a map",
			stackData: map[string]any{
				"components": "invalid",
			},
			component: "vpc",
			expected:  false,
		},
		{
			name: "missing terraform key",
			stackData: map[string]any{
				"components": map[string]any{
					"helmfile": map[string]any{},
				},
			},
			component: "vpc",
			expected:  false,
		},
		{
			name: "terraform not a map",
			stackData: map[string]any{
				"components": map[string]any{
					"terraform": "invalid",
				},
			},
			component: "vpc",
			expected:  false,
		},
		{
			name: "component not found",
			stackData: map[string]any{
				"components": map[string]any{
					"terraform": map[string]any{
						"eks": map[string]any{
							"source": "github.com/example/eks",
						},
					},
				},
			},
			component: "vpc",
			expected:  false,
		},
		{
			name: "component found but data not a map",
			stackData: map[string]any{
				"components": map[string]any{
					"terraform": map[string]any{
						"vpc": "invalid",
					},
				},
			},
			component: "vpc",
			expected:  false,
		},
		{
			name: "component found but no source",
			stackData: map[string]any{
				"components": map[string]any{
					"terraform": map[string]any{
						"vpc": map[string]any{
							"vars": map[string]any{"cidr": "10.0.0.0/16"},
						},
					},
				},
			},
			component: "vpc",
			expected:  false,
		},
		{
			name: "component found with source string",
			stackData: map[string]any{
				"components": map[string]any{
					"terraform": map[string]any{
						"vpc": map[string]any{
							"source": "github.com/example/terraform-aws-vpc",
						},
					},
				},
			},
			component: "vpc",
			expected:  true,
		},
		{
			name: "component found with source map",
			stackData: map[string]any{
				"components": map[string]any{
					"terraform": map[string]any{
						"vpc": map[string]any{
							"source": map[string]any{
								"uri": "github.com/example/terraform-aws-vpc",
							},
						},
					},
				},
			},
			component: "vpc",
			expected:  true,
		},
		{
			name: "component found with empty source string",
			stackData: map[string]any{
				"components": map[string]any{
					"terraform": map[string]any{
						"vpc": map[string]any{
							"source": "",
						},
					},
				},
			},
			component: "vpc",
			expected:  false,
		},
		{
			name: "component found with source map empty uri",
			stackData: map[string]any{
				"components": map[string]any{
					"terraform": map[string]any{
						"vpc": map[string]any{
							"source": map[string]any{
								"uri": "",
							},
						},
					},
				},
			},
			component: "vpc",
			expected:  false,
		},
		{
			name: "different component has source target does not",
			stackData: map[string]any{
				"components": map[string]any{
					"terraform": map[string]any{
						"vpc": map[string]any{
							"vars": map[string]any{"cidr": "10.0.0.0/16"},
						},
						"eks": map[string]any{
							"source": "github.com/example/eks",
						},
					},
				},
			},
			component: "vpc",
			expected:  false,
		},
		{
			name:      "empty component name",
			component: "",
			stackData: map[string]any{
				"components": map[string]any{
					"terraform": map[string]any{
						"vpc": map[string]any{
							"source": "github.com/example/vpc",
						},
					},
				},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := stackContainsComponentWithSource(tt.stackData, tt.component)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// Test collectComponentsWithSource - extracts terraform components with source from a stack.
func TestCollectComponentsWithSource(t *testing.T) {
	tests := []struct {
		name              string
		stackData         any
		existingSet       map[string]struct{}
		expectedSet       map[string]struct{}
		expectedAdditions int
	}{
		{
			name:              "nil stack data",
			stackData:         nil,
			existingSet:       map[string]struct{}{},
			expectedSet:       map[string]struct{}{},
			expectedAdditions: 0,
		},
		{
			name:              "non-map stack data",
			stackData:         "invalid",
			existingSet:       map[string]struct{}{},
			expectedSet:       map[string]struct{}{},
			expectedAdditions: 0,
		},
		{
			name:              "empty map",
			stackData:         map[string]any{},
			existingSet:       map[string]struct{}{},
			expectedSet:       map[string]struct{}{},
			expectedAdditions: 0,
		},
		{
			name: "missing components key",
			stackData: map[string]any{
				"vars": map[string]any{},
			},
			existingSet:       map[string]struct{}{},
			expectedSet:       map[string]struct{}{},
			expectedAdditions: 0,
		},
		{
			name: "components not a map",
			stackData: map[string]any{
				"components": "invalid",
			},
			existingSet:       map[string]struct{}{},
			expectedSet:       map[string]struct{}{},
			expectedAdditions: 0,
		},
		{
			name: "missing terraform key",
			stackData: map[string]any{
				"components": map[string]any{
					"helmfile": map[string]any{},
				},
			},
			existingSet:       map[string]struct{}{},
			expectedSet:       map[string]struct{}{},
			expectedAdditions: 0,
		},
		{
			name: "terraform not a map",
			stackData: map[string]any{
				"components": map[string]any{
					"terraform": "invalid",
				},
			},
			existingSet:       map[string]struct{}{},
			expectedSet:       map[string]struct{}{},
			expectedAdditions: 0,
		},
		{
			name: "empty terraform components",
			stackData: map[string]any{
				"components": map[string]any{
					"terraform": map[string]any{},
				},
			},
			existingSet:       map[string]struct{}{},
			expectedSet:       map[string]struct{}{},
			expectedAdditions: 0,
		},
		{
			name: "component without source not added",
			stackData: map[string]any{
				"components": map[string]any{
					"terraform": map[string]any{
						"vpc": map[string]any{
							"vars": map[string]any{"cidr": "10.0.0.0/16"},
						},
					},
				},
			},
			existingSet:       map[string]struct{}{},
			expectedSet:       map[string]struct{}{},
			expectedAdditions: 0,
		},
		{
			name: "component with source string added",
			stackData: map[string]any{
				"components": map[string]any{
					"terraform": map[string]any{
						"vpc": map[string]any{
							"source": "github.com/example/terraform-aws-vpc",
						},
					},
				},
			},
			existingSet:       map[string]struct{}{},
			expectedSet:       map[string]struct{}{"vpc": {}},
			expectedAdditions: 1,
		},
		{
			name: "component with source map added",
			stackData: map[string]any{
				"components": map[string]any{
					"terraform": map[string]any{
						"eks": map[string]any{
							"source": map[string]any{
								"uri": "github.com/example/terraform-aws-eks",
							},
						},
					},
				},
			},
			existingSet:       map[string]struct{}{},
			expectedSet:       map[string]struct{}{"eks": {}},
			expectedAdditions: 1,
		},
		{
			name: "component with empty source string not added",
			stackData: map[string]any{
				"components": map[string]any{
					"terraform": map[string]any{
						"vpc": map[string]any{
							"source": "",
						},
					},
				},
			},
			existingSet:       map[string]struct{}{},
			expectedSet:       map[string]struct{}{},
			expectedAdditions: 0,
		},
		{
			name: "component data not a map skipped",
			stackData: map[string]any{
				"components": map[string]any{
					"terraform": map[string]any{
						"vpc":     "invalid",
						"rds":     42,
						"lambda":  []string{"item"},
						"valid":   map[string]any{"source": "github.com/example/valid"},
						"invalid": nil,
					},
				},
			},
			existingSet:       map[string]struct{}{},
			expectedSet:       map[string]struct{}{"valid": {}},
			expectedAdditions: 1,
		},
		{
			name: "multiple components some with source",
			stackData: map[string]any{
				"components": map[string]any{
					"terraform": map[string]any{
						"vpc": map[string]any{
							"source": "github.com/example/vpc",
						},
						"eks": map[string]any{
							"vars": map[string]any{"cluster_name": "test"},
						},
						"rds": map[string]any{
							"source": map[string]any{
								"uri": "github.com/example/rds",
							},
						},
					},
				},
			},
			existingSet:       map[string]struct{}{},
			expectedSet:       map[string]struct{}{"vpc": {}, "rds": {}},
			expectedAdditions: 2,
		},
		{
			name: "adds to existing set without duplicates",
			stackData: map[string]any{
				"components": map[string]any{
					"terraform": map[string]any{
						"vpc": map[string]any{
							"source": "github.com/example/vpc",
						},
						"eks": map[string]any{
							"source": "github.com/example/eks",
						},
					},
				},
			},
			existingSet:       map[string]struct{}{"vpc": {}, "rds": {}},
			expectedSet:       map[string]struct{}{"vpc": {}, "rds": {}, "eks": {}},
			expectedAdditions: 1, // Only eks is new.
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clone existing set to avoid mutation affecting test setup.
			componentSet := make(map[string]struct{})
			for k, v := range tt.existingSet {
				componentSet[k] = v
			}

			initialLen := len(componentSet)
			collectComponentsWithSource(tt.stackData, componentSet)

			// Verify the set contains expected components.
			assert.Equal(t, tt.expectedSet, componentSet)

			// Verify number of additions.
			actualAdditions := len(componentSet) - initialLen
			assert.Equal(t, tt.expectedAdditions, actualAdditions)
		})
	}
}

// Test listComponentsWithSource - returns all terraform components that have source configured.
func TestListComponentsWithSource(t *testing.T) {
	// Save originals and restore after test.
	origInitFunc := initCliConfigForPrompt
	origDescribeFunc := executeDescribeStacksFunc
	defer func() {
		initCliConfigForPrompt = origInitFunc
		executeDescribeStacksFunc = origDescribeFunc
	}()

	t.Run("config init error returns error", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		mockLoader := NewMockConfigLoader(ctrl)
		mockLoader.EXPECT().
			InitCliConfig(gomock.Any(), true).
			Return(schema.AtmosConfiguration{}, errors.New("config init failed"))

		initCliConfigForPrompt = mockLoader.InitCliConfig

		result, err := listComponentsWithSource()
		require.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "config init failed")
	})

	t.Run("describe stacks error returns error", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		mockLoader := NewMockConfigLoader(ctrl)
		mockLoader.EXPECT().
			InitCliConfig(gomock.Any(), true).
			Return(schema.AtmosConfiguration{}, nil)

		initCliConfigForPrompt = mockLoader.InitCliConfig
		executeDescribeStacksFunc = func(
			atmosConfig *schema.AtmosConfiguration,
			filterByStack string,
			components []string,
			componentTypes []string,
			sections []string,
			ignoreMissingFiles bool,
			processTemplates bool,
			processYamlFunctions bool,
			includeEmptyStacks bool,
			skip []string,
			authManager auth.AuthManager,
		) (map[string]any, error) {
			return nil, errors.New("describe stacks failed")
		}

		result, err := listComponentsWithSource()
		require.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "describe stacks failed")
	})

	t.Run("empty stacks returns empty list", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		mockLoader := NewMockConfigLoader(ctrl)
		mockLoader.EXPECT().
			InitCliConfig(gomock.Any(), true).
			Return(schema.AtmosConfiguration{}, nil)

		initCliConfigForPrompt = mockLoader.InitCliConfig
		executeDescribeStacksFunc = func(
			atmosConfig *schema.AtmosConfiguration,
			filterByStack string,
			components []string,
			componentTypes []string,
			sections []string,
			ignoreMissingFiles bool,
			processTemplates bool,
			processYamlFunctions bool,
			includeEmptyStacks bool,
			skip []string,
			authManager auth.AuthManager,
		) (map[string]any, error) {
			return map[string]any{}, nil
		}

		result, err := listComponentsWithSource()
		require.NoError(t, err)
		assert.Empty(t, result)
	})

	t.Run("returns sorted unique components with source", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		mockLoader := NewMockConfigLoader(ctrl)
		mockLoader.EXPECT().
			InitCliConfig(gomock.Any(), true).
			Return(schema.AtmosConfiguration{}, nil)

		initCliConfigForPrompt = mockLoader.InitCliConfig
		executeDescribeStacksFunc = func(
			atmosConfig *schema.AtmosConfiguration,
			filterByStack string,
			components []string,
			componentTypes []string,
			sections []string,
			ignoreMissingFiles bool,
			processTemplates bool,
			processYamlFunctions bool,
			includeEmptyStacks bool,
			skip []string,
			authManager auth.AuthManager,
		) (map[string]any, error) {
			return map[string]any{
				"dev-us-east-1": map[string]any{
					"components": map[string]any{
						"terraform": map[string]any{
							"vpc": map[string]any{"source": "github.com/example/vpc"},
							"eks": map[string]any{"vars": map[string]any{}}, // No source.
						},
					},
				},
				"prod-us-west-2": map[string]any{
					"components": map[string]any{
						"terraform": map[string]any{
							"vpc": map[string]any{"source": "github.com/example/vpc"}, // Same component.
							"rds": map[string]any{"source": map[string]any{"uri": "github.com/example/rds"}},
						},
					},
				},
			}, nil
		}

		result, err := listComponentsWithSource()
		require.NoError(t, err)
		assert.Equal(t, []string{"rds", "vpc"}, result) // Sorted, deduplicated.
	})
}

// Test listStacksWithSource - returns stacks with source-configured components.
func TestListStacksWithSource(t *testing.T) {
	// Save originals and restore after test.
	origInitFunc := initCliConfigForPrompt
	origDescribeFunc := executeDescribeStacksFunc
	defer func() {
		initCliConfigForPrompt = origInitFunc
		executeDescribeStacksFunc = origDescribeFunc
	}()

	t.Run("config init error returns error", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		mockLoader := NewMockConfigLoader(ctrl)
		mockLoader.EXPECT().
			InitCliConfig(gomock.Any(), true).
			Return(schema.AtmosConfiguration{}, errors.New("config init failed"))

		initCliConfigForPrompt = mockLoader.InitCliConfig

		result, err := listStacksWithSource()
		require.Error(t, err)
		assert.Nil(t, result)
	})

	t.Run("describe stacks error returns error", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		mockLoader := NewMockConfigLoader(ctrl)
		mockLoader.EXPECT().
			InitCliConfig(gomock.Any(), true).
			Return(schema.AtmosConfiguration{}, nil)

		initCliConfigForPrompt = mockLoader.InitCliConfig
		executeDescribeStacksFunc = func(
			atmosConfig *schema.AtmosConfiguration,
			filterByStack string,
			components []string,
			componentTypes []string,
			sections []string,
			ignoreMissingFiles bool,
			processTemplates bool,
			processYamlFunctions bool,
			includeEmptyStacks bool,
			skip []string,
			authManager auth.AuthManager,
		) (map[string]any, error) {
			return nil, errors.New("describe stacks failed")
		}

		result, err := listStacksWithSource()
		require.Error(t, err)
		assert.Nil(t, result)
	})

	t.Run("returns sorted stacks with source-configured components", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		mockLoader := NewMockConfigLoader(ctrl)
		mockLoader.EXPECT().
			InitCliConfig(gomock.Any(), true).
			Return(schema.AtmosConfiguration{}, nil)

		initCliConfigForPrompt = mockLoader.InitCliConfig
		executeDescribeStacksFunc = func(
			atmosConfig *schema.AtmosConfiguration,
			filterByStack string,
			components []string,
			componentTypes []string,
			sections []string,
			ignoreMissingFiles bool,
			processTemplates bool,
			processYamlFunctions bool,
			includeEmptyStacks bool,
			skip []string,
			authManager auth.AuthManager,
		) (map[string]any, error) {
			return map[string]any{
				"dev-us-east-1": map[string]any{
					"components": map[string]any{
						"terraform": map[string]any{
							"vpc": map[string]any{"source": "github.com/example/vpc"},
						},
					},
				},
				"staging-eu-west-1": map[string]any{
					"components": map[string]any{
						"terraform": map[string]any{
							"eks": map[string]any{"vars": map[string]any{}}, // No source.
						},
					},
				},
				"prod-us-west-2": map[string]any{
					"components": map[string]any{
						"terraform": map[string]any{
							"rds": map[string]any{"source": map[string]any{"uri": "github.com/example/rds"}},
						},
					},
				},
			}, nil
		}

		result, err := listStacksWithSource()
		require.NoError(t, err)
		assert.Equal(t, []string{"dev-us-east-1", "prod-us-west-2"}, result) // Sorted, staging excluded.
	})
}

// Test listStacksWithSourceForComponent - returns stacks containing a specific component with source.
func TestListStacksWithSourceForComponent(t *testing.T) {
	// Save originals and restore after test.
	origInitFunc := initCliConfigForPrompt
	origDescribeFunc := executeDescribeStacksFunc
	defer func() {
		initCliConfigForPrompt = origInitFunc
		executeDescribeStacksFunc = origDescribeFunc
	}()

	t.Run("config init error returns error", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		mockLoader := NewMockConfigLoader(ctrl)
		mockLoader.EXPECT().
			InitCliConfig(gomock.Any(), true).
			Return(schema.AtmosConfiguration{}, errors.New("config init failed"))

		initCliConfigForPrompt = mockLoader.InitCliConfig

		result, err := listStacksWithSourceForComponent("vpc")
		require.Error(t, err)
		assert.Nil(t, result)
	})

	t.Run("returns sorted stacks containing component with source", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		mockLoader := NewMockConfigLoader(ctrl)
		mockLoader.EXPECT().
			InitCliConfig(gomock.Any(), true).
			Return(schema.AtmosConfiguration{}, nil)

		initCliConfigForPrompt = mockLoader.InitCliConfig
		executeDescribeStacksFunc = func(
			atmosConfig *schema.AtmosConfiguration,
			filterByStack string,
			components []string,
			componentTypes []string,
			sections []string,
			ignoreMissingFiles bool,
			processTemplates bool,
			processYamlFunctions bool,
			includeEmptyStacks bool,
			skip []string,
			authManager auth.AuthManager,
		) (map[string]any, error) {
			return map[string]any{
				"dev-us-east-1": map[string]any{
					"components": map[string]any{
						"terraform": map[string]any{
							"vpc": map[string]any{"source": "github.com/example/vpc"},
						},
					},
				},
				"staging-eu-west-1": map[string]any{
					"components": map[string]any{
						"terraform": map[string]any{
							"vpc": map[string]any{"vars": map[string]any{}}, // Has vpc but no source.
						},
					},
				},
				"prod-us-west-2": map[string]any{
					"components": map[string]any{
						"terraform": map[string]any{
							"vpc": map[string]any{"source": map[string]any{"uri": "github.com/example/vpc"}},
							"rds": map[string]any{"source": "github.com/example/rds"},
						},
					},
				},
				"test-ap-south-1": map[string]any{
					"components": map[string]any{
						"terraform": map[string]any{
							"rds": map[string]any{"source": "github.com/example/rds"}, // Different component.
						},
					},
				},
			}, nil
		}

		result, err := listStacksWithSourceForComponent("vpc")
		require.NoError(t, err)
		assert.Equal(t, []string{"dev-us-east-1", "prod-us-west-2"}, result) // Only stacks with vpc+source.
	})

	t.Run("component not found in any stack", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		mockLoader := NewMockConfigLoader(ctrl)
		mockLoader.EXPECT().
			InitCliConfig(gomock.Any(), true).
			Return(schema.AtmosConfiguration{}, nil)

		initCliConfigForPrompt = mockLoader.InitCliConfig
		executeDescribeStacksFunc = func(
			atmosConfig *schema.AtmosConfiguration,
			filterByStack string,
			components []string,
			componentTypes []string,
			sections []string,
			ignoreMissingFiles bool,
			processTemplates bool,
			processYamlFunctions bool,
			includeEmptyStacks bool,
			skip []string,
			authManager auth.AuthManager,
		) (map[string]any, error) {
			return map[string]any{
				"dev-us-east-1": map[string]any{
					"components": map[string]any{
						"terraform": map[string]any{
							"rds": map[string]any{"source": "github.com/example/rds"},
						},
					},
				},
			}, nil
		}

		result, err := listStacksWithSourceForComponent("nonexistent")
		require.NoError(t, err)
		assert.Empty(t, result)
	})
}

// Test ComponentArgCompletion - shell completion for component argument.
func TestComponentArgCompletion(t *testing.T) {
	// Save originals and restore after test.
	origInitFunc := initCliConfigForPrompt
	origDescribeFunc := executeDescribeStacksFunc
	defer func() {
		initCliConfigForPrompt = origInitFunc
		executeDescribeStacksFunc = origDescribeFunc
	}()

	t.Run("returns components when args is empty", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		mockLoader := NewMockConfigLoader(ctrl)
		mockLoader.EXPECT().
			InitCliConfig(gomock.Any(), true).
			Return(schema.AtmosConfiguration{}, nil)

		initCliConfigForPrompt = mockLoader.InitCliConfig
		executeDescribeStacksFunc = func(
			atmosConfig *schema.AtmosConfiguration,
			filterByStack string,
			components []string,
			componentTypes []string,
			sections []string,
			ignoreMissingFiles bool,
			processTemplates bool,
			processYamlFunctions bool,
			includeEmptyStacks bool,
			skip []string,
			authManager auth.AuthManager,
		) (map[string]any, error) {
			return map[string]any{
				"dev": map[string]any{
					"components": map[string]any{
						"terraform": map[string]any{
							"vpc": map[string]any{"source": "github.com/example/vpc"},
						},
					},
				},
			}, nil
		}

		cmd := &cobra.Command{Use: "test"}
		result, directive := ComponentArgCompletion(cmd, []string{}, "")

		assert.Equal(t, []string{"vpc"}, result)
		assert.Equal(t, cobra.ShellCompDirectiveNoFileComp, directive)
	})

	t.Run("returns nil when args already provided", func(t *testing.T) {
		cmd := &cobra.Command{Use: "test"}
		result, directive := ComponentArgCompletion(cmd, []string{"existing"}, "")

		assert.Nil(t, result)
		assert.Equal(t, cobra.ShellCompDirectiveNoFileComp, directive)
	})

	t.Run("returns nil on error", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		mockLoader := NewMockConfigLoader(ctrl)
		mockLoader.EXPECT().
			InitCliConfig(gomock.Any(), true).
			Return(schema.AtmosConfiguration{}, errors.New("config error"))

		initCliConfigForPrompt = mockLoader.InitCliConfig

		cmd := &cobra.Command{Use: "test"}
		result, directive := ComponentArgCompletion(cmd, []string{}, "")

		assert.Nil(t, result)
		assert.Equal(t, cobra.ShellCompDirectiveNoFileComp, directive)
	})
}

// Test StackFlagCompletion - shell completion for stack flag.
func TestStackFlagCompletion(t *testing.T) {
	// Save originals and restore after test.
	origInitFunc := initCliConfigForPrompt
	origDescribeFunc := executeDescribeStacksFunc
	defer func() {
		initCliConfigForPrompt = origInitFunc
		executeDescribeStacksFunc = origDescribeFunc
	}()

	t.Run("returns filtered stacks when component provided", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		mockLoader := NewMockConfigLoader(ctrl)
		mockLoader.EXPECT().
			InitCliConfig(gomock.Any(), true).
			Return(schema.AtmosConfiguration{}, nil)

		initCliConfigForPrompt = mockLoader.InitCliConfig
		executeDescribeStacksFunc = func(
			atmosConfig *schema.AtmosConfiguration,
			filterByStack string,
			components []string,
			componentTypes []string,
			sections []string,
			ignoreMissingFiles bool,
			processTemplates bool,
			processYamlFunctions bool,
			includeEmptyStacks bool,
			skip []string,
			authManager auth.AuthManager,
		) (map[string]any, error) {
			return map[string]any{
				"dev": map[string]any{
					"components": map[string]any{
						"terraform": map[string]any{
							"vpc": map[string]any{"source": "github.com/example/vpc"},
						},
					},
				},
				"prod": map[string]any{
					"components": map[string]any{
						"terraform": map[string]any{
							"rds": map[string]any{"source": "github.com/example/rds"},
						},
					},
				},
			}, nil
		}

		cmd := &cobra.Command{Use: "test"}
		result, directive := StackFlagCompletion(cmd, []string{"vpc"}, "")

		assert.Equal(t, []string{"dev"}, result)
		assert.Equal(t, cobra.ShellCompDirectiveNoFileComp, directive)
	})

	t.Run("returns all stacks when no component provided", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		mockLoader := NewMockConfigLoader(ctrl)
		mockLoader.EXPECT().
			InitCliConfig(gomock.Any(), true).
			Return(schema.AtmosConfiguration{}, nil)

		initCliConfigForPrompt = mockLoader.InitCliConfig
		executeDescribeStacksFunc = func(
			atmosConfig *schema.AtmosConfiguration,
			filterByStack string,
			components []string,
			componentTypes []string,
			sections []string,
			ignoreMissingFiles bool,
			processTemplates bool,
			processYamlFunctions bool,
			includeEmptyStacks bool,
			skip []string,
			authManager auth.AuthManager,
		) (map[string]any, error) {
			return map[string]any{
				"dev": map[string]any{
					"components": map[string]any{
						"terraform": map[string]any{
							"vpc": map[string]any{"source": "github.com/example/vpc"},
						},
					},
				},
				"prod": map[string]any{
					"components": map[string]any{
						"terraform": map[string]any{
							"rds": map[string]any{"source": "github.com/example/rds"},
						},
					},
				},
			}, nil
		}

		cmd := &cobra.Command{Use: "test"}
		result, directive := StackFlagCompletion(cmd, []string{}, "")

		assert.Equal(t, []string{"dev", "prod"}, result)
		assert.Equal(t, cobra.ShellCompDirectiveNoFileComp, directive)
	})

	t.Run("returns nil on error with component filter", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		mockLoader := NewMockConfigLoader(ctrl)
		mockLoader.EXPECT().
			InitCliConfig(gomock.Any(), true).
			Return(schema.AtmosConfiguration{}, errors.New("config error"))

		initCliConfigForPrompt = mockLoader.InitCliConfig

		cmd := &cobra.Command{Use: "test"}
		result, directive := StackFlagCompletion(cmd, []string{"vpc"}, "")

		assert.Nil(t, result)
		assert.Equal(t, cobra.ShellCompDirectiveNoFileComp, directive)
	})

	t.Run("returns nil on error without component filter", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		mockLoader := NewMockConfigLoader(ctrl)
		mockLoader.EXPECT().
			InitCliConfig(gomock.Any(), true).
			Return(schema.AtmosConfiguration{}, errors.New("config error"))

		initCliConfigForPrompt = mockLoader.InitCliConfig

		cmd := &cobra.Command{Use: "test"}
		result, directive := StackFlagCompletion(cmd, []string{}, "")

		assert.Nil(t, result)
		assert.Equal(t, cobra.ShellCompDirectiveNoFileComp, directive)
	})
}
