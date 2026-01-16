package cmd

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth"
	"github.com/cloudposse/atmos/pkg/schema"
)

// TestHandlePromptError_NilError tests that HandlePromptError returns nil for nil error.
func TestHandlePromptError_NilError(t *testing.T) {
	err := HandlePromptError(nil, "test")
	assert.NoError(t, err)
}

// TestHandlePromptError_InteractiveModeNotAvailable tests that HandlePromptError returns nil
// for ErrInteractiveModeNotAvailable (graceful fallback).
func TestHandlePromptError_InteractiveModeNotAvailable(t *testing.T) {
	err := HandlePromptError(errUtils.ErrInteractiveModeNotAvailable, "test")
	assert.NoError(t, err)
}

// TestHandlePromptError_OtherError tests that HandlePromptError propagates other errors.
func TestHandlePromptError_OtherError(t *testing.T) {
	err := HandlePromptError(assert.AnError, "test")
	assert.Error(t, err)
	assert.Equal(t, assert.AnError, err)
}

// TestStackContainsComponentWithSource_InvalidStackData tests with non-map stack data.
func TestStackContainsComponentWithSource_InvalidStackData(t *testing.T) {
	result := stackContainsComponentWithSource("invalid", "vpc")
	assert.False(t, result)
}

// TestStackContainsComponentWithSource_NoComponents tests with missing components.
func TestStackContainsComponentWithSource_NoComponents(t *testing.T) {
	stackData := map[string]any{
		"vars": map[string]any{"foo": "bar"},
	}
	result := stackContainsComponentWithSource(stackData, "vpc")
	assert.False(t, result)
}

// TestStackContainsComponentWithSource_NoTerraform tests with missing terraform section.
func TestStackContainsComponentWithSource_NoTerraform(t *testing.T) {
	stackData := map[string]any{
		"components": map[string]any{
			"helmfile": map[string]any{},
		},
	}
	result := stackContainsComponentWithSource(stackData, "vpc")
	assert.False(t, result)
}

// TestStackContainsComponentWithSource_ComponentNotFound tests with component not in stack.
func TestStackContainsComponentWithSource_ComponentNotFound(t *testing.T) {
	stackData := map[string]any{
		"components": map[string]any{
			"terraform": map[string]any{
				"other": map[string]any{
					"vars": map[string]any{"foo": "bar"},
				},
			},
		},
	}
	result := stackContainsComponentWithSource(stackData, "vpc")
	assert.False(t, result)
}

// TestStackContainsComponentWithSource_ComponentNoSource tests with component without source.
func TestStackContainsComponentWithSource_ComponentNoSource(t *testing.T) {
	stackData := map[string]any{
		"components": map[string]any{
			"terraform": map[string]any{
				"vpc": map[string]any{
					"vars": map[string]any{"foo": "bar"},
				},
			},
		},
	}
	result := stackContainsComponentWithSource(stackData, "vpc")
	assert.False(t, result)
}

// TestStackContainsComponentWithSource_ComponentWithSource tests with component that has source.
func TestStackContainsComponentWithSource_ComponentWithSource(t *testing.T) {
	stackData := map[string]any{
		"components": map[string]any{
			"terraform": map[string]any{
				"vpc": map[string]any{
					"source": map[string]any{
						"uri": "github.com/example/vpc",
					},
				},
			},
		},
	}
	result := stackContainsComponentWithSource(stackData, "vpc")
	assert.True(t, result)
}

// TestStackContainsComponentWithSource_InvalidComponentData tests with invalid component data type.
func TestStackContainsComponentWithSource_InvalidComponentData(t *testing.T) {
	stackData := map[string]any{
		"components": map[string]any{
			"terraform": map[string]any{
				"vpc": "invalid", // Should be a map.
			},
		},
	}
	result := stackContainsComponentWithSource(stackData, "vpc")
	assert.False(t, result)
}

// TestStackHasAnySource_InvalidStackData tests with non-map stack data.
func TestStackHasAnySource_InvalidStackData(t *testing.T) {
	result := stackHasAnySource("invalid")
	assert.False(t, result)
}

// TestStackHasAnySource_NoComponents tests with missing components.
func TestStackHasAnySource_NoComponents(t *testing.T) {
	stackData := map[string]any{
		"vars": map[string]any{"foo": "bar"},
	}
	result := stackHasAnySource(stackData)
	assert.False(t, result)
}

// TestStackHasAnySource_NoTerraform tests with missing terraform section.
func TestStackHasAnySource_NoTerraform(t *testing.T) {
	stackData := map[string]any{
		"components": map[string]any{
			"helmfile": map[string]any{},
		},
	}
	result := stackHasAnySource(stackData)
	assert.False(t, result)
}

