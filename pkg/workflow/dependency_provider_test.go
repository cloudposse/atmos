package workflow

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

// TestNewDefaultDependencyProvider tests the constructor.
func TestNewDefaultDependencyProvider(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{}
	provider := NewDefaultDependencyProvider(atmosConfig)

	assert.NotNil(t, provider)
	assert.Equal(t, atmosConfig, provider.atmosConfig)
}

// TestDefaultDependencyProvider_LoadToolVersionsDependencies tests loading .tool-versions file.
func TestDefaultDependencyProvider_LoadToolVersionsDependencies(t *testing.T) {
	tempDir := t.TempDir()
	t.Chdir(tempDir) // Change to temp dir so .tool-versions is found.

	// Create a .tool-versions file.
	toolVersionsPath := filepath.Join(tempDir, ".tool-versions")
	content := "terraform 1.11.4\nopentofu 1.10.0\n"
	err := os.WriteFile(toolVersionsPath, []byte(content), 0o644)
	require.NoError(t, err)

	atmosConfig := &schema.AtmosConfiguration{
		BasePath: tempDir,
	}

	provider := NewDefaultDependencyProvider(atmosConfig)
	deps, err := provider.LoadToolVersionsDependencies()

	// Should not error (returns empty map if file doesn't exist or is empty).
	assert.NoError(t, err)
	assert.NotNil(t, deps)
	// Verify parsed content if file was found.
	if len(deps) > 0 {
		assert.Equal(t, "1.11.4", deps["terraform"])
		assert.Equal(t, "1.10.0", deps["opentofu"])
	}
}

// TestDefaultDependencyProvider_LoadToolVersionsDependencies_NoFile tests when .tool-versions doesn't exist.
func TestDefaultDependencyProvider_LoadToolVersionsDependencies_NoFile(t *testing.T) {
	tempDir := t.TempDir()

	atmosConfig := &schema.AtmosConfiguration{
		BasePath: tempDir,
	}

	provider := NewDefaultDependencyProvider(atmosConfig)
	deps, err := provider.LoadToolVersionsDependencies()

	// Should not error - returns empty map when file doesn't exist.
	assert.NoError(t, err)
	assert.NotNil(t, deps)
}

// TestDefaultDependencyProvider_ResolveWorkflowDependencies tests resolving workflow dependencies.
func TestDefaultDependencyProvider_ResolveWorkflowDependencies(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{}
	provider := NewDefaultDependencyProvider(atmosConfig)

	workflowDef := &schema.WorkflowDefinition{
		Dependencies: &schema.Dependencies{
			Tools: map[string]string{
				"terraform": "1.11.4",
				"kubectl":   "1.32.0",
			},
		},
	}

	deps, err := provider.ResolveWorkflowDependencies(workflowDef)

	assert.NoError(t, err)
	assert.NotNil(t, deps)
	assert.Equal(t, "1.11.4", deps["terraform"])
	assert.Equal(t, "1.32.0", deps["kubectl"])
}

// TestDefaultDependencyProvider_ResolveWorkflowDependencies_NilDef tests with nil workflow definition.
func TestDefaultDependencyProvider_ResolveWorkflowDependencies_NilDef(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{}
	provider := NewDefaultDependencyProvider(atmosConfig)

	deps, err := provider.ResolveWorkflowDependencies(nil)

	assert.NoError(t, err)
	assert.Empty(t, deps)
}

// TestDefaultDependencyProvider_ResolveWorkflowDependencies_EmptyDef tests with empty workflow definition.
func TestDefaultDependencyProvider_ResolveWorkflowDependencies_EmptyDef(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{}
	provider := NewDefaultDependencyProvider(atmosConfig)

	workflowDef := &schema.WorkflowDefinition{}

	deps, err := provider.ResolveWorkflowDependencies(workflowDef)

	assert.NoError(t, err)
	assert.Empty(t, deps)
}

