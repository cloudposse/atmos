package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMergeConfig_ImportOverrideBehavior(t *testing.T) {
	// Test that the main config file's settings override imported settings.
	tempDir := t.TempDir()

	// Create an import file with a command.
	importDir := filepath.Join(tempDir, "imports")
	err := os.Mkdir(importDir, 0o755)
	require.NoError(t, err)

	importContent := `
commands:
  - name: "imported-command"
    description: "This is from import"
settings:
  imported: true
  shared: "from-import"
`
	createConfigFile(t, importDir, "commands.yaml", importContent)

	// Create main config that imports the above file and overrides the command.
	mainContent := `
base_path: ./
import:
  - "./imports/commands.yaml"
commands:
  - name: "main-command"
    description: "This is from main"
settings:
  main: true
  shared: "from-main"
`
	createConfigFile(t, tempDir, "atmos.yaml", mainContent)

	v := viper.New()
	v.SetConfigType("yaml")
	err = mergeConfig(v, tempDir, CliConfigFileName, true)
	assert.NoError(t, err)

	// Verify that main config overrides imports.
	commands := v.Get("commands")
	assert.NotNil(t, commands)

	// Verify that commands were replaced, not appended.
	commandsList, ok := commands.([]interface{})
	assert.True(t, ok, "commands should be a slice")
	assert.Equal(t, 1, len(commandsList), "should have exactly one command (imported commands replaced)")

	// Verify the single command is from the main config.
	if len(commandsList) > 0 {
		cmd, ok := commandsList[0].(map[string]interface{})
		assert.True(t, ok, "command should be a map")
		assert.Equal(t, "main-command", cmd["name"], "command should be from main config")
		assert.Equal(t, "This is from main", cmd["description"])
	}

	// The main config's settings should override imported settings.
	assert.Equal(t, "from-main", v.GetString("settings.shared"))
	assert.True(t, v.GetBool("settings.main"))
	// Note: settings.imported is NOT present because the entire settings section
	// from the main config replaces the imported settings section.
}

func TestMergeConfig_ImportDeepMerge(t *testing.T) {
	// Test that imports are deep merged at the top level, but sections are replaced.
	tempDir := t.TempDir()

	// Create an import file with various settings.
	importDir := filepath.Join(tempDir, "imports")
	err := os.Mkdir(importDir, 0o755)
	require.NoError(t, err)

	importContent := `
base_path: /imported
vendor:
  base_path: /imported/vendor
  setting1: imported
logs:
  level: Debug
  file: /imported.log
`
	createConfigFile(t, importDir, "base.yaml", importContent)

	// Create main config that imports and partially overrides.
	mainContent := `
base_path: ./
import:
  - "./imports/base.yaml"
vendor:
  base_path: /main/vendor
  setting2: main
logs:
  level: Info
`
	createConfigFile(t, tempDir, "atmos.yaml", mainContent)

	v := viper.New()
	v.SetConfigType("yaml")
	err = mergeConfig(v, tempDir, CliConfigFileName, true)
	assert.NoError(t, err)

	// base_path from main config should override import.
	assert.Equal(t, "./", v.GetString("base_path"))

	// vendor section is completely replaced by main config.
	assert.Equal(t, "/main/vendor", v.GetString("vendor.base_path"))
	assert.Equal(t, "main", v.GetString("vendor.setting2"))
	assert.False(t, v.IsSet("vendor.setting1"), "vendor.setting1 should not exist (section replaced)")

	// logs section is completely replaced by main config.
	assert.Equal(t, "Info", v.GetString("logs.level"))
	assert.False(t, v.IsSet("logs.file"), "logs.file should not exist (section replaced)")
}