// TestStackHasAnySource_NoComponentsWithSource tests with components but none have source.
func TestStackHasAnySource_NoComponentsWithSource(t *testing.T) {
	stackData := map[string]any{
		"components": map[string]any{
			"terraform": map[string]any{
				"vpc": map[string]any{
					"vars": map[string]any{"foo": "bar"},
				},
				"rds": map[string]any{
					"vars": map[string]any{"bar": "baz"},
				},
			},
		},
	}
	result := stackHasAnySource(stackData)
	assert.False(t, result)
}

// TestStackHasAnySource_HasComponentWithSource tests with at least one component with source.
func TestStackHasAnySource_HasComponentWithSource(t *testing.T) {
	stackData := map[string]any{
		"components": map[string]any{
			"terraform": map[string]any{
				"vpc": map[string]any{
					"vars": map[string]any{"foo": "bar"},
				},
				"rds": map[string]any{
					"source": map[string]any{
						"uri": "github.com/example/rds",
					},
				},
			},
		},
	}
	result := stackHasAnySource(stackData)
	assert.True(t, result)
}

// TestStackHasAnySource_InvalidComponentData tests with invalid component data type.
func TestStackHasAnySource_InvalidComponentData(t *testing.T) {
	stackData := map[string]any{
		"components": map[string]any{
			"terraform": map[string]any{
				"vpc": "invalid", // Should be a map.
			},
		},
	}
	result := stackHasAnySource(stackData)
	assert.False(t, result)
}

// TestCollectComponentsWithSource_InvalidStackData tests with non-map stack data.
func TestCollectComponentsWithSource_InvalidStackData(t *testing.T) {
	componentSet := make(map[string]struct{})
	collectComponentsWithSource("invalid", componentSet)
	assert.Empty(t, componentSet)
}

// TestCollectComponentsWithSource_NoComponents tests with missing components.
func TestCollectComponentsWithSource_NoComponents(t *testing.T) {
	componentSet := make(map[string]struct{})
	stackData := map[string]any{
		"vars": map[string]any{"foo": "bar"},
	}
	collectComponentsWithSource(stackData, componentSet)
	assert.Empty(t, componentSet)
}

// TestCollectComponentsWithSource_NoTerraform tests with missing terraform section.
func TestCollectComponentsWithSource_NoTerraform(t *testing.T) {
	componentSet := make(map[string]struct{})
	stackData := map[string]any{
		"components": map[string]any{
			"helmfile": map[string]any{},
		},
	}
	collectComponentsWithSource(stackData, componentSet)
	assert.Empty(t, componentSet)
}

// TestCollectComponentsWithSource_CollectsSourceComponents tests that it collects components with source.
func TestCollectComponentsWithSource_CollectsSourceComponents(t *testing.T) {
	componentSet := make(map[string]struct{})
	stackData := map[string]any{
		"components": map[string]any{
			"terraform": map[string]any{
				"vpc": map[string]any{
					"source": map[string]any{
						"uri": "github.com/example/vpc",
					},
				},
				"rds": map[string]any{
					"vars": map[string]any{"foo": "bar"}, // No source.
				},
				"eks": map[string]any{
					"source": map[string]any{
						"uri": "github.com/example/eks",
					},
				},
			},
		},
	}
	collectComponentsWithSource(stackData, componentSet)
	assert.Len(t, componentSet, 2)
	assert.Contains(t, componentSet, "vpc")
	assert.Contains(t, componentSet, "eks")
	assert.NotContains(t, componentSet, "rds")
}

// TestCollectComponentsWithSource_SkipsInvalidComponents tests that invalid component data is skipped.
func TestCollectComponentsWithSource_SkipsInvalidComponents(t *testing.T) {
	componentSet := make(map[string]struct{})
	stackData := map[string]any{
		"components": map[string]any{
			"terraform": map[string]any{
				"vpc": map[string]any{
					"source": map[string]any{
						"uri": "github.com/example/vpc",
					},
				},
				"invalid": "not-a-map", // Should be skipped.
			},
		},
	}
	collectComponentsWithSource(stackData, componentSet)
	assert.Len(t, componentSet, 1)
	assert.Contains(t, componentSet, "vpc")
}

