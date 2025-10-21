package utils

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/cloudposse/atmos/pkg/schema"
)

// TestIncludeExtensionBased tests the !include function with extension-based parsing.
func TestIncludeExtensionBased(t *testing.T) {
	// Create a temporary directory for test files
	tempDir := t.TempDir()

	// Create test files with different extensions

	// JSON file - should be parsed
	jsonFile := filepath.Join(tempDir, "config.json")
	jsonContent := `{"database": {"host": "localhost", "port": 5432}}`
	err := os.WriteFile(jsonFile, []byte(jsonContent), 0o644)
	assert.NoError(t, err)

	// YAML file - should be parsed
	yamlFile := filepath.Join(tempDir, "settings.yaml")
	yamlContent := `server:
  name: "test-server"
  enabled: true`
	err = os.WriteFile(yamlFile, []byte(yamlContent), 0o644)
	assert.NoError(t, err)

	// HCL file - should be parsed
	hclFile := filepath.Join(tempDir, "terraform.tfvars")
	hclContent := `region = "us-east-1"
instance_type = "t2.micro"`
	err = os.WriteFile(hclFile, []byte(hclContent), 0o644)
	assert.NoError(t, err)

	// Text file - should NOT be parsed (returned as string)
	txtFile := filepath.Join(tempDir, "readme.txt")
	txtContent := `This is plain text
It should not be parsed`
	err = os.WriteFile(txtFile, []byte(txtContent), 0o644)
	assert.NoError(t, err)

	// JSON content with .txt extension - should NOT be parsed
	jsonAsTxtFile := filepath.Join(tempDir, "data.json.txt")
	jsonAsTxtContent := `{"key": "value", "should_parse": false}`
	err = os.WriteFile(jsonAsTxtFile, []byte(jsonAsTxtContent), 0o644)
	assert.NoError(t, err)

	// YAML content with .txt extension - should NOT be parsed
	yamlAsTxtFile := filepath.Join(tempDir, "config.yaml.txt")
	yamlAsTxtContent := `key: value
parse: false`
	err = os.WriteFile(yamlAsTxtFile, []byte(yamlAsTxtContent), 0o644)
	assert.NoError(t, err)

	// Create a test manifest that uses !include
	manifestFile := filepath.Join(tempDir, "test_manifest.yaml")
	manifestContent := `---
components:
  terraform:
    test_component:
      metadata:
        component: test_component
      vars:
        # Extension-based parsing
        json_config: !include config.json
        yaml_config: !include settings.yaml
        hcl_config: !include terraform.tfvars
        text_content: !include readme.txt
        json_as_text: !include data.json.txt
        yaml_as_text: !include config.yaml.txt

        # Test with YQ expressions
        db_host: !include config.json .database.host
        server_name: !include settings.yaml .server.name`

	err = os.WriteFile(manifestFile, []byte(manifestContent), 0o644)
	assert.NoError(t, err)

	// Change to temp directory
	t.Chdir(tempDir)

	// Read and parse the manifest
	yamlFileContent, err := os.ReadFile("test_manifest.yaml")
	assert.NoError(t, err)

	atmosConfig := &schema.AtmosConfiguration{
		BasePath: ".",
		Logs: schema.Logs{
			Level: "Info",
		},
	}

	// Parse the YAML file
	manifest, err := UnmarshalYAMLFromFile[schema.AtmosSectionMapType](atmosConfig, string(yamlFileContent), "test_manifest.yaml")
	assert.NoError(t, err)

	// Get the component vars
	componentVars := manifest["components"].(map[string]any)["terraform"].(map[string]any)["test_component"].(map[string]any)["vars"].(map[string]any)

	// Test JSON parsing
	jsonConfig := componentVars["json_config"].(map[string]any)
	assert.Equal(t, "localhost", jsonConfig["database"].(map[string]any)["host"])
	// Check the port - JSON should parse numbers as float64, but YAML might use int
	port := jsonConfig["database"].(map[string]any)["port"]
	switch v := port.(type) {
	case float64:
		assert.Equal(t, float64(5432), v)
	case int:
		assert.Equal(t, 5432, v)
	default:
		t.Errorf("Unexpected type for port: %T", v)
	}

	// Test YAML parsing
	yamlConfig := componentVars["yaml_config"].(map[string]any)
	assert.Equal(t, "test-server", yamlConfig["server"].(map[string]any)["name"])
	assert.Equal(t, true, yamlConfig["server"].(map[string]any)["enabled"])

	// Test HCL parsing
	hclConfig := componentVars["hcl_config"].(map[string]any)
	assert.Equal(t, "us-east-1", hclConfig["region"])
	assert.Equal(t, "t2.micro", hclConfig["instance_type"])

	// Test text file (should be raw string)
	textContent := componentVars["text_content"].(string)
	assert.Equal(t, txtContent, textContent)

	// Test JSON with .txt extension (should be raw string, NOT parsed)
	jsonAsText := componentVars["json_as_text"].(string)
	assert.Equal(t, jsonAsTxtContent, jsonAsText)

	// Test YAML with .txt extension (should be raw string, NOT parsed)
	yamlAsText := componentVars["yaml_as_text"].(string)
	assert.Equal(t, yamlAsTxtContent, yamlAsText)

	// Test YQ expressions
	assert.Equal(t, "localhost", componentVars["db_host"])
	assert.Equal(t, "test-server", componentVars["server_name"])
}

