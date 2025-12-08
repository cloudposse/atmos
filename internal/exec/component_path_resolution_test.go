package exec

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
)

// getComponentPathResolutionFixturePath returns the path to the component-path-resolution fixture.
func getComponentPathResolutionFixturePath(t *testing.T) string {
	t.Helper()

	// Get the project root by walking up from the current file location.
	wd, err := os.Getwd()
	require.NoError(t, err)

	// Navigate from internal/exec to the project root.
	projectRoot := filepath.Join(wd, "..", "..")
	fixturePath := filepath.Join(projectRoot, "tests", "fixtures", "scenarios", "component-path-resolution")

	// Verify the fixture exists.
	_, err = os.Stat(filepath.Join(fixturePath, "atmos.yaml"))
	require.NoError(t, err, "component-path-resolution fixture not found at %s", fixturePath)

	return fixturePath
}

// initAtmosConfigForFixture initializes the atmos configuration for the fixture.
func initAtmosConfigForFixture(t *testing.T, fixturePath string) schema.AtmosConfiguration {
	t.Helper()

	// Change to fixture directory using t.Chdir for automatic cleanup.
	t.Chdir(fixturePath)

	// Initialize config with processStacks=true to enable stack loading.
	atmosConfig, err := config.InitCliConfig(schema.ConfigAndStacksInfo{}, true)
	require.NoError(t, err, "Failed to initialize atmos config for fixture")

	return atmosConfig
}

// TestComponentPathResolution_SimpleComponent tests resolving a simple component that matches its folder name.
func TestComponentPathResolution_SimpleComponent(t *testing.T) {
	fixturePath := getComponentPathResolutionFixturePath(t)
	atmosConfig := initAtmosConfigForFixture(t, fixturePath)

	componentPath := filepath.Join(fixturePath, "components", "terraform", "simple-component")

	// Test resolution with stack validation.
	result, err := ResolveComponentFromPath(&atmosConfig, componentPath, "dev", "terraform")

	require.NoError(t, err, "Expected simple-component to resolve without error")
	assert.Equal(t, "simple-component", result, "Expected component name to be 'simple-component'")
}

// TestComponentPathResolution_SimpleComponentProd tests resolving a simple component in prod stack.
func TestComponentPathResolution_SimpleComponentProd(t *testing.T) {
	fixturePath := getComponentPathResolutionFixturePath(t)
	atmosConfig := initAtmosConfigForFixture(t, fixturePath)

	componentPath := filepath.Join(fixturePath, "components", "terraform", "simple-component")

	// Test resolution with prod stack.
	result, err := ResolveComponentFromPath(&atmosConfig, componentPath, "prod", "terraform")

	require.NoError(t, err, "Expected simple-component to resolve in prod stack without error")
	assert.Equal(t, "simple-component", result, "Expected component name to be 'simple-component'")
}

// TestComponentPathResolution_NestedComponent tests resolving a component in a nested path.
func TestComponentPathResolution_NestedComponent(t *testing.T) {
	fixturePath := getComponentPathResolutionFixturePath(t)
	atmosConfig := initAtmosConfigForFixture(t, fixturePath)

	componentPath := filepath.Join(fixturePath, "components", "terraform", "nested", "component")

	// Test resolution with stack validation.
	result, err := ResolveComponentFromPath(&atmosConfig, componentPath, "dev", "terraform")

	require.NoError(t, err, "Expected nested/component to resolve without error")
	assert.Equal(t, "nested/component", result, "Expected component name to be 'nested/component'")
}

// TestComponentPathResolution_AmbiguousComponent tests detection of ambiguous component paths.
// When multiple Atmos components reference the same terraform folder, an error should be returned.
func TestComponentPathResolution_AmbiguousComponent(t *testing.T) {
	fixturePath := getComponentPathResolutionFixturePath(t)
	atmosConfig := initAtmosConfigForFixture(t, fixturePath)

	componentPath := filepath.Join(fixturePath, "components", "terraform", "ambiguous-component")

	// Test resolution - should return ambiguous path error.
	_, err := ResolveComponentFromPath(&atmosConfig, componentPath, "dev", "terraform")

	require.Error(t, err, "Expected error for ambiguous component path")
	assert.ErrorIs(t, err, errUtils.ErrAmbiguousComponentPath, "Expected ErrAmbiguousComponentPath")

	// Verify error message contains both matching components.
	errMsg := err.Error()
	assert.Contains(t, errMsg, "ambiguous", "Error should mention 'ambiguous'")
}

// TestComponentPathResolution_ComponentNotInStack tests error when component doesn't exist in stack.
func TestComponentPathResolution_ComponentNotInStack(t *testing.T) {
	fixturePath := getComponentPathResolutionFixturePath(t)
	atmosConfig := initAtmosConfigForFixture(t, fixturePath)

	// The ambiguous-component folder exists but ambiguous-alpha/beta components
	// are only in dev stack, not prod.
	componentPath := filepath.Join(fixturePath, "components", "terraform", "ambiguous-component")

	// Test resolution in prod stack where ambiguous components don't exist.
	_, err := ResolveComponentFromPath(&atmosConfig, componentPath, "prod", "terraform")

	require.Error(t, err, "Expected error when component not in stack")
	assert.ErrorIs(t, err, errUtils.ErrComponentNotInStack, "Expected ErrComponentNotInStack")
}