// TestComponentArgCompletion_NoArgs tests ComponentArgCompletion when no args provided.
func TestComponentArgCompletion_NoArgs(t *testing.T) {
	cmd := &cobra.Command{Use: "test"}

	// This will fail to load stacks in test environment, returning empty list.
	options, directive := ComponentArgCompletion(cmd, []string{}, "")

	// Without proper stack config, returns empty list.
	assert.Empty(t, options)
	assert.Equal(t, cobra.ShellCompDirectiveNoFileComp, directive)
}

// TestComponentArgCompletion_HasArgs tests ComponentArgCompletion when args already provided.
func TestComponentArgCompletion_HasArgs(t *testing.T) {
	cmd := &cobra.Command{Use: "test"}

	// When args already provided, returns empty.
	options, directive := ComponentArgCompletion(cmd, []string{"vpc"}, "")

	assert.Empty(t, options)
	assert.Equal(t, cobra.ShellCompDirectiveNoFileComp, directive)
}

// TestStackFlagCompletion_WithComponent tests StackFlagCompletion when component is provided.
func TestStackFlagCompletion_WithComponent(t *testing.T) {
	cmd := &cobra.Command{Use: "test"}

	// This will fail to load stacks in test environment, returning empty list.
	options, directive := StackFlagCompletion(cmd, []string{"vpc"}, "")

	// Without proper stack config, returns empty list.
	assert.Empty(t, options)
	assert.Equal(t, cobra.ShellCompDirectiveNoFileComp, directive)
}

// TestStackFlagCompletion_WithoutComponent tests StackFlagCompletion when no component provided.
func TestStackFlagCompletion_WithoutComponent(t *testing.T) {
	cmd := &cobra.Command{Use: "test"}

	// This will fail to load stacks in test environment, returning empty list.
	options, directive := StackFlagCompletion(cmd, []string{}, "")

	// Without proper stack config, returns empty list.
	assert.Empty(t, options)
	assert.Equal(t, cobra.ShellCompDirectiveNoFileComp, directive)
}

// TestStackFlagCompletion_EmptyComponent tests StackFlagCompletion when component is empty string.
func TestStackFlagCompletion_EmptyComponent(t *testing.T) {
	cmd := &cobra.Command{Use: "test"}

	// Empty string component should fall through to listing all stacks.
	options, directive := StackFlagCompletion(cmd, []string{""}, "")

	// Without proper stack config, returns empty list.
	assert.Empty(t, options)
	assert.Equal(t, cobra.ShellCompDirectiveNoFileComp, directive)
}

// TestListStacksWithSourceForComponent_Success tests listStacksWithSourceForComponent with mocked dependencies.
func TestListStacksWithSourceForComponent_Success(t *testing.T) {
	// Save originals and restore after test.
	origInitFunc := initCliConfigForPrompt
	origDescribeFunc := executeDescribeStacksFunc
	defer func() {
		initCliConfigForPrompt = origInitFunc
		executeDescribeStacksFunc = origDescribeFunc
	}()

	// Mock config init to succeed.
	initCliConfigForPrompt = func(configInfo schema.ConfigAndStacksInfo, validate bool) (schema.AtmosConfiguration, error) {
		return schema.AtmosConfiguration{}, nil
	}

	// Mock describe stacks to return stacks with components.
	executeDescribeStacksFunc = func(atmosConfig *schema.AtmosConfiguration, filterByStack string, components, componentTypes, sections []string, ignoreMissingFiles, processTemplates, processYamlFunctions, includeEmptyStacks bool, skip []string, authManager auth.AuthManager) (map[string]any, error) {
		return map[string]any{
			"dev": map[string]any{
				"components": map[string]any{
					"terraform": map[string]any{
						"vpc": map[string]any{
							"source": map[string]any{
								"uri": "github.com/example/vpc",
							},
						},
					},
				},
			},
			"prod": map[string]any{
				"components": map[string]any{
					"terraform": map[string]any{
						"vpc": map[string]any{
							"source": map[string]any{
								"uri": "github.com/example/vpc",
							},
						},
					},
				},
			},
			"staging": map[string]any{
				"components": map[string]any{
					"terraform": map[string]any{
						"rds": map[string]any{
							"vars": map[string]any{"foo": "bar"},
						},
					},
				},
			},
		}, nil
	}

	stacks, err := listStacksWithSourceForComponent("vpc")
	require.NoError(t, err)
	assert.Len(t, stacks, 2)
	assert.Contains(t, stacks, "dev")
	assert.Contains(t, stacks, "prod")
	assert.NotContains(t, stacks, "staging")
}

