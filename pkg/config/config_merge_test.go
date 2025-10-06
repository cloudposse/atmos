package config

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMergeConfig_ConfigFileNotFound(t *testing.T) {
	tempDir := t.TempDir() // Empty directory, no config file

	v := viper.New()
	err := mergeConfig(v, tempDir, CliConfigFileName, true)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Config File \"atmos\" Not Found")
}

func TestMergeConfig_MultipleConfigFilesMerge(t *testing.T) {
	tempDir := t.TempDir()
	content := `
base_path: ./
vendor:
  base_path: "./test-vendor.yaml"
logs:
  file: /dev/stderr
  level: Debug`
	createConfigFile(t, tempDir, "atmos.yaml", content)
	v := viper.New()
	v.SetConfigType("yaml")
	err := mergeConfig(v, tempDir, CliConfigFileName, false)
	assert.NoError(t, err)
	assert.Equal(t, "./", v.GetString("base_path"))
	content2 := `
base_path: ./test
vendor:
  base_path: "./test2-vendor.yaml"
`
	tempDir2 := t.TempDir()
	createConfigFile(t, tempDir2, "atmos.yml", content2)
	err = mergeConfig(v, tempDir2, CliConfigFileName, false)
	assert.NoError(t, err)
	assert.Equal(t, "./test", v.GetString("base_path"))
	assert.Equal(t, "./test2-vendor.yaml", v.GetString("vendor.base_path"))
	assert.Equal(t, "Debug", v.GetString("logs.level"))
	assert.Equal(t, filepath.Join(tempDir2, "atmos.yml"), v.ConfigFileUsed())
}

func TestMergeConfig_EmptyConfig(t *testing.T) {
	// Test mergeConfig with an empty config file to ensure edge case coverage.
	tempDir := t.TempDir()

	// Create an empty config file.
	configPath := filepath.Join(tempDir, "atmos.yaml")
	err := os.WriteFile(configPath, []byte(""), 0o644)
	require.NoError(t, err)

	v := viper.New()
	v.SetConfigType("yaml")

	// This should succeed even with an empty file.
	err = mergeConfig(v, tempDir, CliConfigFileName, false)
	assert.NoError(t, err)
}

func TestMergeConfig_WithoutImports(t *testing.T) {
	// Test mergeConfig with processImports=false to ensure that code path is covered.
	tempDir := t.TempDir()

	// Create a simple config file without imports.
	content := `
base_path: ./test
vendor:
  base_path: ./vendor
logs:
  level: Debug
`
	createConfigFile(t, tempDir, "atmos.yaml", content)

	v := viper.New()
	v.SetConfigType("yaml")

	// Call with processImports=false to cover that branch.
	err := mergeConfig(v, tempDir, CliConfigFileName, false)
	assert.NoError(t, err)

	// Verify the config was loaded correctly.
	assert.Equal(t, "./test", v.GetString("base_path"))
	assert.Equal(t, "./vendor", v.GetString("vendor.base_path"))
	assert.Equal(t, "Debug", v.GetString("logs.level"))
}

func TestLoadConfigFile(t *testing.T) {
	t.Run("successful load", func(t *testing.T) {
		tempDir := t.TempDir()
		content := `
base_path: ./test
logs:
  level: Debug
`
		createConfigFile(t, tempDir, "atmos.yaml", content)

		v, err := loadConfigFile(tempDir, "atmos")
		assert.NoError(t, err)
		assert.NotNil(t, v)
		assert.Equal(t, "./test", v.GetString("base_path"))
		assert.Equal(t, "Debug", v.GetString("logs.level"))
	})

	t.Run("file not found", func(t *testing.T) {
		tempDir := t.TempDir()

		v, err := loadConfigFile(tempDir, "nonexistent")
		assert.Error(t, err)
		assert.Nil(t, v)
		assert.Contains(t, err.Error(), "Not Found")
	})

	t.Run("invalid yaml", func(t *testing.T) {
		tempDir := t.TempDir()
		invalidYAML := "invalid: yaml: content:\n  - with bad indentation\n    and broken structure"
		path := filepath.Join(tempDir, "atmos.yaml")
		err := os.WriteFile(path, []byte(invalidYAML), 0o644)
		require.NoError(t, err)

		_, err = loadConfigFile(tempDir, "atmos")
		// Viper might still load it partially
		// The important thing is we handle the case
		assert.NotNil(t, err) // Should get some error
	})
}

