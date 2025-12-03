package component

import (
	"testing"

	"github.com/stretchr/testify/assert"

	errUtils "github.com/cloudposse/atmos/errors"
)

// TestHandleComponentMatches_NoMatches tests the case when no component matches are found.
func TestHandleComponentMatches_NoMatches(t *testing.T) {
	matches := []string{}
	componentName := "vpc"
	stack := "dev"
	componentType := "terraform"

	result, err := handleComponentMatches(matches, componentName, stack, componentType)

	assert.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrComponentNotInStack)
	assert.Empty(t, result)
	assert.Contains(t, err.Error(), "not found in stack")
}

// TestHandleComponentMatches_SingleMatch tests the case when exactly one component match is found.
func TestHandleComponentMatches_SingleMatch(t *testing.T) {
	matches := []string{"vpc"}
	componentName := "vpc"
	stack := "dev"
	componentType := "terraform"

	result, err := handleComponentMatches(matches, componentName, stack, componentType)

	assert.NoError(t, err)
	assert.Equal(t, "vpc", result)
}

// TestHandleComponentMatches_SingleMatchAlias tests when a single alias match is found.
func TestHandleComponentMatches_SingleMatchAlias(t *testing.T) {
	matches := []string{"vpc-dev"}
	componentName := "vpc"
	stack := "dev"
	componentType := "terraform"

	result, err := handleComponentMatches(matches, componentName, stack, componentType)

	assert.NoError(t, err)
	assert.Equal(t, "vpc-dev", result)
}

// TestHandleComponentMatches_MultipleMatches_NonTTY tests the case when multiple matches
// are found in a non-interactive terminal. This should return an error.
//
// Note: This test assumes the test environment is non-interactive (no TTY).
// In a real TTY environment, the behavior would be different (interactive prompt).
func TestHandleComponentMatches_MultipleMatches_NonTTY(t *testing.T) {
	matches := []string{"vpc-dev", "vpc-prod", "vpc-staging"}
	componentName := "vpc"
	stack := "dev"
	componentType := "terraform"

	result, err := handleComponentMatches(matches, componentName, stack, componentType)

	// In non-TTY environment (like test), should return ambiguous component error.
	assert.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrAmbiguousComponentPath)
	assert.Empty(t, result)
	// The base sentinel error is "ambiguous component path"
	// The detailed information is in the error's formatted output
	assert.Contains(t, err.Error(), "ambiguous component path")
}

// TestFindComponentMatches_DirectKeyMatch tests finding component by direct key match.
func TestFindComponentMatches_DirectKeyMatch(t *testing.T) {
	typeComponentsMap := map[string]any{
		"vpc": map[string]any{
			"vars": map[string]any{
				"environment": "dev",
			},
		},
	}

	matches := findComponentMatches(typeComponentsMap, "vpc")

	assert.Len(t, matches, 1)
	assert.Equal(t, "vpc", matches[0])
}

// TestFindComponentMatches_ComponentFieldAlias tests finding component via 'component' field.
func TestFindComponentMatches_ComponentFieldAlias(t *testing.T) {
	typeComponentsMap := map[string]any{
		"vpc-dev": map[string]any{
			"component": "vpc",
			"vars": map[string]any{
				"environment": "dev",
			},
		},
	}

	matches := findComponentMatches(typeComponentsMap, "vpc")

	assert.Len(t, matches, 1)
	assert.Equal(t, "vpc-dev", matches[0])
}

// TestFindComponentMatches_MetadataComponentAlias tests finding component via 'metadata.component' field.
func TestFindComponentMatches_MetadataComponentAlias(t *testing.T) {
	typeComponentsMap := map[string]any{
		"vpc-prod": map[string]any{
			"metadata": map[string]any{
				"component": "vpc",
			},
			"vars": map[string]any{
				"environment": "prod",
			},
		},
	}

	matches := findComponentMatches(typeComponentsMap, "vpc")

	assert.Len(t, matches, 1)
	assert.Equal(t, "vpc-prod", matches[0])
}

// TestFindComponentMatches_MultipleAliases tests finding multiple components via aliases.
func TestFindComponentMatches_MultipleAliases(t *testing.T) {
	typeComponentsMap := map[string]any{
		"vpc-dev": map[string]any{
			"component": "vpc",
			"vars": map[string]any{
				"environment": "dev",
			},
		},
		"vpc-prod": map[string]any{
			"metadata": map[string]any{
				"component": "vpc",
			},
			"vars": map[string]any{
				"environment": "prod",
			},
		},
		"vpc-staging": map[string]any{
			"component": "vpc",
			"vars": map[string]any{
				"environment": "staging",
			},
		},
	}

	matches := findComponentMatches(typeComponentsMap, "vpc")

	assert.Len(t, matches, 3)
	// Note: Order is not guaranteed from map iteration, so we check contains.
	assert.Contains(t, matches, "vpc-dev")
	assert.Contains(t, matches, "vpc-prod")
	assert.Contains(t, matches, "vpc-staging")
}

// TestFindComponentMatches_NoMatches tests when no component matches are found.
func TestFindComponentMatches_NoMatches(t *testing.T) {
	typeComponentsMap := map[string]any{
		"eks": map[string]any{
			"vars": map[string]any{
				"cluster_name": "my-cluster",
			},
		},
	}

	matches := findComponentMatches(typeComponentsMap, "vpc")

	assert.Empty(t, matches)
}