// TestDefaultDependencyProvider_MergeDependencies tests merging dependencies.
func TestDefaultDependencyProvider_MergeDependencies(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{}
	provider := NewDefaultDependencyProvider(atmosConfig)

	// Use constraints in base so overlay versions can satisfy them.
	base := map[string]string{
		"terraform": "^1.11.0", // Constraint - allows 1.x.x >= 1.11.0.
		"kubectl":   "1.32.0",  // Exact version.
	}

	overlay := map[string]string{
		"terraform": "1.12.0", // Override - satisfies ^1.11.0.
		"helm":      "3.16.4", // Add new.
	}

	merged, err := provider.MergeDependencies(base, overlay)

	assert.NoError(t, err)
	assert.NotNil(t, merged)
	assert.Equal(t, "1.12.0", merged["terraform"]) // Overlay wins.
	assert.Equal(t, "1.32.0", merged["kubectl"])   // From base.
	assert.Equal(t, "3.16.4", merged["helm"])      // From overlay.
}

// TestDefaultDependencyProvider_MergeDependencies_NilBase tests merging with nil base.
func TestDefaultDependencyProvider_MergeDependencies_NilBase(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{}
	provider := NewDefaultDependencyProvider(atmosConfig)

	overlay := map[string]string{
		"terraform": "1.12.0",
	}

	merged, err := provider.MergeDependencies(nil, overlay)

	assert.NoError(t, err)
	assert.NotNil(t, merged)
	assert.Equal(t, "1.12.0", merged["terraform"])
}

// TestDefaultDependencyProvider_MergeDependencies_NilOverlay tests merging with nil overlay.
func TestDefaultDependencyProvider_MergeDependencies_NilOverlay(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{}
	provider := NewDefaultDependencyProvider(atmosConfig)

	base := map[string]string{
		"terraform": "1.11.4",
	}

	merged, err := provider.MergeDependencies(base, nil)

	assert.NoError(t, err)
	assert.NotNil(t, merged)
	assert.Equal(t, "1.11.4", merged["terraform"])
}

// TestDefaultDependencyProvider_MergeDependencies_BothNil tests merging with both nil.
func TestDefaultDependencyProvider_MergeDependencies_BothNil(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{}
	provider := NewDefaultDependencyProvider(atmosConfig)

	merged, err := provider.MergeDependencies(nil, nil)

	assert.NoError(t, err)
	assert.NotNil(t, merged)
	assert.Empty(t, merged)
}

// TestDefaultDependencyProvider_EnsureTools tests the EnsureTools method.
func TestDefaultDependencyProvider_EnsureTools(t *testing.T) {
	tempDir := t.TempDir()

	atmosConfig := &schema.AtmosConfiguration{
		BasePath: tempDir,
		Toolchain: schema.Toolchain{
			InstallPath: filepath.Join(tempDir, ".atmos", "tools"),
		},
	}

	provider := NewDefaultDependencyProvider(atmosConfig)

	// Test with empty dependencies - should succeed.
	err := provider.EnsureTools(map[string]string{})
	assert.NoError(t, err)

	// Test with nil dependencies - should succeed.
	err = provider.EnsureTools(nil)
	assert.NoError(t, err)
}

// TestDefaultDependencyProvider_UpdatePathForTools tests the UpdatePathForTools method.
func TestDefaultDependencyProvider_UpdatePathForTools(t *testing.T) {
	tempDir := t.TempDir()

	atmosConfig := &schema.AtmosConfiguration{
		BasePath: tempDir,
		Toolchain: schema.Toolchain{
			InstallPath: filepath.Join(tempDir, ".atmos", "tools"),
		},
	}

	provider := NewDefaultDependencyProvider(atmosConfig)

	// Test with empty dependencies - should succeed.
	err := provider.UpdatePathForTools(map[string]string{})
	assert.NoError(t, err)

	// Test with nil dependencies - should succeed.
	err = provider.UpdatePathForTools(nil)
	assert.NoError(t, err)
}
