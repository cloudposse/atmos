package tests

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	e "github.com/cloudposse/atmos/internal/exec"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
)

// TestYAMLFunctionInclude tests the !include YAML function with various file types.
func TestYAMLFunctionInclude(t *testing.T) {
	t.Chdir(filepath.Join(".", "fixtures", "scenarios", "atmos-include-yaml-function"))

	atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, true)
	require.NoError(t, err)
	require.NotNil(t, atmosConfig)

	t.Run("include with YQ expression from JSON file", func(t *testing.T) {
		componentSection, err := e.ExecuteDescribeComponent(
			&e.ExecuteDescribeComponentParams{
				Component: "component-1",
				Stack:     "nonprod",
			},
		)

		require.NoError(t, err)
		require.NotNil(t, componentSection)

		vars, ok := componentSection["vars"].(map[string]interface{})
		require.True(t, ok, "vars should be a map")

		// Verify string_var from JSON file.
		assert.Equal(t, "abc", vars["string_var"], "string_var should be 'abc' from JSON file")

		// Verify boolean_var from YAML file.
		assert.Equal(t, true, vars["boolean_var"], "boolean_var should be true from YAML file")

		// Verify list_var from tfvars file.
		listVar, ok := vars["list_var"].([]interface{})
		require.True(t, ok, "list_var should be a list")
		assert.Equal(t, 3, len(listVar), "list_var should have 3 items")
		assert.Equal(t, "a", listVar[0])
		assert.Equal(t, "b", listVar[1])
		assert.Equal(t, "c", listVar[2])

		// Verify map_var from tfvars file.
		mapVar, ok := vars["map_var"].(map[string]interface{})
		require.True(t, ok, "map_var should be a map")
		assert.Equal(t, 1, mapVar["a"])
		assert.Equal(t, 2, mapVar["b"])
		assert.Equal(t, 3, mapVar["c"])
	})

	t.Run("include entire tfvars file", func(t *testing.T) {
		componentSection, err := e.ExecuteDescribeComponent(
			&e.ExecuteDescribeComponentParams{
				Component: "component-2",
				Stack:     "nonprod",
			},
		)

		require.NoError(t, err)
		require.NotNil(t, componentSection)

		vars, ok := componentSection["vars"].(map[string]interface{})
		require.True(t, ok, "vars should be a map")

		// Verify all vars from tfvars file.
		assert.Contains(t, vars, "string_var")
		assert.Contains(t, vars, "boolean_var")
		assert.Contains(t, vars, "list_var")
		assert.Contains(t, vars, "map_var")
	})

	t.Run("include entire JSON file", func(t *testing.T) {
		componentSection, err := e.ExecuteDescribeComponent(
			&e.ExecuteDescribeComponentParams{
				Component: "component-3",
				Stack:     "nonprod",
			},
		)

		require.NoError(t, err)
		require.NotNil(t, componentSection)

		vars, ok := componentSection["vars"].(map[string]interface{})
		require.True(t, ok, "vars should be a map")

		// Verify string_var from JSON file.
		assert.Equal(t, "abc", vars["string_var"])
	})

	t.Run("include entire YAML file", func(t *testing.T) {
		componentSection, err := e.ExecuteDescribeComponent(
			&e.ExecuteDescribeComponentParams{
				Component: "component-4",
				Stack:     "nonprod",
			},
		)

		require.NoError(t, err)
		require.NotNil(t, componentSection)

		vars, ok := componentSection["vars"].(map[string]interface{})
		require.True(t, ok, "vars should be a map")

		// Verify vars from YAML file.
		assert.Equal(t, "abc", vars["string_var"])
		assert.Equal(t, true, vars["boolean_var"])
	})

	t.Run("include from remote URL with YQ expression", func(t *testing.T) {
		// This tests the remote include with YQ expression.
		// The stack includes settings from a remote URL.
		componentSection, err := e.ExecuteDescribeComponent(
			&e.ExecuteDescribeComponentParams{
				Component: "component-1",
				Stack:     "nonprod",
			},
		)

		require.NoError(t, err)
		require.NotNil(t, componentSection)

		// The settings section should be included from the remote file.
		settings, ok := componentSection["settings"].(map[string]interface{})
		require.True(t, ok, "settings should be a map")
		require.NotEmpty(t, settings, "settings should not be empty (from remote include)")
	})
}

