package atmos

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
	atmosyaml "github.com/cloudposse/atmos/pkg/yaml"
)

func TestNewStackConfigSetTool(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{}
	tool := NewStackConfigSetTool(atmosConfig)

	assert.NotNil(t, tool)
	assert.Same(t, atmosConfig, tool.atmosConfig)
}

func TestStackConfigSetTool_Name(t *testing.T) {
	tool := NewStackConfigSetTool(&schema.AtmosConfiguration{})
	assert.Equal(t, "atmos_stack_config_set", tool.Name())
}

func TestStackConfigSetTool_Description(t *testing.T) {
	tool := NewStackConfigSetTool(&schema.AtmosConfiguration{})
	assert.NotEmpty(t, tool.Description())
}

func TestStackConfigSetTool_Parameters(t *testing.T) {
	tool := NewStackConfigSetTool(&schema.AtmosConfiguration{})
	params := tool.Parameters()

	require.Len(t, params, 6)
	names := make([]string, 0, len(params))
	for _, p := range params {
		names = append(names, p.Name)
	}
	assert.Equal(t, []string{paramStack, paramComponent, "path", "value", "type", "file"}, names)
	assert.True(t, params[0].Required)
	assert.True(t, params[1].Required)
	assert.True(t, params[2].Required)
	assert.True(t, params[3].Required)
	assert.False(t, params[4].Required)
	assert.Equal(t, atmosyaml.TypeString, params[4].Default)
	assert.False(t, params[5].Required)
}

func TestStackConfigSetTool_RequiresPermission(t *testing.T) {
	tool := NewStackConfigSetTool(&schema.AtmosConfiguration{})
	assert.True(t, tool.RequiresPermission())
}

func TestStackConfigSetTool_IsRestricted(t *testing.T) {
	tool := NewStackConfigSetTool(&schema.AtmosConfiguration{})
	assert.False(t, tool.IsRestricted())
}

func TestStackConfigSetTool_Execute_MissingValue(t *testing.T) {
	tool := NewStackConfigSetTool(&schema.AtmosConfiguration{})
	result, err := tool.Execute(context.Background(), map[string]interface{}{
		paramStack:     "dev",
		paramComponent: "vpc",
		"path":         "vars.region",
	})
	require.Error(t, err)
	assert.False(t, result.Success)
	assert.Contains(t, err.Error(), "value")
}

func TestStackConfigSetTool_Execute_MissingStack(t *testing.T) {
	tool := NewStackConfigSetTool(&schema.AtmosConfiguration{})
	result, err := tool.Execute(context.Background(), map[string]interface{}{
		paramComponent: "vpc",
		"path":         "vars.region",
		"value":        "us-west-2",
	})
	require.Error(t, err)
	assert.False(t, result.Success)
}

func TestStackConfigSetTool_Execute_ProvenanceOverride(t *testing.T) {
	atmosConfig, stacksDir := stackConfigLiveFixture(t)
	tool := NewStackConfigSetTool(atmosConfig)

	result, err := tool.Execute(context.Background(), map[string]interface{}{
		paramStack:     "dev",
		paramComponent: "vpc",
		"path":         "vars.foo",
		"value":        "dev-updated",
	})
	require.NoError(t, err)
	require.True(t, result.Success)
	assert.False(t, result.Data["created"].(bool))

	got, err := atmosyaml.GetFile(filepath.Join(stacksDir, "dev.yaml"), "components.terraform.vpc.vars.foo")
	require.NoError(t, err)
	assert.Equal(t, "dev-updated", got)

	// Sibling manifest must be untouched (src->result isolation).
	catalogContent, err := os.ReadFile(filepath.Join(stacksDir, "catalog", "vpc.yaml"))
	require.NoError(t, err)
	assert.Contains(t, string(catalogContent), "foo: base")
}

