package atmos

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestNewVendorDiffTool(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{}
	tool := NewVendorDiffTool(atmosConfig)

	assert.NotNil(t, tool)
	assert.Same(t, atmosConfig, tool.atmosConfig)
}

func TestVendorDiffTool_Name(t *testing.T) {
	tool := NewVendorDiffTool(&schema.AtmosConfiguration{})
	assert.Equal(t, "atmos_vendor_diff", tool.Name())
}

func TestVendorDiffTool_Description(t *testing.T) {
	tool := NewVendorDiffTool(&schema.AtmosConfiguration{})
	assert.NotEmpty(t, tool.Description())
}

func TestVendorDiffTool_Parameters(t *testing.T) {
	tool := NewVendorDiffTool(&schema.AtmosConfiguration{})
	params := tool.Parameters()

	require.Len(t, params, 3)
	assert.Equal(t, paramComponent, params[0].Name)
	assert.True(t, params[0].Required)
	assert.Equal(t, "from", params[1].Name)
	assert.False(t, params[1].Required)
	assert.Equal(t, "to", params[2].Name)
	assert.False(t, params[2].Required)
}

func TestVendorDiffTool_RequiresPermission(t *testing.T) {
	tool := NewVendorDiffTool(&schema.AtmosConfiguration{})
	assert.False(t, tool.RequiresPermission())
}

func TestVendorDiffTool_IsRestricted(t *testing.T) {
	tool := NewVendorDiffTool(&schema.AtmosConfiguration{})
	assert.False(t, tool.IsRestricted())
}

func TestVendorDiffTool_Execute_MissingComponent(t *testing.T) {
	tool := NewVendorDiffTool(&schema.AtmosConfiguration{})
	result, err := tool.Execute(context.Background(), map[string]interface{}{})

	require.Error(t, err)
	assert.False(t, result.Success)
	assert.ErrorIs(t, err, errUtils.ErrAIToolParameterRequired)
}

func TestVendorDiffTool_Execute_ComponentNotFound(t *testing.T) {
	t.Chdir(t.TempDir())

	tool := NewVendorDiffTool(&schema.AtmosConfiguration{})
	result, err := tool.Execute(context.Background(), map[string]interface{}{
		paramComponent: "nonexistent",
	})

	require.Error(t, err)
	assert.False(t, result.Success)
}
