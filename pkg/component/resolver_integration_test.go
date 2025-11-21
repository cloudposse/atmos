package component

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

// Helper to create a minimal AtmosConfiguration for testing.
func newTestAtmosConfig() *schema.AtmosConfiguration {
	return &schema.AtmosConfiguration{
		BasePath: "/test/path",
		Components: schema.Components{
			Terraform: schema.Terraform{
				BasePath: "components/terraform",
			},
			Helmfile: schema.Helmfile{
				BasePath: "components/helmfile",
			},
		},
	}
}

// Helper to create a stack config with terraform components.
func createStackConfig(components map[string]any) map[string]any {
	return map[string]any{
		"components": map[string]any{
			"terraform": components,
		},
	}
}

// TestNewResolver tests resolver creation.
func TestNewResolver(t *testing.T) {
	mockLoader := NewMockStackLoader(nil)
	resolver := NewResolver(mockLoader)

	assert.NotNil(t, resolver)
	assert.Equal(t, mockLoader, resolver.stackLoader)
}

// TestLoadStackConfig tests the loadStackConfig method.
func TestLoadStackConfig(t *testing.T) {
	tests := []struct {
		name        string
		stacksMap   map[string]any
		loaderErr   error
		stack       string
		wantErr     bool
		errContains string
	}{
		{
			name: "valid stack config",
			stacksMap: map[string]any{
				"dev": map[string]any{
					"components": map[string]any{
						"terraform": map[string]any{
							"vpc": map[string]any{},
						},
					},
				},
			},
			stack:   "dev",
			wantErr: false,
		},
		{
			name:        "stack loader error",
			stacksMap:   nil,
			loaderErr:   errors.New("failed to load stacks"),
			stack:       "dev",
			wantErr:     true,
			errContains: "stack not found", // Base error message
		},
		{
			name: "stack not found",
			stacksMap: map[string]any{
				"prod": map[string]any{},
			},
			stack:       "dev",
			wantErr:     true,
			errContains: "not found",
		},
		{
			name: "invalid stack config format",
			stacksMap: map[string]any{
				"dev": "not a map",
			},
			stack:       "dev",
			wantErr:     true,
			errContains: "invalid stack configuration",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var mockLoader *MockStackLoader
			if tt.loaderErr != nil {
				mockLoader = NewMockStackLoaderWithError(tt.loaderErr)
			} else {
				mockLoader = NewMockStackLoader(tt.stacksMap)
			}

			resolver := NewResolver(mockLoader)
			atmosConfig := newTestAtmosConfig()

			result, err := resolver.loadStackConfig(atmosConfig, tt.stack, "test-component")

			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errContains)
				assert.Nil(t, result)
			} else {
				require.NoError(t, err)
				assert.NotNil(t, result)
			}
		})
	}
}

// TestValidateComponentInStack tests the validateComponentInStack method.
func TestValidateComponentInStack(t *testing.T) {
	tests := []struct {
		name          string
		stacksMap     map[string]any
		componentName string
		stack         string
		componentType string
		wantErr       bool
		errContains   string
		expectedKey   string
	}{
		{
			name: "direct component match",
			stacksMap: map[string]any{
				"dev": createStackConfig(map[string]any{
					"vpc": map[string]any{
						"vars": map[string]any{"env": "dev"},
					},
				}),
			},
			componentName: "vpc",
			stack:         "dev",
			componentType: "terraform",
			wantErr:       false,
			expectedKey:   "vpc",
		},
		{
			name: "component alias match",
			stacksMap: map[string]any{
				"dev": createStackConfig(map[string]any{
					"vpc-dev": map[string]any{
						"component": "vpc",
						"vars":      map[string]any{"env": "dev"},
					},
				}),
			},
			componentName: "vpc",
			stack:         "dev",
			componentType: "terraform",
			wantErr:       false,
			expectedKey:   "vpc-dev",
		},
		{
			name: "component not found in stack",
			stacksMap: map[string]any{
				"dev": createStackConfig(map[string]any{
					"eks": map[string]any{},
				}),
			},
			componentName: "vpc",
			stack:         "dev",
			componentType: "terraform",
			wantErr:       true,
			errContains:   "not found in stack",
		},
		{
			name: "no terraform components in stack",
			stacksMap: map[string]any{
				"dev": map[string]any{
					"components": map[string]any{
						"helmfile": map[string]any{
							"nginx": map[string]any{},
						},
					},
				},
			},
			componentName: "vpc",
			stack:         "dev",
			componentType: "terraform",
			wantErr:       true,
			errContains:   "has no terraform components",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockLoader := NewMockStackLoader(tt.stacksMap)
			resolver := NewResolver(mockLoader)
			atmosConfig := newTestAtmosConfig()

			result, err := resolver.validateComponentInStack(atmosConfig, tt.componentName, tt.stack, tt.componentType)

			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errContains)
				assert.Empty(t, result)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expectedKey, result)
			}
		})
	}
}

// TestBuildAmbiguousComponentError tests the error builder for ambiguous paths.
func TestBuildAmbiguousComponentError(t *testing.T) {
	matches := []string{"vpc-dev", "vpc-prod", "vpc-staging"}
	componentName := "vpc"
	stack := "dev"
	componentType := "terraform"

	err := buildAmbiguousComponentError(matches, componentName, stack, componentType)

	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrAmbiguousComponentPath)
	// The base error is "ambiguous component path" - detailed info is in hints.
	assert.Contains(t, err.Error(), "ambiguous component path")
}

// TestHandleNoMatches tests the handleNoMatches function.
func TestHandleNoMatches(t *testing.T) {
	result, err := handleNoMatches("vpc", "dev", "terraform")

	assert.Empty(t, result)
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrComponentNotInStack)
	// The base error is "component not found in stack configuration" - detailed info is in hints.
	assert.Contains(t, err.Error(), "component not found in stack")
}

