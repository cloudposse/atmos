package component

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
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

// TestFindComponentMatches_AllMatchesCollected tests that all matches are collected for ambiguity detection.
func TestFindComponentMatches_AllMatchesCollected(t *testing.T) {
	typeComponentsMap := map[string]any{
		"vpc": map[string]any{
			"vars": map[string]any{},
		},
		"vpc-alias": map[string]any{
			"component": "vpc",
		},
	}

	matches := findComponentMatches(typeComponentsMap, "vpc")

	// All matches should be collected to detect ambiguous component paths.
	// This enables proper error messages when multiple Atmos components reference the same terraform folder.
	assert.Len(t, matches, 2)
	assert.Contains(t, matches, "vpc")
	assert.Contains(t, matches, "vpc-alias")
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

// mockStackLoader is a test implementation of StackLoader.
type mockStackLoader struct {
	stacksMap map[string]any
	err       error
}

func (m *mockStackLoader) FindStacksMap(atmosConfig *schema.AtmosConfiguration, ignoreMissingFiles bool) (
	map[string]any,
	map[string]map[string]any,
	error,
) {
	if m.err != nil {
		return nil, nil, m.err
	}
	return m.stacksMap, nil, nil
}

// TestNewResolver_WithMockLoader tests the NewResolver constructor with a mock loader.
func TestNewResolver_WithMockLoader(t *testing.T) {
	loader := &mockStackLoader{}
	resolver := NewResolver(loader)

	assert.NotNil(t, resolver)
	assert.Equal(t, loader, resolver.stackLoader)
}

// TestResolveComponentFromPath_Success tests successful path resolution.
func TestResolveComponentFromPath_Success(t *testing.T) {
	// Create a temporary directory structure for testing.
	tmpDir := t.TempDir()
	terraformBase := filepath.Join(tmpDir, "components", "terraform")
	componentDir := filepath.Join(terraformBase, "vpc")

	require.NoError(t, os.MkdirAll(componentDir, 0o755))

	// Change to the component directory.
	t.Chdir(componentDir)

	atmosConfig := &schema.AtmosConfiguration{
		BasePath:                 tmpDir,
		TerraformDirAbsolutePath: terraformBase,
		Components: schema.Components{
			Terraform: schema.Terraform{
				BasePath: "components/terraform",
			},
		},
	}

	stacksMap := map[string]any{
		"dev": map[string]any{
			"components": map[string]any{
				"terraform": map[string]any{
					"vpc": map[string]any{
						"vars": map[string]any{
							"environment": "dev",
						},
					},
				},
			},
		},
	}

	loader := &mockStackLoader{stacksMap: stacksMap}
	resolver := NewResolver(loader)

	// Test with "." (current directory).
	result, err := resolver.ResolveComponentFromPath(atmosConfig, ".", "dev", "terraform")

	require.NoError(t, err)
	assert.Equal(t, "vpc", result)
}

// TestResolveComponentFromPath_TypeMismatch tests component type mismatch.
func TestResolveComponentFromPath_TypeMismatch(t *testing.T) {
	// Create a temporary directory structure for testing.
	tmpDir := t.TempDir()
	terraformBase := filepath.Join(tmpDir, "components", "terraform")
	componentDir := filepath.Join(terraformBase, "vpc")

	require.NoError(t, os.MkdirAll(componentDir, 0o755))

	// Change to the component directory.
	t.Chdir(componentDir)

	atmosConfig := &schema.AtmosConfiguration{
		BasePath:                 tmpDir,
		TerraformDirAbsolutePath: terraformBase,
		Components: schema.Components{
			Terraform: schema.Terraform{
				BasePath: "components/terraform",
			},
		},
	}

	loader := &mockStackLoader{stacksMap: map[string]any{}}
	resolver := NewResolver(loader)

	// Test with wrong expected type.
	_, err := resolver.ResolveComponentFromPath(atmosConfig, ".", "dev", "helmfile")

	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrComponentTypeMismatch)
}