// TestListStacksWithSourceForComponent_ConfigError tests listStacksWithSourceForComponent when config init fails.
func TestListStacksWithSourceForComponent_ConfigError(t *testing.T) {
	// Save originals and restore after test.
	origInitFunc := initCliConfigForPrompt
	defer func() {
		initCliConfigForPrompt = origInitFunc
	}()

	// Mock config init to fail.
	initCliConfigForPrompt = func(configInfo schema.ConfigAndStacksInfo, validate bool) (schema.AtmosConfiguration, error) {
		return schema.AtmosConfiguration{}, assert.AnError
	}

	stacks, err := listStacksWithSourceForComponent("vpc")
	require.Error(t, err)
	assert.Nil(t, stacks)
}

// TestListStacksWithSourceForComponent_DescribeError tests listStacksWithSourceForComponent when describe fails.
func TestListStacksWithSourceForComponent_DescribeError(t *testing.T) {
	// Save originals and restore after test.
	origInitFunc := initCliConfigForPrompt
	origDescribeFunc := executeDescribeStacksFunc
	defer func() {
		initCliConfigForPrompt = origInitFunc
		executeDescribeStacksFunc = origDescribeFunc
	}()

	// Mock config init to succeed.
	initCliConfigForPrompt = func(configInfo schema.ConfigAndStacksInfo, validate bool) (schema.AtmosConfiguration, error) {
		return schema.AtmosConfiguration{}, nil
	}

	// Mock describe stacks to fail.
	executeDescribeStacksFunc = func(atmosConfig *schema.AtmosConfiguration, filterByStack string, components, componentTypes, sections []string, ignoreMissingFiles, processTemplates, processYamlFunctions, includeEmptyStacks bool, skip []string, authManager auth.AuthManager) (map[string]any, error) {
		return nil, assert.AnError
	}

	stacks, err := listStacksWithSourceForComponent("vpc")
	require.Error(t, err)
	assert.Nil(t, stacks)
}

// TestListStacksWithSource_Success tests listStacksWithSource with mocked dependencies.
func TestListStacksWithSource_Success(t *testing.T) {
	// Save originals and restore after test.
	origInitFunc := initCliConfigForPrompt
	origDescribeFunc := executeDescribeStacksFunc
	defer func() {
		initCliConfigForPrompt = origInitFunc
		executeDescribeStacksFunc = origDescribeFunc
	}()

	// Mock config init to succeed.
	initCliConfigForPrompt = func(configInfo schema.ConfigAndStacksInfo, validate bool) (schema.AtmosConfiguration, error) {
		return schema.AtmosConfiguration{}, nil
	}

	// Mock describe stacks to return stacks with components.
	executeDescribeStacksFunc = func(atmosConfig *schema.AtmosConfiguration, filterByStack string, components, componentTypes, sections []string, ignoreMissingFiles, processTemplates, processYamlFunctions, includeEmptyStacks bool, skip []string, authManager auth.AuthManager) (map[string]any, error) {
		return map[string]any{
			"dev": map[string]any{
				"components": map[string]any{
					"terraform": map[string]any{
						"vpc": map[string]any{
							"source": map[string]any{
								"uri": "github.com/example/vpc",
							},
						},
					},
				},
			},
			"prod": map[string]any{
				"components": map[string]any{
					"terraform": map[string]any{
						"rds": map[string]any{
							"vars": map[string]any{"foo": "bar"},
						},
					},
				},
			},
		}, nil
	}

	stacks, err := listStacksWithSource()
	require.NoError(t, err)
	assert.Len(t, stacks, 1)
	assert.Contains(t, stacks, "dev")
	assert.NotContains(t, stacks, "prod")
}

// TestListStacksWithSource_ConfigError tests listStacksWithSource when config init fails.
func TestListStacksWithSource_ConfigError(t *testing.T) {
	// Save originals and restore after test.
	origInitFunc := initCliConfigForPrompt
	defer func() {
		initCliConfigForPrompt = origInitFunc
	}()

	// Mock config init to fail.
	initCliConfigForPrompt = func(configInfo schema.ConfigAndStacksInfo, validate bool) (schema.AtmosConfiguration, error) {
		return schema.AtmosConfiguration{}, assert.AnError
	}

	stacks, err := listStacksWithSource()
	require.Error(t, err)
	assert.Nil(t, stacks)
}

