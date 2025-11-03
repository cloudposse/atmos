package utils

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zclconf/go-cty/cty"
)

// TestWriteTerraformBackendConfigToFileAsHcl tests writing backend config with complex types.
func TestWriteTerraformBackendConfigToFileAsHcl(t *testing.T) {
	t.Run("writes backend config with assume_role object", func(t *testing.T) {
		tempDir := t.TempDir()
		backendFile := filepath.Join(tempDir, "backend.tf")

		backendConfig := map[string]any{
			"acl":     "bucket-owner-full-control",
			"bucket":  "test-bucket",
			"encrypt": true,
			"key":     "terraform.tfstate",
			"region":  "us-east-1",
			"assume_role": map[string]any{
				"role_arn": "arn:aws:iam::123456789012:role/TestRole",
			},
		}

		err := WriteTerraformBackendConfigToFileAsHcl(backendFile, "s3", backendConfig)
		require.NoError(t, err)

		// Verify file was created.
		_, err = os.Stat(backendFile)
		require.NoError(t, err)

		// Read and verify content.
		content, err := os.ReadFile(backendFile)
		require.NoError(t, err)

		contentStr := string(content)

		// Check that assume_role is present.
		assert.Contains(t, contentStr, "assume_role")
		assert.Contains(t, contentStr, "role_arn")
		assert.Contains(t, contentStr, "arn:aws:iam::123456789012:role/TestRole")

		// Check other fields.
		assert.Contains(t, contentStr, "bucket")
		assert.Contains(t, contentStr, "test-bucket")
		assert.Contains(t, contentStr, "encrypt")
		assert.Contains(t, contentStr, "true")
	})

	t.Run("writes backend config with nested objects", func(t *testing.T) {
		tempDir := t.TempDir()
		backendFile := filepath.Join(tempDir, "backend.tf")

		backendConfig := map[string]any{
			"bucket": "test-bucket",
			"assume_role": map[string]any{
				"role_arn":     "arn:aws:iam::123456789012:role/TestRole",
				"session_name": "terraform",
				"external_id":  "test-external-id",
			},
		}

		err := WriteTerraformBackendConfigToFileAsHcl(backendFile, "s3", backendConfig)
		require.NoError(t, err)

		content, err := os.ReadFile(backendFile)
		require.NoError(t, err)

		contentStr := string(content)

		// Check all nested fields are present.
		assert.Contains(t, contentStr, "assume_role")
		assert.Contains(t, contentStr, "role_arn")
		assert.Contains(t, contentStr, "session_name")
		assert.Contains(t, contentStr, "external_id")
		assert.Contains(t, contentStr, "test-external-id")
	})

	t.Run("writes backend config with primitive types", func(t *testing.T) {
		tempDir := t.TempDir()
		backendFile := filepath.Join(tempDir, "backend.tf")

		backendConfig := map[string]any{
			"string_val": "test",
			"bool_val":   true,
			"int_val":    int64(42),
			"float_val":  3.14,
		}

		err := WriteTerraformBackendConfigToFileAsHcl(backendFile, "s3", backendConfig)
		require.NoError(t, err)

		content, err := os.ReadFile(backendFile)
		require.NoError(t, err)

		contentStr := string(content)

		assert.Contains(t, contentStr, "string_val")
		assert.Contains(t, contentStr, "test")
		assert.Contains(t, contentStr, "bool_val")
		assert.Contains(t, contentStr, "true")
		assert.Contains(t, contentStr, "int_val")
		assert.Contains(t, contentStr, "42")
		assert.Contains(t, contentStr, "float_val")
		assert.Contains(t, contentStr, "3.14")
	})

	t.Run("writes backend config with list values", func(t *testing.T) {
		tempDir := t.TempDir()
		backendFile := filepath.Join(tempDir, "backend.tf")

		backendConfig := map[string]any{
			"bucket": "test-bucket",
			"tags": []any{
				"tag1",
				"tag2",
				"tag3",
			},
		}

		err := WriteTerraformBackendConfigToFileAsHcl(backendFile, "s3", backendConfig)
		require.NoError(t, err)

		content, err := os.ReadFile(backendFile)
		require.NoError(t, err)

		contentStr := string(content)

		assert.Contains(t, contentStr, "tags")
		assert.Contains(t, contentStr, "tag1")
		assert.Contains(t, contentStr, "tag2")
		assert.Contains(t, contentStr, "tag3")
	})

	t.Run("writes backend config with empty map", func(t *testing.T) {
		tempDir := t.TempDir()
		backendFile := filepath.Join(tempDir, "backend.tf")

		backendConfig := map[string]any{
			"bucket":      "test-bucket",
			"empty_attrs": map[string]any{},
		}

		err := WriteTerraformBackendConfigToFileAsHcl(backendFile, "s3", backendConfig)
		require.NoError(t, err)

		content, err := os.ReadFile(backendFile)
		require.NoError(t, err)

		contentStr := string(content)

		assert.Contains(t, contentStr, "bucket")
		assert.Contains(t, contentStr, "test-bucket")
	})

	t.Run("writes backend config with nil value", func(t *testing.T) {
		tempDir := t.TempDir()
		backendFile := filepath.Join(tempDir, "backend.tf")

		backendConfig := map[string]any{
			"bucket":    "test-bucket",
			"nil_value": nil,
		}

		err := WriteTerraformBackendConfigToFileAsHcl(backendFile, "s3", backendConfig)
		require.NoError(t, err)

		content, err := os.ReadFile(backendFile)
		require.NoError(t, err)

		contentStr := string(content)

		assert.Contains(t, contentStr, "bucket")
		assert.Contains(t, contentStr, "test-bucket")
		// nil values should not be written.
		assert.NotContains(t, contentStr, "nil_value")
	})

	t.Run("creates terraform backend block structure", func(t *testing.T) {
		tempDir := t.TempDir()
		backendFile := filepath.Join(tempDir, "backend.tf")

		backendConfig := map[string]any{
			"bucket": "test-bucket",
		}

		err := WriteTerraformBackendConfigToFileAsHcl(backendFile, "s3", backendConfig)
		require.NoError(t, err)

		content, err := os.ReadFile(backendFile)
		require.NoError(t, err)

		contentStr := string(content)

		// Check terraform block structure.
		assert.Contains(t, contentStr, "terraform {")
		assert.Contains(t, contentStr, "backend \"s3\" {")
		assert.True(t, strings.Count(contentStr, "}") >= 2)
	})
}

