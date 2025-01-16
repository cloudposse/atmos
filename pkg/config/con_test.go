package config

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadEmbeddedConfigSuccessfullyMergesConfig(t *testing.T) {
	viper := viper.New()
	viper.SetConfigType("yaml")
	cl := &ConfigLoader{
		viper:       viper,
		atmosConfig: schema.AtmosConfiguration{},
	}
	err := cl.loadSchemaDefaults()
	assert.NoError(t, err)
	err = cl.loadEmbeddedConfig()

	assert.NoError(t, err)
	assert.NotNil(t, viper.AllSettings())
	// Deep Merge Schema Defaults and Embedded Config
	err = cl.deepMergeConfig()
	assert.NoError(t, err)
}

// Successfully unmarshal valid config data from viper into atmosConfig struct
func TestDeepMergeConfigUnmarshalsValidConfig(t *testing.T) {
	v := viper.New()
	v.SetConfigType("yaml")

	validConfig := []byte(`
    stacks:
        base_path: "stacks"
    `)

	err := v.ReadConfig(bytes.NewBuffer(validConfig))
	require.NoError(t, err)

	cl := &ConfigLoader{
		viper:       v,
		atmosConfig: schema.AtmosConfiguration{},
	}

	err = cl.deepMergeConfig()
	require.NoError(t, err)

	require.Equal(t, "stacks", cl.atmosConfig.Stacks.BasePath)
}

// Returns list of atmos config files with supported extensions  .yaml, .yml
func TestSearchAtmosConfigFileDir_ReturnsConfigFilesWithSupportedExtensions(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "atmos-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// Create test files with different extensions
	files := []string{
		"atmos.yaml",
		"atmos.yml",
	}

	for _, f := range files {
		path := filepath.Join(tmpDir, f)
		if err := os.WriteFile(path, []byte(""), 0644); err != nil {
			t.Fatal(err)
		}
	}
	viper := viper.New()
	viper.SetConfigType("yaml")
	cl := &ConfigLoader{
		viper:       viper,
		atmosConfig: schema.AtmosConfiguration{},
	}
	got, err := cl.SearchAtmosConfigFileDir(tmpDir)
	if err != nil {
		t.Fatal(err)
	}

	if len(got) != 1 {
		t.Errorf("Expected 1 config files, got %d", len(got))
	}

	// Verify extensions are in correct order
	if !strings.HasSuffix(got[0], "atmos.yaml") {
		t.Errorf("Expected config files with supported extensions, got %v", got)
	}
}

// Successfully load single config file from valid command line argument
func TestLoadExplicitConfigsWithValidConfigFile(t *testing.T) {
	// Setup test config file
	tmpDir := t.TempDir()
	defer os.RemoveAll(tmpDir)
	configPath := filepath.Join(tmpDir, "atmos.yaml")

	err := os.WriteFile(configPath, []byte("test: config"), 0644)
	require.NoError(t, err)

	// Save and restore os.Args
	oldArgs := os.Args
	defer func() { os.Args = oldArgs }()
	os.Args = []string{"cmd", "--config", configPath}

	cl := &ConfigLoader{
		atmosConfig: schema.AtmosConfiguration{},
		viper:       viper.New(),
	}

	err = cl.loadExplicitConfigs()
	require.NoError(t, err)

	assert.True(t, cl.configFound)
	assert.Equal(t, configPath, cl.atmosConfig.CliConfigPath)
	assert.Contains(t, cl.AtmosConfigPaths, configPath)
}

// Handle missing --config flag value
func TestLoadExplicitConfigsWithMissingConfigValue(t *testing.T) {
	// Save and restore os.Args
	oldArgs := os.Args
	defer func() { os.Args = oldArgs }()
	os.Args = []string{"cmd", "--config"}

	cl := &ConfigLoader{
		atmosConfig: schema.AtmosConfiguration{},
		viper:       viper.New(),
	}

	err := cl.loadExplicitConfigs()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--config flag provided without a value")
	assert.False(t, cl.configFound)
	assert.Empty(t, cl.AtmosConfigPaths)
}

// Successfully load multiple config file from valid command line argument and directories
func TestLoadExplicitConfigsWithMultipleConfigFiles(t *testing.T) {
	oldArgs := os.Args
	defer func() { os.Args = oldArgs }()
	// Setup test config files
	tmpDir := t.TempDir()
	defer os.RemoveAll(tmpDir)
	configPath1 := filepath.Join(tmpDir, "atmos.yaml")
	configPath2 := filepath.Join(tmpDir, "atmos.yml")
	err := os.WriteFile(configPath1, []byte("test: config1"), 0644)
	require.NoError(t, err)
	err = os.WriteFile(configPath2, []byte("test: config2"), 0644)
	require.NoError(t, err)
	os.Args = []string{"cmd", "--config", configPath1, "--config", configPath2}
	cl := &ConfigLoader{
		atmosConfig: schema.AtmosConfiguration{},
		viper:       viper.New(),
	}
	err = cl.loadExplicitConfigs()
	require.NoError(t, err)
	assert.True(t, cl.configFound)
	paths := ConnectPaths([]string{configPath1, configPath2})
	assert.Equal(t, paths, cl.atmosConfig.CliConfigPath)
	assert.Contains(t, cl.AtmosConfigPaths, configPath1)
	assert.Contains(t, cl.AtmosConfigPaths, configPath2)
	// test read from dir
	os.Args = []string{"cmd", "--config", tmpDir}
	cl = &ConfigLoader{
		atmosConfig: schema.AtmosConfiguration{},
		viper:       viper.New(),
	}
	err = cl.loadExplicitConfigs()
	require.NoError(t, err)
	assert.True(t, cl.configFound)
	assert.Equal(t, configPath1, cl.atmosConfig.CliConfigPath)
	assert.Contains(t, cl.AtmosConfigPaths, configPath1)
}