// TestResolveComponentFromPath_NoStack tests resolution without stack validation.
func TestResolveComponentFromPath_NoStack(t *testing.T) {
	// Create a temporary directory structure for testing.
	tmpDir := t.TempDir()
	terraformBase := filepath.Join(tmpDir, "components", "terraform")
	componentDir := filepath.Join(terraformBase, "vpc")

	require.NoError(t, os.MkdirAll(componentDir, 0o755))

	// Change to the component directory.
	t.Chdir(componentDir)

	atmosConfig := &schema.AtmosConfiguration{
		BasePath:                 tmpDir,
		TerraformDirAbsolutePath: terraformBase,
		Components: schema.Components{
			Terraform: schema.Terraform{
				BasePath: "components/terraform",
			},
		},
	}

	loader := &mockStackLoader{stacksMap: map[string]any{}}
	resolver := NewResolver(loader)

	// Test without stack (empty string) - should not validate against stack.
	result, err := resolver.ResolveComponentFromPath(atmosConfig, ".", "", "terraform")

	require.NoError(t, err)
	assert.Equal(t, "vpc", result)
}

// TestResolveComponentFromPathWithoutTypeCheck tests resolution without type check.
func TestResolveComponentFromPathWithoutTypeCheck(t *testing.T) {
	// Create a temporary directory structure for testing.
	tmpDir := t.TempDir()
	terraformBase := filepath.Join(tmpDir, "components", "terraform")
	componentDir := filepath.Join(terraformBase, "vpc")

	require.NoError(t, os.MkdirAll(componentDir, 0o755))

	// Change to the component directory.
	t.Chdir(componentDir)

	atmosConfig := &schema.AtmosConfiguration{
		BasePath:                 tmpDir,
		TerraformDirAbsolutePath: terraformBase,
		Components: schema.Components{
			Terraform: schema.Terraform{
				BasePath: "components/terraform",
			},
		},
	}

	stacksMap := map[string]any{
		"dev": map[string]any{
			"components": map[string]any{
				"terraform": map[string]any{
					"vpc": map[string]any{
						"vars": map[string]any{},
					},
				},
			},
		},
	}

	loader := &mockStackLoader{stacksMap: stacksMap}
	resolver := NewResolver(loader)

	// Test resolution without type check.
	result, err := resolver.ResolveComponentFromPathWithoutTypeCheck(atmosConfig, ".", "dev")

	require.NoError(t, err)
	assert.Equal(t, "vpc", result)
}

// TestResolveComponentFromPathWithoutTypeCheck_NoStack tests resolution without type check and no stack.
func TestResolveComponentFromPathWithoutTypeCheck_NoStack(t *testing.T) {
	// Create a temporary directory structure for testing.
	tmpDir := t.TempDir()
	terraformBase := filepath.Join(tmpDir, "components", "terraform")
	componentDir := filepath.Join(terraformBase, "vpc")

	require.NoError(t, os.MkdirAll(componentDir, 0o755))

	// Change to the component directory.
	t.Chdir(componentDir)

	atmosConfig := &schema.AtmosConfiguration{
		BasePath:                 tmpDir,
		TerraformDirAbsolutePath: terraformBase,
		Components: schema.Components{
			Terraform: schema.Terraform{
				BasePath: "components/terraform",
			},
		},
	}

	loader := &mockStackLoader{stacksMap: map[string]any{}}
	resolver := NewResolver(loader)

	// Test without stack - should not validate against stack.
	result, err := resolver.ResolveComponentFromPathWithoutTypeCheck(atmosConfig, ".", "")

	require.NoError(t, err)
	assert.Equal(t, "vpc", result)
}

// TestResolveComponentFromPathWithoutValidation tests resolution without stack validation.
func TestResolveComponentFromPathWithoutValidation(t *testing.T) {
	// Create a temporary directory structure for testing.
	tmpDir := t.TempDir()
	terraformBase := filepath.Join(tmpDir, "components", "terraform")
	componentDir := filepath.Join(terraformBase, "vpc")

	require.NoError(t, os.MkdirAll(componentDir, 0o755))

	// Change to the component directory.
	t.Chdir(componentDir)

	atmosConfig := &schema.AtmosConfiguration{
		BasePath:                 tmpDir,
		TerraformDirAbsolutePath: terraformBase,
		Components: schema.Components{
			Terraform: schema.Terraform{
				BasePath: "components/terraform",
			},
		},
	}

	loader := &mockStackLoader{stacksMap: map[string]any{}}
	resolver := NewResolver(loader)

	// Test resolution without stack validation.
	result, err := resolver.ResolveComponentFromPathWithoutValidation(atmosConfig, ".", "terraform")

	require.NoError(t, err)
	assert.Equal(t, "vpc", result)
}