// TestConvertGoValueToCty tests the conversion of Go values to cty.Value.
func TestConvertGoValueToCty(t *testing.T) {
	t.Run("converts string", func(t *testing.T) {
		val, err := convertGoValueToCty("test")
		require.NoError(t, err)
		assert.Equal(t, "test", val.AsString())
	})

	t.Run("converts bool", func(t *testing.T) {
		val, err := convertGoValueToCty(true)
		require.NoError(t, err)
		assert.True(t, val.True())
	})

	t.Run("converts int64", func(t *testing.T) {
		val, err := convertGoValueToCty(int64(42))
		require.NoError(t, err)
		assert.NotNil(t, val)
	})

	t.Run("converts float64", func(t *testing.T) {
		val, err := convertGoValueToCty(3.14)
		require.NoError(t, err)
		assert.NotNil(t, val)
	})

	t.Run("converts map", func(t *testing.T) {
		input := map[string]any{
			"key1": "value1",
			"key2": "value2",
		}
		val, err := convertGoValueToCty(input)
		require.NoError(t, err)
		assert.NotNil(t, val)
		assert.True(t, val.Type().IsObjectType())
	})

	t.Run("converts nested map", func(t *testing.T) {
		input := map[string]any{
			"outer": map[string]any{
				"inner": "value",
			},
		}
		val, err := convertGoValueToCty(input)
		require.NoError(t, err)
		assert.NotNil(t, val)
		assert.True(t, val.Type().IsObjectType())
	})

	t.Run("converts slice", func(t *testing.T) {
		input := []any{"item1", "item2", "item3"}
		val, err := convertGoValueToCty(input)
		require.NoError(t, err)
		assert.NotNil(t, val)
		assert.True(t, val.Type().IsTupleType())
	})

	t.Run("converts empty map", func(t *testing.T) {
		input := map[string]any{}
		val, err := convertGoValueToCty(input)
		require.NoError(t, err)
		assert.NotNil(t, val)
	})

	t.Run("converts empty slice", func(t *testing.T) {
		input := []any{}
		val, err := convertGoValueToCty(input)
		require.NoError(t, err)
		assert.NotNil(t, val)
	})

	t.Run("converts nil", func(t *testing.T) {
		val, err := convertGoValueToCty(nil)
		require.NoError(t, err)
		assert.True(t, val.IsNull())
	})

	t.Run("converts int32", func(t *testing.T) {
		val, err := convertGoValueToCty(int32(42))
		require.NoError(t, err)
		assert.NotNil(t, val)
		assert.Equal(t, cty.Number, val.Type())
	})

	t.Run("converts uint32", func(t *testing.T) {
		val, err := convertGoValueToCty(uint32(42))
		require.NoError(t, err)
		assert.NotNil(t, val)
		assert.Equal(t, cty.Number, val.Type())
	})

	t.Run("converts uint", func(t *testing.T) {
		val, err := convertGoValueToCty(uint(42))
		require.NoError(t, err)
		assert.NotNil(t, val)
		assert.Equal(t, cty.Number, val.Type())
	})

	t.Run("converts float32", func(t *testing.T) {
		val, err := convertGoValueToCty(float32(3.14))
		require.NoError(t, err)
		assert.NotNil(t, val)
		assert.Equal(t, cty.Number, val.Type())
	})
}
