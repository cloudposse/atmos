package filetype

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseHCLWithFunctions(t *testing.T) {
	t.Run("atmos::env function works in HCL", func(t *testing.T) {
		t.Setenv("TEST_HCL_ENV", "hello_from_env")

		hclContent := []byte(`greeting = atmos::env("TEST_HCL_ENV")`)
		result, err := parseHCL(hclContent, "test.hcl")
		require.NoError(t, err)

		resultMap, ok := result.(map[string]any)
		require.True(t, ok)
		assert.Equal(t, "hello_from_env", resultMap["greeting"])
	})

	t.Run("atmos::env with default value", func(t *testing.T) {
		hclContent := []byte(`greeting = atmos::env("NONEXISTENT_VAR default_value")`)
		result, err := parseHCL(hclContent, "test.hcl")
		require.NoError(t, err)

		resultMap, ok := result.(map[string]any)
		require.True(t, ok)
		assert.Equal(t, "default_value", resultMap["greeting"])
	})

	t.Run("HCL block syntax with atmos::env function", func(t *testing.T) {
		t.Setenv("BLOCK_TEST_VAR", "block_value")

		hclContent := []byte(`
components {
  terraform {
    myapp {
      vars {
        static_value = "hello"
        dynamic_value = atmos::env("BLOCK_TEST_VAR")
      }
    }
  }
}`)
		result, err := parseHCL(hclContent, "test.hcl")
		require.NoError(t, err)

		resultMap, ok := result.(map[string]any)
		require.True(t, ok)

		// Navigate the nested structure.
		components := resultMap["components"].(map[string]any)
		terraform := components["terraform"].(map[string]any)
		myapp := terraform["myapp"].(map[string]any)
		vars := myapp["vars"].(map[string]any)

		assert.Equal(t, "hello", vars["static_value"])
		assert.Equal(t, "block_value", vars["dynamic_value"])
	})

	t.Run("atmos::repo_root function in HCL", func(t *testing.T) {
		t.Setenv("TEST_GIT_ROOT", "/test/repo")

		hclContent := []byte(`root_path = atmos::repo_root("")`)
		result, err := parseHCL(hclContent, "test.hcl")
		require.NoError(t, err)

		resultMap, ok := result.(map[string]any)
		require.True(t, ok)
		assert.Equal(t, "/test/repo", resultMap["root_path"])
	})
}