// TestResolveComponentFromPathWithoutValidation_TypeMismatch tests type mismatch.
func TestResolveComponentFromPathWithoutValidation_TypeMismatch(t *testing.T) {
	// Create a temporary directory structure for testing.
	tmpDir := t.TempDir()
	terraformBase := filepath.Join(tmpDir, "components", "terraform")
	componentDir := filepath.Join(terraformBase, "vpc")

	require.NoError(t, os.MkdirAll(componentDir, 0o755))

	// Change to the component directory.
	t.Chdir(componentDir)

	atmosConfig := &schema.AtmosConfiguration{
		BasePath:                 tmpDir,
		TerraformDirAbsolutePath: terraformBase,
		Components: schema.Components{
			Terraform: schema.Terraform{
				BasePath: "components/terraform",
			},
		},
	}

	loader := &mockStackLoader{stacksMap: map[string]any{}}
	resolver := NewResolver(loader)

	// Test with wrong expected type.
	_, err := resolver.ResolveComponentFromPathWithoutValidation(atmosConfig, ".", "helmfile")

	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrComponentTypeMismatch)
}

// TestResolveComponentFromPath_InvalidPath tests resolution with invalid path.
func TestResolveComponentFromPath_InvalidPath(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{
		BasePath: "/tmp/nonexistent",
		Components: schema.Components{
			Terraform: schema.Terraform{
				BasePath: "components/terraform",
			},
		},
	}

	loader := &mockStackLoader{stacksMap: map[string]any{}}
	resolver := NewResolver(loader)

	// Test with path not in component directories.
	_, err := resolver.ResolveComponentFromPath(atmosConfig, "/tmp/random", "dev", "terraform")

	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrPathNotInComponentDir)
}

// TestResolveComponentFromPath_StackNotFound tests resolution when stack is not found.
func TestResolveComponentFromPath_StackNotFound(t *testing.T) {
	// Create a temporary directory structure for testing.
	tmpDir := t.TempDir()
	terraformBase := filepath.Join(tmpDir, "components", "terraform")
	componentDir := filepath.Join(terraformBase, "vpc")

	require.NoError(t, os.MkdirAll(componentDir, 0o755))

	// Change to the component directory.
	t.Chdir(componentDir)

	atmosConfig := &schema.AtmosConfiguration{
		BasePath:                 tmpDir,
		TerraformDirAbsolutePath: terraformBase,
		Components: schema.Components{
			Terraform: schema.Terraform{
				BasePath: "components/terraform",
			},
		},
	}

	// Empty stacks map - stack not found.
	loader := &mockStackLoader{stacksMap: map[string]any{}}
	resolver := NewResolver(loader)

	_, err := resolver.ResolveComponentFromPath(atmosConfig, ".", "nonexistent-stack", "terraform")

	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrStackNotFound)
}

// TestResolveComponentFromPath_ComponentNotInStack tests resolution when component is not in stack.
func TestResolveComponentFromPath_ComponentNotInStack(t *testing.T) {
	// Create a temporary directory structure for testing.
	tmpDir := t.TempDir()
	terraformBase := filepath.Join(tmpDir, "components", "terraform")
	componentDir := filepath.Join(terraformBase, "vpc")

	require.NoError(t, os.MkdirAll(componentDir, 0o755))

	// Change to the component directory.
	t.Chdir(componentDir)

	atmosConfig := &schema.AtmosConfiguration{
		BasePath:                 tmpDir,
		TerraformDirAbsolutePath: terraformBase,
		Components: schema.Components{
			Terraform: schema.Terraform{
				BasePath: "components/terraform",
			},
		},
	}

	// Stack exists but component is different.
	stacksMap := map[string]any{
		"dev": map[string]any{
			"components": map[string]any{
				"terraform": map[string]any{
					"eks": map[string]any{}, // Different component.
				},
			},
		},
	}

	loader := &mockStackLoader{stacksMap: stacksMap}
	resolver := NewResolver(loader)

	_, err := resolver.ResolveComponentFromPath(atmosConfig, ".", "dev", "terraform")

	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrComponentNotInStack)
}