// TestFindComponentMatches_EmptyMap tests with an empty component map.
func TestFindComponentMatches_EmptyMap(t *testing.T) {
	typeComponentsMap := map[string]any{}

	matches := findComponentMatches(typeComponentsMap, "vpc")

	assert.Empty(t, matches)
}

// TestFindComponentMatches_NilMetadata tests component with nil metadata.
func TestFindComponentMatches_NilMetadata(t *testing.T) {
	typeComponentsMap := map[string]any{
		"vpc-dev": map[string]any{
			"metadata": nil, // nil metadata should not cause panic
			"vars":     map[string]any{},
		},
	}

	matches := findComponentMatches(typeComponentsMap, "vpc")

	assert.Empty(t, matches)
}

// TestFindComponentMatches_InvalidMetadataType tests component with invalid metadata type.
func TestFindComponentMatches_InvalidMetadataType(t *testing.T) {
	typeComponentsMap := map[string]any{
		"vpc-dev": map[string]any{
			"metadata": "not a map", // invalid type should be handled gracefully
			"vars":     map[string]any{},
		},
	}

	matches := findComponentMatches(typeComponentsMap, "vpc")

	assert.Empty(t, matches)
}

// TestFindComponentMatches_InvalidComponentFieldType tests invalid component field type.
func TestFindComponentMatches_InvalidComponentFieldType(t *testing.T) {
	typeComponentsMap := map[string]any{
		"vpc-dev": map[string]any{
			"component": 123, // not a string
			"vars":      map[string]any{},
		},
	}

	matches := findComponentMatches(typeComponentsMap, "vpc")

	assert.Empty(t, matches)
}

// TestExtractComponentsSection_EmptyComponentsMap tests with empty components map.
func TestExtractComponentsSection_EmptyComponentsMap(t *testing.T) {
	stackConfig := map[string]any{
		"components": map[string]any{},
	}

	result, err := extractComponentsSection(stackConfig, "terraform", "test-stack")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "has no terraform components")
	assert.Nil(t, result)
}

// TestHandleComponentMatches_NilMatches tests with nil matches slice.
func TestHandleComponentMatches_NilMatches(t *testing.T) {
	var matches []string // nil slice

	result, err := handleComponentMatches(matches, "vpc", "dev", "terraform")

	assert.Empty(t, result)
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrComponentNotInStack)
}

// TestExtractComponentsSection_ValidComponents tests extracting valid component section.
func TestExtractComponentsSection_ValidComponents(t *testing.T) {
	stackConfig := map[string]any{
		"components": map[string]any{
			"terraform": map[string]any{
				"vpc": map[string]any{},
			},
		},
	}

	result, err := extractComponentsSection(stackConfig, "terraform", "dev")

	require.NoError(t, err)
	assert.NotNil(t, result)
	_, hasVPC := result["vpc"]
	assert.True(t, hasVPC)
}

// TestExtractComponentsSection_NoComponents tests stack with no components section.
func TestExtractComponentsSection_NoComponents(t *testing.T) {
	stackConfig := map[string]any{
		"vars": map[string]any{},
	}

	result, err := extractComponentsSection(stackConfig, "terraform", "dev")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "has no components section")
	assert.Nil(t, result)
}

// TestExtractComponentsSection_InvalidComponentsType tests components field with wrong type.
func TestExtractComponentsSection_InvalidComponentsType(t *testing.T) {
	stackConfig := map[string]any{
		"components": "not a map",
	}

	result, err := extractComponentsSection(stackConfig, "terraform", "dev")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid components section")
	assert.Nil(t, result)
}

// TestExtractComponentsSection_HelmfileComponents tests extracting helmfile components.
func TestExtractComponentsSection_HelmfileComponents(t *testing.T) {
	stackConfig := map[string]any{
		"components": map[string]any{
			"helmfile": map[string]any{
				"nginx": map[string]any{},
			},
		},
	}

	result, err := extractComponentsSection(stackConfig, "helmfile", "dev")

	require.NoError(t, err)
	assert.NotNil(t, result)
	_, hasNginx := result["nginx"]
	assert.True(t, hasNginx)
}

// TestBuildAmbiguousComponentError_MultipleMatches tests error with multiple matches.
func TestBuildAmbiguousComponentError_MultipleMatches(t *testing.T) {
	matches := []string{"vpc-dev", "vpc-prod"}

	err := buildAmbiguousComponentError(matches, "vpc", "dev", "terraform")

	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrAmbiguousComponentPath)
	assert.Contains(t, err.Error(), "ambiguous component path")
}

// TestValidateComponentInStack_MultipleMatches tests behavior with multiple matching components in non-TTY.
func TestValidateComponentInStack_MultipleMatches(t *testing.T) {
	stacksMap := map[string]any{
		"dev": createStackConfig(map[string]any{
			"vpc-dev": map[string]any{
				"component": "vpc",
			},
			"vpc-staging": map[string]any{
				"component": "vpc",
			},
		}),
	}

	mockLoader := NewMockStackLoader(stacksMap)
	resolver := NewResolver(mockLoader)
	atmosConfig := newTestAtmosConfig()

	// In non-TTY environment, should return ambiguous error.
	result, err := resolver.validateComponentInStack(atmosConfig, "vpc", "dev", "terraform")

	// Either succeeds with first match or fails with ambiguous error (depends on TTY).
	if err != nil {
		assert.ErrorIs(t, err, errUtils.ErrAmbiguousComponentPath)
	} else {
		// If it didn't error, it should have picked one.
		assert.Contains(t, []string{"vpc-dev", "vpc-staging"}, result)
	}
}