func TestMergeConfig_AtmosDCommandsMerging(t *testing.T) {
	// Test that commands from .atmos.d are merged with main config commands.
	tempDir := t.TempDir()

	// Create .atmos.d directory with a command file.
	atmosDDir := filepath.Join(tempDir, ".atmos.d")
	err := os.Mkdir(atmosDDir, 0o755)
	require.NoError(t, err)

	atmosDContent := `
commands:
  - name: "dev"
    description: "Development workflow commands"
    commands:
      - name: "setup"
        description: "Set up development environment"
        steps:
          - echo "Setting up..."
`
	createConfigFile(t, atmosDDir, "dev.yaml", atmosDContent)

	// Create main config with its own commands.
	mainContent := `
base_path: ./
commands:
  - name: "terraform"
    description: "Terraform commands"
  - name: "helmfile"
    description: "Helmfile commands"
`
	createConfigFile(t, tempDir, "atmos.yaml", mainContent)

	v := viper.New()
	v.SetConfigType("yaml")
	err = mergeConfig(v, tempDir, CliConfigFileName, true)
	assert.NoError(t, err)

	// Verify that commands from both .atmos.d and main config are present.
	commands := v.Get("commands")
	assert.NotNil(t, commands)

	commandsList, ok := commands.([]interface{})
	assert.True(t, ok, "commands should be a slice")
	assert.Equal(t, 3, len(commandsList), "should have all 3 commands (1 from .atmos.d + 2 from main)")

	// Verify all commands are present.
	commandNames := make(map[string]bool)
	for _, cmd := range commandsList {
		cmdMap, ok := cmd.(map[string]interface{})
		assert.True(t, ok, "command should be a map")
		name, ok := cmdMap["name"].(string)
		assert.True(t, ok, "command should have a name")
		commandNames[name] = true
		t.Logf("Found command: %s", name)
	}

	assert.True(t, commandNames["dev"], "dev command from .atmos.d should be present")
	assert.True(t, commandNames["terraform"], "terraform command from main config should be present")
	assert.True(t, commandNames["helmfile"], "helmfile command from main config should be present")
}

func TestMergeConfig_ProcessImportsWithInvalidYAML(t *testing.T) {
	// Test error handling when import file contains invalid YAML.
	tempDir := t.TempDir()

	// Create an import file with invalid YAML.
	importDir := filepath.Join(tempDir, "imports")
	err := os.Mkdir(importDir, 0o755)
	require.NoError(t, err)

	// Write invalid YAML content directly.
	invalidYAMLPath := filepath.Join(importDir, "invalid.yaml")
	err = os.WriteFile(invalidYAMLPath, []byte("invalid: yaml: content:\n  - with bad indentation\n    and broken structure"), 0o644)
	require.NoError(t, err)

	// Create main config that tries to import the invalid file.
	mainContent := `
base_path: ./
import:
  - "./imports/invalid.yaml"
`
	createConfigFile(t, tempDir, "atmos.yaml", mainContent)

	v := viper.New()
	v.SetConfigType("yaml")
	// This should still succeed as invalid imports are logged but not fatal.
	err = mergeConfig(v, tempDir, CliConfigFileName, true)
	assert.NoError(t, err)
}

func TestMergeConfig_ComplexImportHierarchy(t *testing.T) {
	// Test complex import hierarchy to improve coverage of import processing.
	tempDir := t.TempDir()

	// Create a chain of imports: A imports B, B imports C.
	importDir := filepath.Join(tempDir, "imports")
	err := os.Mkdir(importDir, 0o755)
	require.NoError(t, err)

	// Create C (base config).
	configC := `
base_path: /from-c
settings:
  level: 3
  from_c: true
`
	createConfigFile(t, importDir, "c.yaml", configC)

	// Create B (imports C).
	configB := `
import:
  - "./c.yaml"
settings:
  level: 2
  from_b: true
`
	createConfigFile(t, importDir, "b.yaml", configB)

	// Create A (imports B).
	configA := `
base_path: ./
import:
  - "./imports/b.yaml"
settings:
  level: 1
  from_a: true
`
	createConfigFile(t, tempDir, "atmos.yaml", configA)

	v := viper.New()
	v.SetConfigType("yaml")
	err = mergeConfig(v, tempDir, CliConfigFileName, true)
	assert.NoError(t, err)

	// Verify the hierarchy: A overrides B, B overrides C.
	assert.Equal(t, "./", v.GetString("base_path"))
	assert.Equal(t, 1, v.GetInt("settings.level"))
	assert.True(t, v.GetBool("settings.from_a"))
	// B and C's unique settings should not exist (sections are replaced).
	assert.False(t, v.IsSet("settings.from_b"))
	assert.False(t, v.IsSet("settings.from_c"))
}