// TestLoadStackConfig_LoadError tests loadStackConfig when loader returns an error.
func TestLoadStackConfig_LoadError(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{}
	testErr := errUtils.ErrStackNotFound

	loader := &mockStackLoader{err: testErr}
	resolver := NewResolver(loader)

	_, err := resolver.loadStackConfig(atmosConfig, "dev", "vpc")

	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrStackNotFound)
}

// TestLoadStackConfig_InvalidStackConfig tests loadStackConfig with invalid stack config type.
func TestLoadStackConfig_InvalidStackConfig(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{}

	// Stack config is not a map.
	stacksMap := map[string]any{
		"dev": "not-a-map",
	}

	loader := &mockStackLoader{stacksMap: stacksMap}
	resolver := NewResolver(loader)

	_, err := resolver.loadStackConfig(atmosConfig, "dev", "vpc")

	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrInvalidStackConfiguration)
}

// TestValidateComponentInStack_AliasMatch tests validation with component alias matching.
func TestValidateComponentInStack_AliasMatch(t *testing.T) {
	// Create a temporary directory structure for testing.
	tmpDir := t.TempDir()
	terraformBase := filepath.Join(tmpDir, "components", "terraform")
	componentDir := filepath.Join(terraformBase, "vpc")

	require.NoError(t, os.MkdirAll(componentDir, 0o755))

	atmosConfig := &schema.AtmosConfiguration{
		BasePath:                 tmpDir,
		TerraformDirAbsolutePath: terraformBase,
		Components: schema.Components{
			Terraform: schema.Terraform{
				BasePath: "components/terraform",
			},
		},
	}

	// Stack with aliased component.
	stacksMap := map[string]any{
		"dev": map[string]any{
			"components": map[string]any{
				"terraform": map[string]any{
					"vpc-dev": map[string]any{
						"component": "vpc", // Alias to "vpc".
						"vars":      map[string]any{},
					},
				},
			},
		},
	}

	loader := &mockStackLoader{stacksMap: stacksMap}
	resolver := NewResolver(loader)

	// Validate "vpc" should resolve to "vpc-dev" alias.
	result, err := resolver.validateComponentInStack(atmosConfig, "vpc", "dev", "terraform")

	require.NoError(t, err)
	assert.Equal(t, "vpc-dev", result)
}

// TestResolver_ConcurrentResolution tests that the resolver is safe for concurrent use.
func TestResolver_ConcurrentResolution(t *testing.T) {
	// Create a temporary directory structure for testing.
	tmpDir := t.TempDir()
	terraformBase := filepath.Join(tmpDir, "components", "terraform")
	vpcDir := filepath.Join(terraformBase, "vpc")
	eksDir := filepath.Join(terraformBase, "eks")
	rdsDir := filepath.Join(terraformBase, "rds")

	require.NoError(t, os.MkdirAll(vpcDir, 0o755))
	require.NoError(t, os.MkdirAll(eksDir, 0o755))
	require.NoError(t, os.MkdirAll(rdsDir, 0o755))

	atmosConfig := &schema.AtmosConfiguration{
		BasePath:                 tmpDir,
		TerraformDirAbsolutePath: terraformBase,
		Components: schema.Components{
			Terraform: schema.Terraform{
				BasePath: "components/terraform",
			},
		},
	}

	stacksMap := map[string]any{
		"dev": map[string]any{
			"components": map[string]any{
				"terraform": map[string]any{
					"vpc": map[string]any{"vars": map[string]any{}},
					"eks": map[string]any{"vars": map[string]any{}},
					"rds": map[string]any{"vars": map[string]any{}},
				},
			},
		},
	}

	loader := &mockStackLoader{stacksMap: stacksMap}
	resolver := NewResolver(loader)

	// Run multiple goroutines concurrently.
	const numGoroutines = 10
	var wg sync.WaitGroup
	errors := make(chan error, numGoroutines*3)

	for i := 0; i < numGoroutines; i++ {
		wg.Add(3)

		// Goroutine resolving vpc.
		go func() {
			defer wg.Done()
			_, err := resolver.validateComponentInStack(atmosConfig, "vpc", "dev", "terraform")
			if err != nil {
				errors <- err
			}
		}()

		// Goroutine resolving eks.
		go func() {
			defer wg.Done()
			_, err := resolver.validateComponentInStack(atmosConfig, "eks", "dev", "terraform")
			if err != nil {
				errors <- err
			}
		}()

		// Goroutine resolving rds.
		go func() {
			defer wg.Done()
			_, err := resolver.validateComponentInStack(atmosConfig, "rds", "dev", "terraform")
			if err != nil {
				errors <- err
			}
		}()
	}

	wg.Wait()
	close(errors)

	// Check that no errors occurred.
	for err := range errors {
		t.Errorf("Concurrent resolution error: %v", err)
	}
}