// TestStackConfigSetTool_Execute_CreatesNewPath exercises the "created"
// branch. A path with zero provenance anywhere (like vars.az here) has no
// file for provenance resolution to pick, mirroring the "not defined, pass
// --file" behavior of cmd/stack/operations.go's resolveTargetByProvenance,
// so this test uses an explicit file the same way
// cmd/stack/operations_test.go's own
// TestRunStackSet_ExplicitFile_CreatesNewPath does.
func TestStackConfigSetTool_Execute_CreatesNewPath(t *testing.T) {
	atmosConfig, stacksDir := stackConfigLiveFixture(t)
	tool := NewStackConfigSetTool(atmosConfig)
	devFile := filepath.Join(stacksDir, "dev.yaml")

	result, err := tool.Execute(context.Background(), map[string]interface{}{
		paramStack:     "dev",
		paramComponent: "vpc",
		"path":         "vars.az",
		"value":        "a",
		"file":         devFile,
	})
	require.NoError(t, err)
	require.True(t, result.Success)
	assert.True(t, result.Data["created"].(bool))

	got, err := atmosyaml.GetFile(filepath.Join(stacksDir, "dev.yaml"), "components.terraform.vpc.vars.az")
	require.NoError(t, err)
	assert.Equal(t, "a", got)
}

func TestStackConfigSetTool_Execute_ExplicitFile(t *testing.T) {
	atmosConfig, _ := stackConfigLiveFixture(t)
	tool := NewStackConfigSetTool(atmosConfig)

	dir := t.TempDir()
	file := filepath.Join(dir, "explicit.yaml")
	require.NoError(t, os.WriteFile(file, []byte(`components:
  terraform:
    vpc:
      vars:
        region: eu-west-1
`), 0o644))

	result, err := tool.Execute(context.Background(), map[string]interface{}{
		paramStack:     "dev",
		paramComponent: "vpc",
		"path":         "vars.region",
		"value":        "us-west-2",
		"file":         file,
	})
	require.NoError(t, err)
	require.True(t, result.Success)

	got, err := atmosyaml.GetFile(file, "components.terraform.vpc.vars.region")
	require.NoError(t, err)
	assert.Equal(t, "us-west-2", got)
}

func TestStackConfigSetTool_Execute_InheritedValueRequiresFile(t *testing.T) {
	atmosConfig, _ := stackConfigLiveFixture(t)
	tool := NewStackConfigSetTool(atmosConfig)

	result, err := tool.Execute(context.Background(), map[string]interface{}{
		paramStack:     "dev",
		paramComponent: "vpc",
		"path":         "vars.region",
		"value":        "us-west-2",
	})
	require.Error(t, err)
	assert.False(t, result.Success)
	require.ErrorIs(t, err, errUtils.ErrAIStackConfigPathNotEditable)
}

// TestStackConfigSetTool_Execute_TypedValue targets a brand new path, so
// (like TestStackConfigSetTool_Execute_CreatesNewPath) it must use an
// explicit file rather than relying on provenance resolution.
func TestStackConfigSetTool_Execute_TypedValue(t *testing.T) {
	atmosConfig, stacksDir := stackConfigLiveFixture(t)
	tool := NewStackConfigSetTool(atmosConfig)
	devFile := filepath.Join(stacksDir, "dev.yaml")

	result, err := tool.Execute(context.Background(), map[string]interface{}{
		paramStack:     "dev",
		paramComponent: "vpc",
		"path":         "vars.enabled",
		"value":        "true",
		"type":         atmosyaml.TypeBool,
		"file":         devFile,
	})
	require.NoError(t, err)
	require.True(t, result.Success)

	got, err := atmosyaml.GetFile(filepath.Join(stacksDir, "dev.yaml"), "components.terraform.vpc.vars.enabled")
	require.NoError(t, err)
	assert.Equal(t, "true", got)
}

func TestStackConfigSetTool_Execute_InvalidType(t *testing.T) {
	atmosConfig, _ := stackConfigLiveFixture(t)
	tool := NewStackConfigSetTool(atmosConfig)

	result, err := tool.Execute(context.Background(), map[string]interface{}{
		paramStack:     "dev",
		paramComponent: "vpc",
		"path":         "vars.foo",
		"value":        "not-a-bool",
		"type":         atmosyaml.TypeBool,
	})
	require.Error(t, err)
	assert.False(t, result.Success)
}
