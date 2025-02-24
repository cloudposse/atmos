package config

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func setupTestFile(content, tempDir string, filename string) (string, error) {
	filePath := filepath.Join(tempDir, filename)
	err := os.WriteFile(filePath, []byte(content), 0o600)
	return filePath, err
}

// Test for processImports.
func TestProcessImports(t *testing.T) {
	// Step 1: Setup a mock HTTP server for a remote URL
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "base_path: ./") // Mock YAML content
	}))
	defer server.Close()

	// Step 2: Create temporary base directory and files
	baseDir := t.TempDir()
	defer os.Remove(baseDir)
	// Step 2.1: Create a directory for recursive imports
	configDir := filepath.Join(baseDir, "configs.d")
	err := os.MkdirAll(configDir, 0o755)
	assert.NoError(t, err)

	// Create mock configuration files in the directory
	configFile1 := filepath.Join(configDir, "config1.yaml")
	err = os.WriteFile(configFile1, []byte("key1: value1"), 0o600)
	assert.NoError(t, err)

	configFile2 := filepath.Join(configDir, "config2.yaml")
	err = os.WriteFile(configFile2, []byte("key2: value2"), 0o600)
	assert.NoError(t, err)

	// Step 2.2: Create a specific local file
	localFile := filepath.Join(baseDir, "logs.yaml")
	err = os.WriteFile(localFile, []byte("key3: value3"), 0o600)
	assert.NoError(t, err)
	// step 2.3
	configDir2 := filepath.Join(baseDir, "config")
	err = os.MkdirAll(configDir2, 0o755)
	assert.NoError(t, err)
	configFile3 := filepath.Join(configDir2, "config3.yaml")
	err = os.WriteFile(configFile3, []byte("key4: value4"), 0o600)
	assert.NoError(t, err)
	// Step 3: Define test imports
	imports := []string{
		server.URL,               // Remote URL
		"configs.d/**/*",         // Recursive directory
		"config/**/*.yaml",       // recursive/**/*.yaml", // Recursive directory with specific pattern extension
		"./logs.yaml",            // Specific local file
		"http://invalid-url.com", // Invalid URL
		"",                       // Empty import path
		"/config/foo.yaml",       // Invalid import path
	}

	// Step 5: Run the processImports method
	imported, err := processImports(baseDir, imports, baseDir, 0, 10)

	// Step 6: Assertions
	assert.NoError(t, err, "processImports should not return an error")
	var resolvedPaths []string
	for _, resolvedPath := range imported {
		resolvedPaths = append(resolvedPaths, resolvedPath.filePath)
	}

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

func TestProcessImportNested(t *testing.T) {
	baseDir, err := os.MkdirTemp("", "config-test")
	assert.NoError(t, err)
	defer os.RemoveAll(baseDir)

	// Setting up test files
	_, err = setupTestFile(`
import:
 - "./nested-local.yaml"
 `, baseDir, "local.yaml")
	assert.NoError(t, err)

	nestedLocalConfigPath, err := setupTestFile(`import: []`, baseDir, "nested-local.yaml")
	assert.NoError(t, err)

	remoteContent := `
import:
  - nested-local.yaml
`
	nestedRemoteContent := `import: []`
	// Create an HTTP server to simulate remote imports
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/config.yaml":
			fmt.Fprint(w, remoteContent)
		case "/nested-remote.yaml":
			fmt.Fprint(w, nestedRemoteContent)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	t.Run("Test remote import processing", func(t *testing.T) {
		tempDir, err := os.MkdirTemp("", "config-test")
		assert.NoError(t, err)
		defer os.RemoveAll(tempDir)
		importPaths := []string{server.URL + "/config.yaml"}
		resolved, err := processImports(baseDir, importPaths, tempDir, 1, 5)
		assert.NoError(t, err)
		assert.Len(t, resolved, 2, "should resolve main and nested remote imports")
	})

	t.Run("Test local import processing", func(t *testing.T) {
		tempDir, err := os.MkdirTemp("", "config-test")
		assert.NoError(t, err)
		defer os.RemoveAll(tempDir)
		importPaths := []string{"local.yaml"}
		imported, err := processImports(baseDir, importPaths, tempDir, 1, 5)
		assert.NoError(t, err)
		var resolvedPaths []string
		for _, resolvedPath := range imported {
			resolvedPaths = append(resolvedPaths, resolvedPath.filePath)
		}
		assert.Contains(t, resolvedPaths, nestedLocalConfigPath, "should resolve nested local imports")
	})

	t.Run("Test mixed imports with depth limit", func(t *testing.T) {
		tempDir, err := os.MkdirTemp("", "config-test")
		assert.NoError(t, err)
		defer os.RemoveAll(tempDir)
		importPaths := []string{
			"local.yaml",
			server.URL + "/config.yaml",
		}
		resolved, err := processImports(baseDir, importPaths, tempDir, 11, 10)
		assert.Error(t, err, "should return an error when maxDepth is exceeded")
		assert.Nil(t, resolved, "no resolved paths should be returned on depth limit breach")
	})
}