// TestListStacksWithSource_DescribeError tests listStacksWithSource when describe fails.
func TestListStacksWithSource_DescribeError(t *testing.T) {
	// Save originals and restore after test.
	origInitFunc := initCliConfigForPrompt
	origDescribeFunc := executeDescribeStacksFunc
	defer func() {
		initCliConfigForPrompt = origInitFunc
		executeDescribeStacksFunc = origDescribeFunc
	}()

	// Mock config init to succeed.
	initCliConfigForPrompt = func(configInfo schema.ConfigAndStacksInfo, validate bool) (schema.AtmosConfiguration, error) {
		return schema.AtmosConfiguration{}, nil
	}

	// Mock describe stacks to fail.
	executeDescribeStacksFunc = func(atmosConfig *schema.AtmosConfiguration, filterByStack string, components, componentTypes, sections []string, ignoreMissingFiles, processTemplates, processYamlFunctions, includeEmptyStacks bool, skip []string, authManager auth.AuthManager) (map[string]any, error) {
		return nil, assert.AnError
	}

	stacks, err := listStacksWithSource()
	require.Error(t, err)
	assert.Nil(t, stacks)
}

// TestListComponentsWithSource_Success tests listComponentsWithSource with mocked dependencies.
func TestListComponentsWithSource_Success(t *testing.T) {
	// Save originals and restore after test.
	origInitFunc := initCliConfigForPrompt
	origDescribeFunc := executeDescribeStacksFunc
	defer func() {
		initCliConfigForPrompt = origInitFunc
		executeDescribeStacksFunc = origDescribeFunc
	}()

	// Mock config init to succeed.
	initCliConfigForPrompt = func(configInfo schema.ConfigAndStacksInfo, validate bool) (schema.AtmosConfiguration, error) {
		return schema.AtmosConfiguration{}, nil
	}

	// Mock describe stacks to return stacks with components.
	executeDescribeStacksFunc = func(atmosConfig *schema.AtmosConfiguration, filterByStack string, components, componentTypes, sections []string, ignoreMissingFiles, processTemplates, processYamlFunctions, includeEmptyStacks bool, skip []string, authManager auth.AuthManager) (map[string]any, error) {
		return map[string]any{
			"dev": map[string]any{
				"components": map[string]any{
					"terraform": map[string]any{
						"vpc": map[string]any{
							"source": map[string]any{
								"uri": "github.com/example/vpc",
							},
						},
						"rds": map[string]any{
							"source": map[string]any{
								"uri": "github.com/example/rds",
							},
						},
					},
				},
			},
			"prod": map[string]any{
				"components": map[string]any{
					"terraform": map[string]any{
						"vpc": map[string]any{
							"source": map[string]any{
								"uri": "github.com/example/vpc",
							},
						},
						"eks": map[string]any{
							"vars": map[string]any{"foo": "bar"}, // No source.
						},
					},
				},
			},
		}, nil
	}

	components, err := listComponentsWithSource()
	require.NoError(t, err)
	assert.Len(t, components, 2)
	assert.Contains(t, components, "vpc")
	assert.Contains(t, components, "rds")
	assert.NotContains(t, components, "eks")
}

// TestListComponentsWithSource_ConfigError tests listComponentsWithSource when config init fails.
func TestListComponentsWithSource_ConfigError(t *testing.T) {
	// Save originals and restore after test.
	origInitFunc := initCliConfigForPrompt
	defer func() {
		initCliConfigForPrompt = origInitFunc
	}()

	// Mock config init to fail.
	initCliConfigForPrompt = func(configInfo schema.ConfigAndStacksInfo, validate bool) (schema.AtmosConfiguration, error) {
		return schema.AtmosConfiguration{}, assert.AnError
	}

	components, err := listComponentsWithSource()
	require.Error(t, err)
	assert.Nil(t, components)
}

// TestListComponentsWithSource_DescribeError tests listComponentsWithSource when describe fails.
func TestListComponentsWithSource_DescribeError(t *testing.T) {
	// Save originals and restore after test.
	origInitFunc := initCliConfigForPrompt
	origDescribeFunc := executeDescribeStacksFunc
	defer func() {
		initCliConfigForPrompt = origInitFunc
		executeDescribeStacksFunc = origDescribeFunc
	}()

	// Mock config init to succeed.
	initCliConfigForPrompt = func(configInfo schema.ConfigAndStacksInfo, validate bool) (schema.AtmosConfiguration, error) {
		return schema.AtmosConfiguration{}, nil
	}

	// Mock describe stacks to fail.
	executeDescribeStacksFunc = func(atmosConfig *schema.AtmosConfiguration, filterByStack string, components, componentTypes, sections []string, ignoreMissingFiles, processTemplates, processYamlFunctions, includeEmptyStacks bool, skip []string, authManager auth.AuthManager) (map[string]any, error) {
		return nil, assert.AnError
	}

	components, err := listComponentsWithSource()
	require.Error(t, err)
	assert.Nil(t, components)
}
