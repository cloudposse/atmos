package atmos

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

func TestNewStackConfigFormatTool(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{}
	tool := NewStackConfigFormatTool(atmosConfig)

	assert.NotNil(t, tool)
	assert.Same(t, atmosConfig, tool.atmosConfig)
}

func TestStackConfigFormatTool_Name(t *testing.T) {
	tool := NewStackConfigFormatTool(&schema.AtmosConfiguration{})
	assert.Equal(t, "atmos_stack_config_format", tool.Name())
}

func TestStackConfigFormatTool_Description(t *testing.T) {
	tool := NewStackConfigFormatTool(&schema.AtmosConfiguration{})
	assert.NotEmpty(t, tool.Description())
}

func TestStackConfigFormatTool_Parameters(t *testing.T) {
	tool := NewStackConfigFormatTool(&schema.AtmosConfiguration{})
	params := tool.Parameters()

	require.Len(t, params, 2)
	assert.Equal(t, paramStack, params[0].Name)
	assert.True(t, params[0].Required)
	assert.Equal(t, paramComponent, params[1].Name)
	assert.True(t, params[1].Required)
}

func TestStackConfigFormatTool_RequiresPermission(t *testing.T) {
	tool := NewStackConfigFormatTool(&schema.AtmosConfiguration{})
	assert.True(t, tool.RequiresPermission())
}

func TestStackConfigFormatTool_IsRestricted(t *testing.T) {
	tool := NewStackConfigFormatTool(&schema.AtmosConfiguration{})
	assert.False(t, tool.IsRestricted())
}

func TestStackConfigFormatTool_Execute_MissingStack(t *testing.T) {
	tool := NewStackConfigFormatTool(&schema.AtmosConfiguration{})
	result, err := tool.Execute(context.Background(), map[string]interface{}{
		paramComponent: "vpc",
	})
	require.Error(t, err)
	assert.False(t, result.Success)
}

func TestStackConfigFormatTool_Execute_MissingComponent(t *testing.T) {
	tool := NewStackConfigFormatTool(&schema.AtmosConfiguration{})
	result, err := tool.Execute(context.Background(), map[string]interface{}{
		paramStack: "dev",
	})
	require.Error(t, err)
	assert.False(t, result.Success)
}

// TestStackConfigFormatTool_Execute_FormatsQueryingManifest proves format
// normalizes the manifest provenance attributes the component to. Note: in a
// single-level import (dev.yaml imports catalog/vpc.yaml), every provenance
// path under the component -- whether literally overridden in dev.yaml
// (vars.foo) or purely inherited from the catalog (vars.region) -- records
// dev.yaml as its LAST entry (imports always append a trailing entry for the
// importing file), so PickProvenanceFile's last-wins rule collects only
// dev.yaml here; the catalog manifest is never picked up by provenance-based
// format alone. Collecting genuinely distinct files is covered directly
// against stackFormatFilesFromProvenance in stack_config_common_test.go
// (TestStackFormatFilesFromProvenance), which supplies provenance where
// different paths truly resolve to different files.
func TestStackConfigFormatTool_Execute_FormatsQueryingManifest(t *testing.T) {
	atmosConfig, stacksDir := stackConfigLiveFixture(t)

	devFile := filepath.Join(stacksDir, "dev.yaml")

	// Deliberately messy formatting on the importing manifest so FormatFile
	// has something to normalize.
	messy := "components:\n  terraform:\n    vpc:\n      vars:\n        foo:    dev-override\n\n\n"
	require.NoError(t, os.WriteFile(devFile, []byte("import:\n  - catalog/vpc\n\nvars:\n  stage: dev\n\n"+messy), 0o644))

	tool := NewStackConfigFormatTool(atmosConfig)
	result, err := tool.Execute(context.Background(), map[string]interface{}{
		paramStack:     "dev",
		paramComponent: "vpc",
	})
	require.NoError(t, err)
	require.True(t, result.Success)

	files, ok := result.Data["files"].([]string)
	require.True(t, ok)
	assert.Equal(t, []string{devFile}, files)

	got, err := os.ReadFile(devFile)
	require.NoError(t, err)
	assert.Contains(t, string(got), "foo: dev-override")
	assert.NotContains(t, string(got), "foo:    dev-override")
}

func TestStackConfigFormatTool_Execute_InvalidStack(t *testing.T) {
	atmosConfig, _ := stackConfigLiveFixture(t)
	tool := NewStackConfigFormatTool(atmosConfig)

	result, err := tool.Execute(context.Background(), map[string]interface{}{
		paramStack:     "does-not-exist",
		paramComponent: "vpc",
	})
	require.Error(t, err)
	assert.False(t, result.Success)
}
