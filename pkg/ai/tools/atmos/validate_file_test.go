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
)

const validateFileTestSchema = `{
  "$schema": "http://json-schema.org/draft-07/schema#",
  "type": "object",
  "properties": {
    "name": { "type": "string" }
  },
  "required": ["name"]
}
`

func TestNewValidateFileTool(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{}
	tool := NewValidateFileTool(atmosConfig)

	assert.NotNil(t, tool)
	assert.Same(t, atmosConfig, tool.atmosConfig)
}

func TestValidateFileTool_Name(t *testing.T) {
	tool := NewValidateFileTool(&schema.AtmosConfiguration{})
	assert.Equal(t, "atmos_validate_file", tool.Name())
}

func TestValidateFileTool_Description(t *testing.T) {
	tool := NewValidateFileTool(&schema.AtmosConfiguration{})
	assert.NotEmpty(t, tool.Description())
}

func TestValidateFileTool_Parameters(t *testing.T) {
	tool := NewValidateFileTool(&schema.AtmosConfiguration{})
	params := tool.Parameters()

	require.Len(t, params, 2)
	assert.Equal(t, "file_path", params[0].Name)
	assert.True(t, params[0].Required)
	assert.Equal(t, "schema_path", params[1].Name)
	assert.False(t, params[1].Required)
}

func TestValidateFileTool_RequiresPermission(t *testing.T) {
	tool := NewValidateFileTool(&schema.AtmosConfiguration{})
	assert.False(t, tool.RequiresPermission())
}

func TestValidateFileTool_IsRestricted(t *testing.T) {
	tool := NewValidateFileTool(&schema.AtmosConfiguration{})
	assert.False(t, tool.IsRestricted())
}

func setupValidateFileFixture(t *testing.T) (tmpDir, schemaFile string) {
	t.Helper()
	tmpDir = t.TempDir()
	schemaFile = filepath.Join(tmpDir, "schema.json")
	require.NoError(t, os.WriteFile(schemaFile, []byte(validateFileTestSchema), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "valid.yaml"), []byte("name: hello\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "invalid.yaml"), []byte("other: 1\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "wrong_type.yaml"), []byte("other: 1\nname: 123\n"), 0o644))
	return tmpDir, schemaFile
}

func TestValidateFileTool_Execute_ExplicitSchema(t *testing.T) {
	tmpDir, schemaFile := setupValidateFileFixture(t)
	atmosConfig := &schema.AtmosConfiguration{BasePath: tmpDir}
	tool := NewValidateFileTool(atmosConfig)
	ctx := context.Background()

	t.Run("valid file passes", func(t *testing.T) {
		result, err := tool.Execute(ctx, map[string]interface{}{
			"file_path":   "valid.yaml",
			"schema_path": schemaFile,
		})
		require.NoError(t, err)
		assert.True(t, result.Success)
		assert.Equal(t, true, result.Data["valid"])
	})

	t.Run("invalid file reports a line number", func(t *testing.T) {
		result, err := tool.Execute(ctx, map[string]interface{}{
			"file_path":   "invalid.yaml",
			"schema_path": schemaFile,
		})
		require.NoError(t, err)
		assert.False(t, result.Success)
		assert.Equal(t, false, result.Data["valid"])

		findings, ok := result.Data["findings"].([]validateFileFinding)
		require.True(t, ok)
		require.Len(t, findings, 1)
		assert.Positive(t, findings[0].Line)
		assert.Contains(t, result.Output, "invalid.yaml:1")
	})

	t.Run("nested field error resolves to its own line", func(t *testing.T) {
		result, err := tool.Execute(ctx, map[string]interface{}{
			"file_path":   "wrong_type.yaml",
			"schema_path": schemaFile,
		})
		require.NoError(t, err)
		assert.False(t, result.Success)

		findings, ok := result.Data["findings"].([]validateFileFinding)
		require.True(t, ok)
		require.Len(t, findings, 1)
		assert.Equal(t, "name", findings[0].Field)
		assert.Equal(t, 2, findings[0].Line, "name is declared on line 2, not the required-field's line 1")
	})

	t.Run("fails with missing file_path", func(t *testing.T) {
		result, err := tool.Execute(ctx, map[string]interface{}{
			"schema_path": schemaFile,
		})
		require.Error(t, err)
		assert.False(t, result.Success)
		assert.ErrorIs(t, err, errUtils.ErrAIToolParameterRequired)
	})

	t.Run("fails with nonexistent file", func(t *testing.T) {
		result, err := tool.Execute(ctx, map[string]interface{}{
			"file_path":   "nonexistent.yaml",
			"schema_path": schemaFile,
		})
		require.Error(t, err)
		assert.False(t, result.Success)
		assert.ErrorIs(t, err, errUtils.ErrAIFileNotFound)
	})
}

func TestValidateFileTool_Execute_AutoResolveSchema(t *testing.T) {
	tmpDir, schemaFile := setupValidateFileFixture(t)
	t.Chdir(tmpDir)

	atmosConfig := &schema.AtmosConfiguration{
		BasePath: tmpDir,
		Schemas: map[string]any{
			"test": schema.SchemaRegistry{
				Schema:  schemaFile,
				Matches: []string{"*.yaml"},
			},
		},
	}
	tool := NewValidateFileTool(atmosConfig)

	result, err := tool.Execute(context.Background(), map[string]interface{}{
		"file_path": "valid.yaml",
	})
	require.NoError(t, err)
	assert.True(t, result.Success)
	assert.Equal(t, schemaFile, result.Data["schema_path"])
}

func TestValidateFileTool_Execute_NoSchemaFound(t *testing.T) {
	tmpDir, _ := setupValidateFileFixture(t)
	t.Chdir(tmpDir)

	atmosConfig := &schema.AtmosConfiguration{BasePath: tmpDir}
	tool := NewValidateFileTool(atmosConfig)

	result, err := tool.Execute(context.Background(), map[string]interface{}{
		"file_path": "valid.yaml",
	})
	require.Error(t, err)
	assert.False(t, result.Success)
	assert.ErrorIs(t, err, errUtils.ErrAINoSchemaForFile)
}