// TestIncludeRawFunction tests the !include.raw function.
func TestIncludeRawFunction(t *testing.T) {
	// Create a temporary directory for test files
	tempDir := t.TempDir()

	// Create test files with different extensions

	// JSON file
	jsonFile := filepath.Join(tempDir, "config.json")
	jsonContent := `{"type": "json", "parsed": false}`
	err := os.WriteFile(jsonFile, []byte(jsonContent), 0o644)
	assert.NoError(t, err)

	// YAML file
	yamlFile := filepath.Join(tempDir, "settings.yaml")
	yamlContent := `type: yaml
parsed: false`
	err = os.WriteFile(yamlFile, []byte(yamlContent), 0o644)
	assert.NoError(t, err)

	// HCL file
	hclFile := filepath.Join(tempDir, "terraform.tfvars")
	hclContent := `type = "hcl"
parsed = false`
	err = os.WriteFile(hclFile, []byte(hclContent), 0o644)
	assert.NoError(t, err)

	// Text file
	txtFile := filepath.Join(tempDir, "readme.txt")
	txtContent := `Plain text file`
	err = os.WriteFile(txtFile, []byte(txtContent), 0o644)
	assert.NoError(t, err)

	// Create a test manifest that uses !include.raw
	manifestFile := filepath.Join(tempDir, "test_raw_manifest.yaml")
	manifestContent := `---
components:
  terraform:
    test_component:
      metadata:
        component: test_component
      vars:
        # All should be raw strings with !include.raw
        json_raw: !include.raw config.json
        yaml_raw: !include.raw settings.yaml
        hcl_raw: !include.raw terraform.tfvars
        text_raw: !include.raw readme.txt`

	err = os.WriteFile(manifestFile, []byte(manifestContent), 0o644)
	assert.NoError(t, err)

	// Change to temp directory
	t.Chdir(tempDir)

	// Read and parse the manifest
	yamlFileContent, err := os.ReadFile("test_raw_manifest.yaml")
	assert.NoError(t, err)

	atmosConfig := &schema.AtmosConfiguration{
		BasePath: ".",
		Logs: schema.Logs{
			Level: "Info",
		},
	}

	// Parse the YAML file
	manifest, err := UnmarshalYAMLFromFile[schema.AtmosSectionMapType](atmosConfig, string(yamlFileContent), "test_raw_manifest.yaml")
	assert.NoError(t, err)

	// Get the component vars
	componentVars := manifest["components"].(map[string]any)["terraform"].(map[string]any)["test_component"].(map[string]any)["vars"].(map[string]any)

	// All should be raw strings, regardless of extension
	assert.Equal(t, jsonContent, componentVars["json_raw"].(string))
	assert.Equal(t, yamlContent, componentVars["yaml_raw"].(string))
	assert.Equal(t, hclContent, componentVars["hcl_raw"].(string))
	assert.Equal(t, txtContent, componentVars["text_raw"].(string))
}

