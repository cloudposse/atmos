package atmos

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestNewVendorCheckUpdatesTool(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{}
	tool := NewVendorCheckUpdatesTool(atmosConfig)

	assert.NotNil(t, tool)
	assert.Same(t, atmosConfig, tool.atmosConfig)
}

func TestVendorCheckUpdatesTool_Name(t *testing.T) {
	tool := NewVendorCheckUpdatesTool(&schema.AtmosConfiguration{})
	assert.Equal(t, "atmos_vendor_check_updates", tool.Name())
}

func TestVendorCheckUpdatesTool_Description(t *testing.T) {
	tool := NewVendorCheckUpdatesTool(&schema.AtmosConfiguration{})
	assert.NotEmpty(t, tool.Description())
}

func TestVendorCheckUpdatesTool_Parameters(t *testing.T) {
	tool := NewVendorCheckUpdatesTool(&schema.AtmosConfiguration{})
	params := tool.Parameters()

	require.Len(t, params, 2)
	assert.Equal(t, paramComponent, params[0].Name)
	assert.False(t, params[0].Required)
	assert.Equal(t, paramTags, params[1].Name)
	assert.False(t, params[1].Required)
}

func TestVendorCheckUpdatesTool_RequiresPermission(t *testing.T) {
	tool := NewVendorCheckUpdatesTool(&schema.AtmosConfiguration{})
	assert.False(t, tool.RequiresPermission())
}

func TestVendorCheckUpdatesTool_IsRestricted(t *testing.T) {
	tool := NewVendorCheckUpdatesTool(&schema.AtmosConfiguration{})
	assert.False(t, tool.IsRestricted())
}

func TestVendorCheckUpdatesTool_Execute_NoVendorFile(t *testing.T) {
	t.Chdir(t.TempDir())

	tool := NewVendorCheckUpdatesTool(&schema.AtmosConfiguration{})
	result, err := tool.Execute(context.Background(), map[string]interface{}{})

	require.Error(t, err)
	assert.False(t, result.Success)
	assert.ErrorIs(t, err, errUtils.ErrAIVendorFileNotFound)
}

func TestVendorCheckUpdatesTool_Execute_ComponentNotFound(t *testing.T) {
	t.Chdir(t.TempDir())

	tool := NewVendorCheckUpdatesTool(&schema.AtmosConfiguration{})
	result, err := tool.Execute(context.Background(), map[string]interface{}{
		paramComponent: "nonexistent",
	})

	require.Error(t, err)
	assert.False(t, result.Success)
}
