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

func TestValidateSchemaTool_Interface(t *testing.T) {
	tool := NewValidateSchemaTool(&schema.AtmosConfiguration{})

	assert.Equal(t, "atmos_validate_schema", tool.Name())
	assert.NotEmpty(t, tool.Description())
	assert.False(t, tool.RequiresPermission())
	assert.False(t, tool.IsRestricted())

	params := tool.Parameters()
	require.Len(t, params, 1)
	assert.Equal(t, "key", params[0].Name)
	assert.False(t, params[0].Required)
}

func TestValidateSchemaTool_NewValidateSchemaTool(t *testing.T) {
	config := &schema.AtmosConfiguration{BasePath: t.TempDir()}
	tool := NewValidateSchemaTool(config)

	assert.NotNil(t, tool)
	assert.Equal(t, config, tool.atmosConfig)
}

// TestValidateSchemaTool_Execute_ValidConfig runs the tool from a fixture with
// a valid atmos.yaml: the built-in `config` schema entry validates it against
// the embedded generated schema.
func TestValidateSchemaTool_Execute_ValidConfig(t *testing.T) {
	t.Chdir("../../../../examples/demo-stacks")

	tool := NewValidateSchemaTool(&schema.AtmosConfiguration{})

	result, err := tool.Execute(context.Background(), map[string]interface{}{})

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.Success)
	assert.Contains(t, result.Output, "validated successfully")
}

// TestValidateSchemaTool_Execute_InvalidConfig confirms schema violations
// surface as a failed result (the negative path for the test above).
func TestValidateSchemaTool_Execute_InvalidConfig(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "atmos.yaml"), []byte("logs:\n  level: 42\n"), 0o644))
	t.Chdir(dir)

	tool := NewValidateSchemaTool(&schema.AtmosConfiguration{})

	result, err := tool.Execute(context.Background(), map[string]interface{}{"key": "config"})

	require.Error(t, err)
	require.NotNil(t, result)
	assert.False(t, result.Success)
}