// TestFindComponentMatches_IgnoresInvalidConfig tests that invalid component configs are skipped.
func TestFindComponentMatches_IgnoresInvalidConfig(t *testing.T) {
	typeComponentsMap := map[string]any{
		"vpc-dev": "invalid-config-not-a-map",
		"vpc-prod": map[string]any{
			"component": "vpc",
		},
	}

	matches := findComponentMatches(typeComponentsMap, "vpc")

	// Should only find vpc-prod, vpc-dev is skipped due to invalid format.
	assert.Len(t, matches, 1)
	assert.Equal(t, "vpc-prod", matches[0])
}

// TestFindComponentMatches_DirectMatchHasPriority tests that direct match returns only the direct match.
func TestFindComponentMatches_DirectMatchHasPriority(t *testing.T) {
	typeComponentsMap := map[string]any{
		"vpc": map[string]any{
			"vars": map[string]any{},
		},
		"vpc-alias": map[string]any{
			"component": "vpc",
		},
	}

	matches := findComponentMatches(typeComponentsMap, "vpc")

	// Direct match should only return the direct match, not aliases.
	assert.Len(t, matches, 1)
	assert.Equal(t, "vpc", matches[0])
}

// TestFindComponentMatches_NilComponentField tests that nil component field is handled.
func TestFindComponentMatches_NilComponentField(t *testing.T) {
	typeComponentsMap := map[string]any{
		"vpc-dev": map[string]any{
			"component": nil, // nil component field
			"vars":      map[string]any{},
		},
	}

	matches := findComponentMatches(typeComponentsMap, "vpc")

	// Should return empty matches - nil component field shouldn't match.
	assert.Empty(t, matches)
}

// TestFindComponentMatches_NilMetadataComponent tests that nil metadata.component is handled.
func TestFindComponentMatches_NilMetadataComponent(t *testing.T) {
	typeComponentsMap := map[string]any{
		"vpc-dev": map[string]any{
			"metadata": map[string]any{
				"component": nil, // nil metadata component
			},
		},
	}

	matches := findComponentMatches(typeComponentsMap, "vpc")

	// Should return empty matches - nil metadata component shouldn't match.
	assert.Empty(t, matches)
}

// TestExtractComponentsSection_EmptyStack tests extraction from empty stack config.
func TestExtractComponentsSection_EmptyStack(t *testing.T) {
	stackConfig := map[string]any{}

	result, err := extractComponentsSection(stackConfig, "terraform", "dev")

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "has no components section")
}

// TestExtractComponentsSection_ComponentsNotMap tests invalid components type.
func TestExtractComponentsSection_ComponentsNotMap(t *testing.T) {
	stackConfig := map[string]any{
		"components": "not-a-map",
	}

	result, err := extractComponentsSection(stackConfig, "terraform", "dev")

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "invalid components section")
}

// TestExtractComponentsSection_MissingComponentType tests missing component type section.
func TestExtractComponentsSection_MissingComponentType(t *testing.T) {
	stackConfig := map[string]any{
		"components": map[string]any{
			"helmfile": map[string]any{},
		},
	}

	result, err := extractComponentsSection(stackConfig, "terraform", "dev")

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "has no terraform components")
}

// TestExtractComponentsSection_ComponentTypeNotMap tests invalid component type section.
func TestExtractComponentsSection_ComponentTypeNotMap(t *testing.T) {
	stackConfig := map[string]any{
		"components": map[string]any{
			"terraform": "not-a-map",
		},
	}

	result, err := extractComponentsSection(stackConfig, "terraform", "dev")

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "invalid terraform components section")
}

// TestHandleNoMatches_DifferentComponentTypes tests error messages for different component types.
func TestHandleNoMatches_DifferentComponentTypes(t *testing.T) {
	tests := []struct {
		name          string
		componentName string
		stack         string
		componentType string
	}{
		{
			name:          "terraform component",
			componentName: "vpc",
			stack:         "prod-us-east-1",
			componentType: "terraform",
		},
		{
			name:          "helmfile component",
			componentName: "nginx",
			stack:         "dev-us-west-2",
			componentType: "helmfile",
		},
		{
			name:          "packer component",
			componentName: "base-ami",
			stack:         "shared",
			componentType: "packer",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := handleNoMatches(tt.componentName, tt.stack, tt.componentType)

			assert.Error(t, err)
			assert.ErrorIs(t, err, errUtils.ErrComponentNotInStack)
			assert.Empty(t, result)
			assert.Contains(t, err.Error(), "not found in stack")
		})
	}
}

// TestBuildAmbiguousComponentError_VariousMatchCounts tests error building with different match counts.
func TestBuildAmbiguousComponentError_VariousMatchCounts(t *testing.T) {
	tests := []struct {
		name          string
		matches       []string
		componentName string
		stack         string
		componentType string
	}{
		{
			name:          "two matches terraform",
			matches:       []string{"vpc-1", "vpc-2"},
			componentName: "vpc",
			stack:         "dev",
			componentType: "terraform",
		},
		{
			name:          "three matches helmfile",
			matches:       []string{"app-1", "app-2", "app-3"},
			componentName: "app",
			stack:         "prod",
			componentType: "helmfile",
		},
		{
			name:          "many matches",
			matches:       []string{"comp-1", "comp-2", "comp-3", "comp-4", "comp-5"},
			componentName: "comp",
			stack:         "staging",
			componentType: "terraform",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := buildAmbiguousComponentError(tt.matches, tt.componentName, tt.stack, tt.componentType)

			assert.Error(t, err)
			assert.ErrorIs(t, err, errUtils.ErrAmbiguousComponentPath)
			assert.Contains(t, err.Error(), "ambiguous component path")
			// The formatted error message contains the hints which include the match information.
			// Use the full formatted error to verify the matches are included.
			formattedErr := errUtils.Format(err, errUtils.DefaultFormatterConfig())
			for _, match := range tt.matches {
				assert.Contains(t, formattedErr, match)
			}
		})
	}
}