// TestResolver_VeryLongComponentPaths tests resolution with very long component paths.
func TestResolver_VeryLongComponentPaths(t *testing.T) {
	// Skip on Windows due to 260-char path limit.
	if runtime.GOOS == "windows" {
		t.Skip("Skipping long path test on Windows")
	}

	// Create a deeply nested path structure.
	tmpDir := t.TempDir()
	terraformBase := filepath.Join(tmpDir, "components", "terraform")

	// Create a path with 10 nested directories.
	deepPath := terraformBase
	for i := 0; i < 10; i++ {
		deepPath = filepath.Join(deepPath, "level"+strings.Repeat("x", 20))
	}

	require.NoError(t, os.MkdirAll(deepPath, 0o755))

	// Change to the deep directory.
	t.Chdir(deepPath)

	atmosConfig := &schema.AtmosConfiguration{
		BasePath:                 tmpDir,
		TerraformDirAbsolutePath: terraformBase,
		Components: schema.Components{
			Terraform: schema.Terraform{
				BasePath: "components/terraform",
			},
		},
	}

	loader := &mockStackLoader{stacksMap: map[string]any{}}
	resolver := NewResolver(loader)

	// Test resolution from deep path - should extract component name from path.
	// It won't find it in stack (empty stacks), but should not panic.
	result, err := resolver.ResolveComponentFromPathWithoutValidation(atmosConfig, ".", "terraform")

	// Should succeed in extracting component from path.
	require.NoError(t, err)
	// The component name should be the deepest folder.
	assert.Contains(t, result, "level")
}

// TestResolver_UnicodeComponentNames tests resolution with Unicode characters in paths.
func TestResolver_UnicodeComponentNames(t *testing.T) {
	// Skip on Windows due to potential filesystem encoding issues.
	if runtime.GOOS == "windows" {
		t.Skip("Skipping Unicode path test on Windows")
	}

	tmpDir := t.TempDir()
	terraformBase := filepath.Join(tmpDir, "components", "terraform")

	// Test various Unicode component names.
	unicodeNames := []string{
		"组件",        // Chinese for "component"
		"компонент", // Russian for "component"
		"コンポーネント",   // Japanese for "component"
		"café",      // Accented Latin
		"emoji-🚀",   // Emoji
	}

	for _, name := range unicodeNames {
		t.Run(name, func(t *testing.T) {
			componentDir := filepath.Join(terraformBase, name)
			err := os.MkdirAll(componentDir, 0o755)
			if err != nil {
				t.Skipf("Filesystem doesn't support Unicode name %q: %v", name, err)
			}

			t.Chdir(componentDir)

			atmosConfig := &schema.AtmosConfiguration{
				BasePath:                 tmpDir,
				TerraformDirAbsolutePath: terraformBase,
				Components: schema.Components{
					Terraform: schema.Terraform{
						BasePath: "components/terraform",
					},
				},
			}

			loader := &mockStackLoader{stacksMap: map[string]any{}}
			resolver := NewResolver(loader)

			// Should resolve without error.
			result, err := resolver.ResolveComponentFromPathWithoutValidation(atmosConfig, ".", "terraform")

			require.NoError(t, err)
			assert.Equal(t, name, result)
		})
	}
}

