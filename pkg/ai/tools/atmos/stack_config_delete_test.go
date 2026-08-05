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

func TestNewStackConfigDeleteTool(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{}
	tool := NewStackConfigDeleteTool(atmosConfig)

	assert.NotNil(t, tool)
	assert.Same(t, atmosConfig, tool.atmosConfig)
}

func TestStackConfigDeleteTool_Name(t *testing.T) {
	tool := NewStackConfigDeleteTool(&schema.AtmosConfiguration{})
	assert.Equal(t, "atmos_stack_config_delete", tool.Name())
}

func TestStackConfigDeleteTool_Description(t *testing.T) {
	tool := NewStackConfigDeleteTool(&schema.AtmosConfiguration{})
	assert.NotEmpty(t, tool.Description())
}

func TestStackConfigDeleteTool_Parameters(t *testing.T) {
	tool := NewStackConfigDeleteTool(&schema.AtmosConfiguration{})
	params := tool.Parameters()

	require.Len(t, params, 4)
	assert.Equal(t, paramStack, params[0].Name)
	assert.Equal(t, paramComponent, params[1].Name)
	assert.Equal(t, "path", params[2].Name)
	assert.True(t, params[2].Required)
	assert.Equal(t, "file", params[3].Name)
	assert.False(t, params[3].Required)
}

func TestStackConfigDeleteTool_RequiresPermission(t *testing.T) {
	tool := NewStackConfigDeleteTool(&schema.AtmosConfiguration{})
	assert.True(t, tool.RequiresPermission())
}

func TestStackConfigDeleteTool_IsRestricted(t *testing.T) {
	tool := NewStackConfigDeleteTool(&schema.AtmosConfiguration{})
	assert.False(t, tool.IsRestricted())
}

func TestStackConfigDeleteTool_Execute_MissingPath(t *testing.T) {
	tool := NewStackConfigDeleteTool(&schema.AtmosConfiguration{})
	result, err := tool.Execute(context.Background(), map[string]interface{}{
		paramStack:     "dev",
		paramComponent: "vpc",
	})
	require.Error(t, err)
	assert.False(t, result.Success)
}

func TestStackConfigDeleteTool_Execute_ProvenanceOverride(t *testing.T) {
	atmosConfig, stacksDir := stackConfigLiveFixture(t)
	tool := NewStackConfigDeleteTool(atmosConfig)

	result, err := tool.Execute(context.Background(), map[string]interface{}{
		paramStack:     "dev",
		paramComponent: "vpc",
		"path":         "vars.foo",
	})
	require.NoError(t, err)
	require.True(t, result.Success)
	assert.True(t, result.Data["deleted"].(bool))

	_, err = atmosyaml.GetFile(filepath.Join(stacksDir, "dev.yaml"), "components.terraform.vpc.vars.foo")
	require.Error(t, err)

	// The catalog manifest (which the value is inherited from) must survive
	// untouched (src->result isolation).
	catalogContent, err := os.ReadFile(filepath.Join(stacksDir, "catalog", "vpc.yaml"))
	require.NoError(t, err)
	assert.Contains(t, string(catalogContent), "foo: base")
}

func TestStackConfigDeleteTool_Execute_ExplicitFile(t *testing.T) {
	atmosConfig, _ := stackConfigLiveFixture(t)
	tool := NewStackConfigDeleteTool(atmosConfig)

	dir := t.TempDir()
	file := filepath.Join(dir, "explicit.yaml")
	require.NoError(t, os.WriteFile(file, []byte(`components:
  terraform:
    vpc:
      vars:
        region: eu-west-1
        az: a
`), 0o644))

	result, err := tool.Execute(context.Background(), map[string]interface{}{
		paramStack:     "dev",
		paramComponent: "vpc",
		"path":         "vars.region",
		"file":         file,
	})
	require.NoError(t, err)
	require.True(t, result.Success)
	assert.True(t, result.Data["deleted"].(bool))

	// The sibling key must survive the deletion.
	got, err := atmosyaml.GetFile(file, "components.terraform.vpc.vars.az")
	require.NoError(t, err)
	assert.Equal(t, "a", got)
}

func TestStackConfigDeleteTool_Execute_NothingToDelete(t *testing.T) {
	atmosConfig, _ := stackConfigLiveFixture(t)
	tool := NewStackConfigDeleteTool(atmosConfig)

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
		"path":         "vars.does_not_exist",
		"file":         file,
	})
	require.NoError(t, err)
	require.True(t, result.Success)
	assert.False(t, result.Data["deleted"].(bool))
}

func TestStackConfigDeleteTool_Execute_InheritedValueRequiresFile(t *testing.T) {
	atmosConfig, _ := stackConfigLiveFixture(t)
	tool := NewStackConfigDeleteTool(atmosConfig)

	result, err := tool.Execute(context.Background(), map[string]interface{}{
		paramStack:     "dev",
		paramComponent: "vpc",
		"path":         "vars.region",
	})
	require.Error(t, err)
	assert.False(t, result.Success)
	require.ErrorIs(t, err, errUtils.ErrAIStackConfigPathNotEditable)
}