// TestComponentPathResolution_StackNotFound tests error when stack doesn't exist.
func TestComponentPathResolution_StackNotFound(t *testing.T) {
	fixturePath := getComponentPathResolutionFixturePath(t)
	atmosConfig := initAtmosConfigForFixture(t, fixturePath)

	componentPath := filepath.Join(fixturePath, "components", "terraform", "simple-component")

	// Test resolution with non-existent stack.
	_, err := ResolveComponentFromPath(&atmosConfig, componentPath, "nonexistent", "terraform")

	require.Error(t, err, "Expected error for nonexistent stack")
	assert.ErrorIs(t, err, errUtils.ErrStackNotFound, "Expected ErrStackNotFound")
}

// TestComponentPathResolution_TypeMismatch tests error when component type doesn't match.
func TestComponentPathResolution_TypeMismatch(t *testing.T) {
	fixturePath := getComponentPathResolutionFixturePath(t)
	atmosConfig := initAtmosConfigForFixture(t, fixturePath)

	componentPath := filepath.Join(fixturePath, "components", "terraform", "simple-component")

	// Try to resolve a terraform component as helmfile.
	_, err := ResolveComponentFromPath(&atmosConfig, componentPath, "dev", "helmfile")

	require.Error(t, err, "Expected error for component type mismatch")
	assert.ErrorIs(t, err, errUtils.ErrComponentTypeMismatch, "Expected ErrComponentTypeMismatch")
}

// TestComponentPathResolution_PathNotInComponentDir tests error when path is outside component directories.
func TestComponentPathResolution_PathNotInComponentDir(t *testing.T) {
	fixturePath := getComponentPathResolutionFixturePath(t)
	atmosConfig := initAtmosConfigForFixture(t, fixturePath)

	// Use stacks directory which is not a component directory.
	invalidPath := filepath.Join(fixturePath, "stacks")

	// Test resolution with invalid path.
	_, err := ResolveComponentFromPath(&atmosConfig, invalidPath, "dev", "terraform")

	require.Error(t, err, "Expected error for path not in component directory")
	assert.ErrorIs(t, err, errUtils.ErrPathNotInComponentDir, "Expected ErrPathNotInComponentDir")
}

// TestComponentPathResolution_WithoutTypeCheck tests resolution without type checking.
func TestComponentPathResolution_WithoutTypeCheck(t *testing.T) {
	fixturePath := getComponentPathResolutionFixturePath(t)
	atmosConfig := initAtmosConfigForFixture(t, fixturePath)

	tests := []struct {
		name        string
		path        string
		stack       string
		want        string
		wantErr     bool
		errIs       error
		errContains string
	}{
		{
			name:  "simple component without type check",
			path:  filepath.Join(fixturePath, "components", "terraform", "simple-component"),
			stack: "dev",
			want:  "simple-component",
		},
		{
			name:  "nested component without type check",
			path:  filepath.Join(fixturePath, "components", "terraform", "nested", "component"),
			stack: "dev",
			want:  "nested/component",
		},
		{
			name:    "ambiguous component without type check",
			path:    filepath.Join(fixturePath, "components", "terraform", "ambiguous-component"),
			stack:   "dev",
			wantErr: true,
			errIs:   errUtils.ErrAmbiguousComponentPath,
		},
		{
			name:    "stack not found",
			path:    filepath.Join(fixturePath, "components", "terraform", "simple-component"),
			stack:   "nonexistent",
			wantErr: true,
			errIs:   errUtils.ErrStackNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ResolveComponentFromPathWithoutTypeCheck(&atmosConfig, tt.path, tt.stack)

			if tt.wantErr {
				require.Error(t, err, "Expected error")
				if tt.errIs != nil {
					assert.ErrorIs(t, err, tt.errIs)
				}
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.want, result)
		})
	}
}

// TestComponentPathResolution_WithoutValidation tests resolution without stack validation.
func TestComponentPathResolution_WithoutValidation(t *testing.T) {
	fixturePath := getComponentPathResolutionFixturePath(t)
	atmosConfig := initAtmosConfigForFixture(t, fixturePath)

	tests := []struct {
		name          string
		path          string
		componentType string
		want          string
		wantErr       bool
		errIs         error
	}{
		{
			name:          "simple component without validation",
			path:          filepath.Join(fixturePath, "components", "terraform", "simple-component"),
			componentType: "terraform",
			want:          "simple-component",
		},
		{
			name:          "nested component without validation",
			path:          filepath.Join(fixturePath, "components", "terraform", "nested", "component"),
			componentType: "terraform",
			want:          "nested/component",
		},
		{
			name:          "ambiguous component without validation",
			path:          filepath.Join(fixturePath, "components", "terraform", "ambiguous-component"),
			componentType: "terraform",
			want:          "ambiguous-component", // Without validation, ambiguity is not detected
		},
		{
			name:          "type mismatch detected even without validation",
			path:          filepath.Join(fixturePath, "components", "terraform", "simple-component"),
			componentType: "helmfile",
			wantErr:       true,
			errIs:         errUtils.ErrComponentTypeMismatch,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ResolveComponentFromPathWithoutValidation(&atmosConfig, tt.path, tt.componentType)

			if tt.wantErr {
				require.Error(t, err, "Expected error")
				if tt.errIs != nil {
					assert.ErrorIs(t, err, tt.errIs)
				}
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.want, result)
		})
	}
}