// TestResolver_UNCPaths tests UNC path handling on Windows.
func TestResolver_UNCPaths(t *testing.T) {
	// UNC paths are Windows-specific.
	if runtime.GOOS != "windows" {
		t.Skip("Skipping UNC path test on non-Windows")
	}

	// Test with a mock UNC-style path structure.
	// Note: We can't easily create actual UNC paths in tests, but we can test
	// that the resolver doesn't panic when encountering UNC path patterns.
	atmosConfig := &schema.AtmosConfiguration{
		BasePath: `\\server\share\project`,
		Components: schema.Components{
			Terraform: schema.Terraform{
				BasePath: `components\terraform`,
			},
		},
	}

	loader := &mockStackLoader{stacksMap: map[string]any{}}
	resolver := NewResolver(loader)

	// Test with a UNC-style path - should fail gracefully, not panic.
	_, err := resolver.ResolveComponentFromPath(atmosConfig, `\\server\share\project\components\terraform\vpc`, "dev", "terraform")
	// Error is expected (path likely doesn't exist), but no panic should occur.
	// The test verifies the code handles UNC paths without panicking.
	if err != nil {
		assert.NotContains(t, err.Error(), "panic")
	}
}

// TestResolver_SpecialCharacterComponentNames tests component names with special characters.
func TestResolver_SpecialCharacterComponentNames(t *testing.T) {
	tmpDir := t.TempDir()
	terraformBase := filepath.Join(tmpDir, "components", "terraform")

	// Test component names with special but filesystem-safe characters.
	specialNames := []string{
		"vpc-primary",
		"vpc_secondary",
		"vpc.backup",
		"vpc-123",
		"123-vpc",
		"VPC-UPPER",
		"vpc--double-dash",
		"vpc__double_underscore",
	}

	for _, name := range specialNames {
		t.Run(name, func(t *testing.T) {
			componentDir := filepath.Join(terraformBase, name)
			require.NoError(t, os.MkdirAll(componentDir, 0o755))

			t.Chdir(componentDir)

			atmosConfig := &schema.AtmosConfiguration{
				BasePath:                 tmpDir,
				TerraformDirAbsolutePath: terraformBase,
				Components: schema.Components{
					Terraform: schema.Terraform{
						BasePath: "components/terraform",
					},
				},
			}

			loader := &mockStackLoader{stacksMap: map[string]any{}}
			resolver := NewResolver(loader)

			result, err := resolver.ResolveComponentFromPathWithoutValidation(atmosConfig, ".", "terraform")

			require.NoError(t, err)
			assert.Equal(t, name, result)
		})
	}
}

// TestResolver_EmptyStacksMap tests behavior when stacks map is empty.
func TestResolver_EmptyStacksMap(t *testing.T) {
	tmpDir := t.TempDir()
	terraformBase := filepath.Join(tmpDir, "components", "terraform")
	componentDir := filepath.Join(terraformBase, "vpc")

	require.NoError(t, os.MkdirAll(componentDir, 0o755))
	t.Chdir(componentDir)

	atmosConfig := &schema.AtmosConfiguration{
		BasePath:                 tmpDir,
		TerraformDirAbsolutePath: terraformBase,
		Components: schema.Components{
			Terraform: schema.Terraform{
				BasePath: "components/terraform",
			},
		},
	}

	loader := &mockStackLoader{stacksMap: map[string]any{}}
	resolver := NewResolver(loader)

	// Without stack validation - should succeed.
	result, err := resolver.ResolveComponentFromPathWithoutValidation(atmosConfig, ".", "terraform")
	require.NoError(t, err)
	assert.Equal(t, "vpc", result)

	// With stack validation - should fail with stack not found.
	_, err = resolver.ResolveComponentFromPath(atmosConfig, ".", "nonexistent", "terraform")
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrStackNotFound)
}