// TestYAMLFunctionIncludeExtended tests extended !include scenarios including
// !include.raw, .txt, .tf, extensionless files, and advanced YQ expressions.
func TestYAMLFunctionIncludeExtended(t *testing.T) {
	t.Chdir(filepath.Join(".", "fixtures", "scenarios", "atmos-include-yaml-function"))

	atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, true)
	require.NoError(t, err)
	require.NotNil(t, atmosConfig)

	t.Run("include.raw returns raw string for YAML file", func(t *testing.T) {
		componentSection, err := e.ExecuteDescribeComponent(
			&e.ExecuteDescribeComponentParams{
				Component: "component-5",
				Stack:     "nonprod",
			},
		)

		require.NoError(t, err)
		require.NotNil(t, componentSection)

		vars, ok := componentSection["vars"].(map[string]interface{})
		require.True(t, ok, "vars should be a map")

		// !include.raw should return raw string, not a parsed map.
		yamlRaw, ok := vars["yaml_raw"].(string)
		require.True(t, ok, "yaml_raw should be a string, got %T", vars["yaml_raw"])
		assert.Contains(t, yamlRaw, "string_var")
	})

	t.Run("include.raw returns raw string for JSON file", func(t *testing.T) {
		componentSection, err := e.ExecuteDescribeComponent(
			&e.ExecuteDescribeComponentParams{
				Component: "component-5",
				Stack:     "nonprod",
			},
		)

		require.NoError(t, err)
		require.NotNil(t, componentSection)

		vars, ok := componentSection["vars"].(map[string]interface{})
		require.True(t, ok, "vars should be a map")

		// !include.raw should return raw string, not a parsed map.
		jsonRaw, ok := vars["json_raw"].(string)
		require.True(t, ok, "json_raw should be a string, got %T", vars["json_raw"])
		assert.Contains(t, jsonRaw, "\"string_var\"")
	})

	t.Run("YQ array index expression", func(t *testing.T) {
		componentSection, err := e.ExecuteDescribeComponent(
			&e.ExecuteDescribeComponentParams{
				Component: "component-6",
				Stack:     "nonprod",
			},
		)

		require.NoError(t, err)
		require.NotNil(t, componentSection)

		vars, ok := componentSection["vars"].(map[string]interface{})
		require.True(t, ok, "vars should be a map")

		// .list_var[0] should return "a".
		assert.Equal(t, "a", vars["first_item"], "first_item should be 'a' from list_var[0]")
	})

	t.Run("YQ nested map key expression", func(t *testing.T) {
		componentSection, err := e.ExecuteDescribeComponent(
			&e.ExecuteDescribeComponentParams{
				Component: "component-6",
				Stack:     "nonprod",
			},
		)

		require.NoError(t, err)
		require.NotNil(t, componentSection)

		vars, ok := componentSection["vars"].(map[string]interface{})
		require.True(t, ok, "vars should be a map")

		// .map_var.a should return 1.
		assert.Equal(t, 1, vars["map_value"], "map_value should be 1 from map_var.a")
	})

	t.Run("include txt file as raw string", func(t *testing.T) {
		componentSection, err := e.ExecuteDescribeComponent(
			&e.ExecuteDescribeComponentParams{
				Component: "component-7",
				Stack:     "nonprod",
			},
		)

		require.NoError(t, err)
		require.NotNil(t, componentSection)

		vars, ok := componentSection["vars"].(map[string]interface{})
		require.True(t, ok, "vars should be a map")

		// .txt files should be returned as raw strings.
		note, ok := vars["note"].(string)
		require.True(t, ok, "note should be a string, got %T", vars["note"])
		assert.Contains(t, note, "plain text note")
	})

	t.Run("include extensionless file as raw string", func(t *testing.T) {
		componentSection, err := e.ExecuteDescribeComponent(
			&e.ExecuteDescribeComponentParams{
				Component: "component-8",
				Stack:     "nonprod",
			},
		)

		require.NoError(t, err)
		require.NotNil(t, componentSection)

		vars, ok := componentSection["vars"].(map[string]interface{})
		require.True(t, ok, "vars should be a map")

		// Extensionless files should be returned as raw strings.
		readme, ok := vars["readme"].(string)
		require.True(t, ok, "readme should be a string, got %T", vars["readme"])
		assert.Contains(t, readme, "Extensionless file")
	})

	t.Run("include tf file with HCL parsing", func(t *testing.T) {
		componentSection, err := e.ExecuteDescribeComponent(
			&e.ExecuteDescribeComponentParams{
				Component: "component-9",
				Stack:     "nonprod",
			},
		)

		require.NoError(t, err)
		require.NotNil(t, componentSection)

		vars, ok := componentSection["vars"].(map[string]interface{})
		require.True(t, ok, "vars should be a map")

		// .tf files should be parsed as HCL (same as .tfvars).
		assert.Equal(t, "t3.micro", vars["instance_type"], "instance_type should be 't3.micro'")
		assert.Equal(t, "us-west-2", vars["region"], "region should be 'us-west-2'")
	})
}

// TestYAMLFunctionIncludeEdgeCases tests edge cases for the !include function.
func TestYAMLFunctionIncludeEdgeCases(t *testing.T) {
	t.Chdir(filepath.Join(".", "fixtures", "scenarios", "atmos-include-yaml-function"))

	_, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, true)
	require.NoError(t, err)

	t.Run("include preserves types", func(t *testing.T) {
		componentSection, err := e.ExecuteDescribeComponent(
			&e.ExecuteDescribeComponentParams{
				Component: "component-1",
				Stack:     "nonprod",
			},
		)

		require.NoError(t, err)
		vars := componentSection["vars"].(map[string]interface{})

		// String type.
		_, ok := vars["string_var"].(string)
		assert.True(t, ok, "string_var should be a string type")

		// Boolean type.
		_, ok = vars["boolean_var"].(bool)
		assert.True(t, ok, "boolean_var should be a bool type")

		// List type.
		_, ok = vars["list_var"].([]interface{})
		assert.True(t, ok, "list_var should be a list type")

		// Map type.
		_, ok = vars["map_var"].(map[string]interface{})
		assert.True(t, ok, "map_var should be a map type")
	})

	t.Run("all components load successfully", func(t *testing.T) {
		components := []string{
			"component-1",
			"component-2",
			"component-3",
			"component-4",
			"component-5",
			"component-6",
			"component-7",
			"component-8",
			"component-9",
		}

		for _, componentName := range components {
			t.Run(componentName, func(t *testing.T) {
				componentSection, err := e.ExecuteDescribeComponent(
					&e.ExecuteDescribeComponentParams{
						Component: componentName,
						Stack:     "nonprod",
					},
				)

				require.NoError(t, err, "component %s should load without errors", componentName)
				require.NotNil(t, componentSection)
			})
		}
	})
}