// Function correctly prioritizes .yaml over .yml for same base filename
func TestDetectPriorityFilesPreferYamlOverYml(t *testing.T) {
	cl := &ConfigLoader{}

	files := []string{
		"config/app.yml",
		"config/app.yaml",
		"config/db.yml",
	}

	result := cl.detectPriorityFiles(files)

	expected := []string{
		"config/app.yaml",
		"config/db.yml",
	}

	result = cl.sortFilesByDepth(result)
	expected = cl.sortFilesByDepth(expected)

	if !reflect.DeepEqual(result, expected) {
		t.Errorf("Expected %v but got %v", expected, result)
	}
}

// Sort files by directory depth in ascending order
func TestSortFilesByDepthSortsFilesCorrectly(t *testing.T) {
	cl := &ConfigLoader{}

	files := []string{
		"a/b/c/file1.yaml",
		"x/file2.yaml",
		"file1.yaml",
		"a/b/file3.yaml",
		"file4.yaml",
	}

	expected := []string{
		"file1.yaml",
		"file4.yaml",
		"x/file2.yaml",
		"a/b/file3.yaml",
		"a/b/c/file1.yaml",
	}

	result := cl.sortFilesByDepth(files)

	if !reflect.DeepEqual(result, expected) {
		t.Errorf("Expected %v but got %v", expected, result)
	}
}

func TestDownloadRemoteConfig(t *testing.T) {
	// Create a mock HTTP server
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("mock content"))
	}))
	defer mockServer.Close()

	t.Run("Valid URL", func(t *testing.T) {
		tempDir, tempFile, err := downloadRemoteConfig(mockServer.URL)
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		defer os.RemoveAll(tempDir)

		// Verify the temporary file contains the correct content
		content, err := os.ReadFile(tempFile)
		if err != nil {
			t.Fatalf("Failed to read temp file: %v", err)
		}
		if string(content) != "mock content" {
			t.Errorf("Unexpected file content: got %v, want %v", string(content), "mock content")
		}
	})

	t.Run("Invalid URL", func(t *testing.T) {
		_, _, err := downloadRemoteConfig("http://invalid-url")
		if err == nil {
			t.Fatal("Expected an error for invalid URL, got nil")
		}
	})

}

// Test for processImports
func TestProcessImports(t *testing.T) {
	// Step 1: Setup a mock HTTP server for a remote URL
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "key: value") // Mock YAML content
	}))
	defer server.Close()

	// Step 2: Create temporary base directory and files
	baseDir := t.TempDir()
	defer os.Remove(baseDir)
	// Step 2.1: Create a directory for recursive imports
	configDir := filepath.Join(baseDir, "configs.d")
	err := os.MkdirAll(configDir, 0755)
	assert.NoError(t, err)

	// Create mock configuration files in the directory
	configFile1 := filepath.Join(configDir, "config1.yaml")
	err = os.WriteFile(configFile1, []byte("key1: value1"), 0644)
	assert.NoError(t, err)

	configFile2 := filepath.Join(configDir, "config2.yaml")
	err = os.WriteFile(configFile2, []byte("key2: value2"), 0644)
	assert.NoError(t, err)

	// Step 2.2: Create a specific local file
	localFile := filepath.Join(baseDir, "logs.yaml")
	err = os.WriteFile(localFile, []byte("key3: value3"), 0644)
	assert.NoError(t, err)
	// step 2.3
	configDir2 := filepath.Join(baseDir, "config")
	err = os.MkdirAll(configDir2, 0755)
	assert.NoError(t, err)
	configFile3 := filepath.Join(configDir2, "config3.yaml")
	err = os.WriteFile(configFile3, []byte("key4: value4"), 0644)
	assert.NoError(t, err)
	// Step 3: Define test imports
	imports := []string{
		server.URL,               // Remote URL
		"configs.d/**/*",         // Recursive directory
		"config/**/*.yaml",       //recursive/**/*.yaml", // Recursive directory with specific pattern extension
		"./logs.yaml",            // Specific local file
		"http://invalid-url.com", // Invalid URL
		"",                       // Empty import path
	}

	// Step 4: Prepare the ConfigLoader instance
	configLoader := &ConfigLoader{
		atmosConfig: schema.AtmosConfiguration{
			BasePath: baseDir,
			Import:   imports,
		},
	}

	// Step 5: Run the processImports method
	resolvedPaths, err := configLoader.processImports()

	// Step 6: Assertions
	assert.NoError(t, err, "processImports should not return an error")

	// Verify resolved paths contain expected files
	expectedPaths := []string{
		filepath.Join(baseDir, "logs.yaml"),
		configFile1,
		configFile2,
		configFile3,
	}
	for _, expectedPath := range expectedPaths {
		assert.Contains(t, resolvedPaths, expectedPath, fmt.Sprintf("resolvedPaths should contain %s", expectedPath))
	}

	// Ensure invalid and empty imports are handled gracefully
	assert.NotContains(t, resolvedPaths, "http://invalid-url.com", "Invalid URL should not be resolved")
	assert.NotContains(t, resolvedPaths, "", "Empty import path should not be resolved")
}