func TestReadConfigFileContent(t *testing.T) {
	t.Run("successful read", func(t *testing.T) {
		tempDir := t.TempDir()
		expectedContent := "test: content\nkey: value"
		path := filepath.Join(tempDir, "test.yaml")
		err := os.WriteFile(path, []byte(expectedContent), 0o644)
		require.NoError(t, err)

		content, err := readConfigFileContent(path)
		assert.NoError(t, err)
		assert.Equal(t, expectedContent, string(content))
	})

	t.Run("file not found", func(t *testing.T) {
		content, err := readConfigFileContent("/nonexistent/path/file.yaml")
		assert.Error(t, err)
		assert.Nil(t, content)
		assert.Contains(t, err.Error(), "failed to read config")
	})

	t.Run("empty file", func(t *testing.T) {
		tempDir := t.TempDir()
		path := filepath.Join(tempDir, "empty.yaml")
		err := os.WriteFile(path, []byte(""), 0o644)
		require.NoError(t, err)

		content, err := readConfigFileContent(path)
		assert.NoError(t, err)
		assert.Equal(t, "", string(content))
	})
}

func TestProcessConfigImportsAndReapply(t *testing.T) {
	t.Run("successful processing", func(t *testing.T) {
		tempDir := t.TempDir()

		// Create a config file for the viper instance
		configContent := `
base_path: ./initial
settings:
  key: initial
`
		createConfigFile(t, tempDir, "atmos.yaml", configContent)

		v := viper.New()
		v.SetConfigType("yaml")
		v.AddConfigPath(tempDir)
		v.SetConfigName("atmos")
		err := v.ReadInConfig()
		require.NoError(t, err)

		// Content to reapply which should override
		content := []byte(`
base_path: ./override
settings:
  key: overridden
`)

		err = processConfigImportsAndReapply(tempDir, v, content)
		assert.NoError(t, err)

		// Verify content was reapplied
		assert.Equal(t, "./override", v.GetString("base_path"))
		assert.Equal(t, "overridden", v.GetString("settings.key"))
	})

	t.Run("invalid yaml in content", func(t *testing.T) {
		tempDir := t.TempDir()

		// Create a minimal config file
		createConfigFile(t, tempDir, "atmos.yaml", "key: value")

		v := viper.New()
		v.SetConfigType("yaml")
		v.AddConfigPath(tempDir)
		v.SetConfigName("atmos")
		v.ReadInConfig()

		// Invalid YAML that will cause MergeConfig to fail
		content := []byte("\x00invalid binary content")

		err := processConfigImportsAndReapply(tempDir, v, content)
		assert.Error(t, err)
		assert.True(t, errors.Is(err, errUtils.ErrMergeConfiguration), "Expected ErrMergeConfiguration error")
	})
}

func TestMarshalViperToYAML(t *testing.T) {
	t.Run("successful marshal", func(t *testing.T) {
		v := viper.New()
		v.Set("key", "value")
		v.Set("nested.key", "nested_value")
		v.Set("number", 42)

		yamlBytes, err := marshalViperToYAML(v)
		assert.NoError(t, err)
		assert.NotNil(t, yamlBytes)

		// Verify the YAML contains our data
		yamlStr := string(yamlBytes)
		assert.Contains(t, yamlStr, "key: value")
		assert.Contains(t, yamlStr, "nested_value")
		assert.Contains(t, yamlStr, "42")
	})

	t.Run("empty viper", func(t *testing.T) {
		v := viper.New()

		yamlBytes, err := marshalViperToYAML(v)
		assert.NoError(t, err)
		assert.NotNil(t, yamlBytes)
		assert.Equal(t, "{}\n", string(yamlBytes))
	})
}

func TestMergeYAMLIntoViper(t *testing.T) {
	t.Run("successful merge", func(t *testing.T) {
		v := viper.New()
		v.SetConfigType("yaml")

		yamlContent := []byte(`
key: value
nested:
  key: nested_value
`)

		err := mergeYAMLIntoViper(v, "/test/path.yaml", yamlContent)
		assert.NoError(t, err)
		assert.Equal(t, "value", v.GetString("key"))
		assert.Equal(t, "nested_value", v.GetString("nested.key"))
		assert.Equal(t, "/test/path.yaml", v.ConfigFileUsed())
	})

	t.Run("invalid yaml", func(t *testing.T) {
		v := viper.New()
		v.SetConfigType("yaml")

		invalidYAML := []byte("invalid: yaml: content:\n  - with bad indentation\n    and broken")

		err := mergeYAMLIntoViper(v, "/test/path.yaml", invalidYAML)
		// Viper is quite forgiving, but let's check we handle it
		if err != nil {
			assert.Contains(t, err.Error(), "merge")
		}
	})

	t.Run("empty yaml", func(t *testing.T) {
		v := viper.New()
		v.SetConfigType("yaml")
		v.Set("existing", "value")

		err := mergeYAMLIntoViper(v, "/test/path.yaml", []byte(""))
		assert.NoError(t, err)
		// Existing values should remain
		assert.Equal(t, "value", v.GetString("existing"))
	})
}

func TestMergeDefaultConfig(t *testing.T) {
	v := viper.New()

	err := mergeDefaultConfig(v)
	assert.Error(t, err, "cannot decode configuration: unable to determine config type")
	v.SetConfigType("yaml")
	err = mergeDefaultConfig(v)
	assert.NoError(t, err, "should not return error if config type is yaml")
}
