package atmos

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestNewVendorUpdateTool(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{}
	tool := NewVendorUpdateTool(atmosConfig)

	assert.NotNil(t, tool)
	assert.Same(t, atmosConfig, tool.atmosConfig)
}

func TestVendorUpdateTool_Name(t *testing.T) {
	tool := NewVendorUpdateTool(&schema.AtmosConfiguration{})
	assert.Equal(t, "atmos_vendor_update", tool.Name())
}

func TestVendorUpdateTool_Description(t *testing.T) {
	tool := NewVendorUpdateTool(&schema.AtmosConfiguration{})
	assert.NotEmpty(t, tool.Description())
}

func TestVendorUpdateTool_Parameters(t *testing.T) {
	tool := NewVendorUpdateTool(&schema.AtmosConfiguration{})
	params := tool.Parameters()

	require.Len(t, params, 2)
	assert.Equal(t, paramComponent, params[0].Name)
	assert.False(t, params[0].Required)
	assert.Equal(t, paramTags, params[1].Name)
	assert.False(t, params[1].Required)
}

func TestVendorUpdateTool_RequiresPermission(t *testing.T) {
	tool := NewVendorUpdateTool(&schema.AtmosConfiguration{})
	assert.True(t, tool.RequiresPermission())
}

func TestVendorUpdateTool_IsRestricted(t *testing.T) {
	tool := NewVendorUpdateTool(&schema.AtmosConfiguration{})
	assert.False(t, tool.IsRestricted())
}

func TestVendorUpdateTool_Execute_NoVendorFile(t *testing.T) {
	t.Chdir(t.TempDir())

	tool := NewVendorUpdateTool(&schema.AtmosConfiguration{})
	result, err := tool.Execute(context.Background(), map[string]interface{}{})

	require.Error(t, err)
	assert.False(t, result.Success)
	assert.ErrorIs(t, err, errUtils.ErrAIVendorFileNotFound)
}