// TestIncludeWithNoExtension tests files without extensions.
func TestIncludeWithNoExtension(t *testing.T) {
	// Create a temporary directory for test files
	tempDir := t.TempDir()

	// Create files without extensions
	readmeFile := filepath.Join(tempDir, "README")
	readmeContent := `This is a README file without extension`
	err := os.WriteFile(readmeFile, []byte(readmeContent), 0o644)
	assert.NoError(t, err)

	licenseFile := filepath.Join(tempDir, "LICENSE")
	licenseContent := `MIT License`
	err = os.WriteFile(licenseFile, []byte(licenseContent), 0o644)
	assert.NoError(t, err)

	// Hidden file (no extension)
	hiddenFile := filepath.Join(tempDir, ".env")
	hiddenContent := `DATABASE_URL=postgres://localhost/db`
	err = os.WriteFile(hiddenFile, []byte(hiddenContent), 0o644)
	assert.NoError(t, err)

	// Create a test manifest
	manifestFile := filepath.Join(tempDir, "test_noext_manifest.yaml")
	manifestContent := `---
components:
  terraform:
    test_component:
      vars:
        # Files without extensions should be raw strings
        readme: !include README
        license: !include LICENSE
        env: !include .env`

	err = os.WriteFile(manifestFile, []byte(manifestContent), 0o644)
	assert.NoError(t, err)

	// Change to temp directory
	t.Chdir(tempDir)

	// Read and parse the manifest
	yamlFileContent, err := os.ReadFile("test_noext_manifest.yaml")
	assert.NoError(t, err)

	atmosConfig := &schema.AtmosConfiguration{
		BasePath: ".",
		Logs: schema.Logs{
			Level: "Info",
		},
	}

	// Parse the YAML file
	manifest, err := UnmarshalYAMLFromFile[schema.AtmosSectionMapType](atmosConfig, string(yamlFileContent), "test_noext_manifest.yaml")
	assert.NoError(t, err)

	// Get the component vars
	componentVars := manifest["components"].(map[string]any)["terraform"].(map[string]any)["test_component"].(map[string]any)["vars"].(map[string]any)

	// All should be raw strings
	assert.Equal(t, readmeContent, componentVars["readme"].(string))
	assert.Equal(t, licenseContent, componentVars["license"].(string))
	assert.Equal(t, hiddenContent, componentVars["env"].(string))
}

// TestIncludeMixedScenarios tests various mixed scenarios.
func TestIncludeMixedScenarios(t *testing.T) {
	// Create a temporary directory for test files
	tempDir := t.TempDir()

	// File with multiple dots
	multiDotFile := filepath.Join(tempDir, "backup.2024.config.json")
	multiDotContent := `{"backup": "2024", "type": "config"}`
	err := os.WriteFile(multiDotFile, []byte(multiDotContent), 0o644)
	assert.NoError(t, err)

	// Hidden file with extension
	hiddenJsonFile := filepath.Join(tempDir, ".hidden.json")
	hiddenJsonContent := `{"hidden": true}`
	err = os.WriteFile(hiddenJsonFile, []byte(hiddenJsonContent), 0o644)
	assert.NoError(t, err)

	// Create a test manifest
	manifestFile := filepath.Join(tempDir, "test_mixed_manifest.yaml")
	manifestContent := `---
components:
  terraform:
    test_component:
      vars:
        # Multiple dots - should use last extension (.json)
        multi_dot: !include backup.2024.config.json

        # Hidden file with extension - should parse as JSON
        hidden_json: !include .hidden.json

        # Same files with !include.raw
        multi_dot_raw: !include.raw backup.2024.config.json
        hidden_json_raw: !include.raw .hidden.json`

	err = os.WriteFile(manifestFile, []byte(manifestContent), 0o644)
	assert.NoError(t, err)

	// Change to temp directory
	t.Chdir(tempDir)

	// Read and parse the manifest
	yamlFileContent, err := os.ReadFile("test_mixed_manifest.yaml")
	assert.NoError(t, err)

	atmosConfig := &schema.AtmosConfiguration{
		BasePath: ".",
		Logs: schema.Logs{
			Level: "Info",
		},
	}

	// Parse the YAML file
	manifest, err := UnmarshalYAMLFromFile[schema.AtmosSectionMapType](atmosConfig, string(yamlFileContent), "test_mixed_manifest.yaml")
	assert.NoError(t, err)

	// Get the component vars
	componentVars := manifest["components"].(map[string]any)["terraform"].(map[string]any)["test_component"].(map[string]any)["vars"].(map[string]any)

	// Multi-dot file should be parsed as JSON
	multiDot := componentVars["multi_dot"].(map[string]any)
	assert.Equal(t, "2024", multiDot["backup"])
	assert.Equal(t, "config", multiDot["type"])

	// Hidden JSON file should be parsed
	hiddenJson := componentVars["hidden_json"].(map[string]any)
	assert.Equal(t, true, hiddenJson["hidden"])

	// Raw versions should be strings
	assert.Equal(t, multiDotContent, componentVars["multi_dot_raw"].(string))
	assert.Equal(t, hiddenJsonContent, componentVars["hidden_json_raw"].(string))
}