// TestComponentPathResolution_CurrentDirectory tests resolution using "." from component directory.
func TestComponentPathResolution_CurrentDirectory(t *testing.T) {
	fixturePath := getComponentPathResolutionFixturePath(t)

	// Change to the simple-component directory using t.Chdir for automatic cleanup.
	componentDir := filepath.Join(fixturePath, "components", "terraform", "simple-component")
	t.Chdir(componentDir)

	// Initialize config from component directory.
	atmosConfig, err := config.InitCliConfig(schema.ConfigAndStacksInfo{}, true)
	require.NoError(t, err)

	// Resolve "." as the component path.
	result, err := ResolveComponentFromPath(&atmosConfig, ".", "dev", "terraform")

	require.NoError(t, err, "Expected '.' to resolve to simple-component")
	assert.Equal(t, "simple-component", result)
}

// TestComponentPathResolution_RelativePath tests resolution using relative paths.
func TestComponentPathResolution_RelativePath(t *testing.T) {
	fixturePath := getComponentPathResolutionFixturePath(t)

	// Change to the components/terraform directory using t.Chdir for automatic cleanup.
	terraformDir := filepath.Join(fixturePath, "components", "terraform")
	t.Chdir(terraformDir)

	// Initialize config from terraform base directory.
	atmosConfig, err := config.InitCliConfig(schema.ConfigAndStacksInfo{}, true)
	require.NoError(t, err)

	// Resolve "./simple-component" as relative path.
	result, err := ResolveComponentFromPath(&atmosConfig, "./simple-component", "dev", "terraform")

	require.NoError(t, err, "Expected './simple-component' to resolve")
	assert.Equal(t, "simple-component", result)
}

// TestComponentPathResolution_AllStacksForComponent tests that component exists in all expected stacks.
func TestComponentPathResolution_AllStacksForComponent(t *testing.T) {
	fixturePath := getComponentPathResolutionFixturePath(t)
	atmosConfig := initAtmosConfigForFixture(t, fixturePath)

	componentPath := filepath.Join(fixturePath, "components", "terraform", "simple-component")

	stacks := []string{"dev", "prod"}

	for _, stack := range stacks {
		t.Run("stack_"+stack, func(t *testing.T) {
			result, err := ResolveComponentFromPath(&atmosConfig, componentPath, stack, "terraform")

			require.NoError(t, err, "Expected simple-component to exist in stack %s", stack)
			assert.Equal(t, "simple-component", result)
		})
	}
}

// TestComponentPathResolution_NestedComponentAllStacks tests nested component in all stacks.
func TestComponentPathResolution_NestedComponentAllStacks(t *testing.T) {
	fixturePath := getComponentPathResolutionFixturePath(t)
	atmosConfig := initAtmosConfigForFixture(t, fixturePath)

	componentPath := filepath.Join(fixturePath, "components", "terraform", "nested", "component")

	stacks := []string{"dev", "prod"}

	for _, stack := range stacks {
		t.Run("stack_"+stack, func(t *testing.T) {
			result, err := ResolveComponentFromPath(&atmosConfig, componentPath, stack, "terraform")

			require.NoError(t, err, "Expected nested/component to exist in stack %s", stack)
			assert.Equal(t, "nested/component", result)
		})
	}
}

// TestComponentPathResolution_AmbiguousOnlyInDev tests that ambiguous components only exist in dev.
func TestComponentPathResolution_AmbiguousOnlyInDev(t *testing.T) {
	fixturePath := getComponentPathResolutionFixturePath(t)
	atmosConfig := initAtmosConfigForFixture(t, fixturePath)

	componentPath := filepath.Join(fixturePath, "components", "terraform", "ambiguous-component")

	t.Run("dev_has_ambiguous_components", func(t *testing.T) {
		_, err := ResolveComponentFromPath(&atmosConfig, componentPath, "dev", "terraform")

		// In dev, there are two components (ambiguous-alpha, ambiguous-beta) pointing to this folder.
		require.Error(t, err)
		assert.ErrorIs(t, err, errUtils.ErrAmbiguousComponentPath)
	})

	t.Run("prod_has_no_ambiguous_components", func(t *testing.T) {
		_, err := ResolveComponentFromPath(&atmosConfig, componentPath, "prod", "terraform")

		// In prod, there are no components pointing to the ambiguous-component folder.
		require.Error(t, err)
		assert.ErrorIs(t, err, errUtils.ErrComponentNotInStack)
	})
}
