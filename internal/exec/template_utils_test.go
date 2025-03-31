// exec_helpers_test.go
package exec

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

// TestCreateTempDirectory verifies that a temporary directory is created with the expected permissions.
func TestCreateTempDirectory(t *testing.T) {
	dir, err := createTempDirectory()
	if err != nil {
		t.Fatalf("createTempDirectory returned error: %v", err)
	}
	defer os.RemoveAll(dir)

	// Check that the directory exists.
	info, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("failed to stat directory: %v", err)
	}
	if !info.IsDir() {
		t.Errorf("Expected a directory, got a file")
	}

	// Check that the permissions are exactly DefaultDirPerm.
	mode := info.Mode().Perm()
	if mode != DefaultDirPerm {
		t.Errorf("Expected mode %o, got %o", DefaultDirPerm, mode)
	}
}

// TestWriteMergedDataToFile tests that merged data is written to a file and returns a valid file URL.
func TestWriteMergedDataToFile(t *testing.T) {
	tempDir, err := createTempDirectory()
	if err != nil {
		t.Fatalf("createTempDirectory error: %v", err)
	}
	defer os.RemoveAll(tempDir)

	mergedData := map[string]interface{}{
		"foo": "bar",
	}

	finalURL, err := writeMergedDataToFile(tempDir, mergedData)
	if err != nil {
		t.Fatalf("writeMergedDataToFile returned error: %v", err)
	}
	if finalURL == nil {
		t.Error("Expected non-nil URL")
	}

	urlStr := finalURL.String()
	if !strings.HasPrefix(urlStr, "file://") {
		t.Errorf("Expected URL to start with file://, got %s", urlStr)
	}

	// Read the file from the URL.
	// Remove the "file://" prefix.
	filePath := strings.TrimPrefix(urlStr, "file://")
	content, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("failed to read file: %v", err)
	}
	var data map[string]interface{}
	if err := json.Unmarshal(content, &data); err != nil {
		t.Fatalf("failed to unmarshal JSON: %v", err)
	}
	if data["foo"] != "bar" {
		t.Errorf("Expected foo to be 'bar', got %v", data["foo"])
	}
}

// TestWriteOuterTopLevelFile tests that the top-level JSON file is written and its URL is valid.
func TestWriteOuterTopLevelFile(t *testing.T) {
	tempDir, err := createTempDirectory()
	if err != nil {
		t.Fatalf("createTempDirectory error: %v", err)
	}
	defer os.RemoveAll(tempDir)

	dummyFileURL := "dummyFileURL"
	finalURL, err := writeOuterTopLevelFile(tempDir, dummyFileURL)
	if err != nil {
		t.Fatalf("writeOuterTopLevelFile returned error: %v", err)
	}
	if finalURL == nil {
		t.Error("Expected non-nil URL")
	}
	urlStr := finalURL.String()
	if !strings.HasPrefix(urlStr, "file://") {
		t.Errorf("Expected URL to start with file://, got %s", urlStr)
	}

	// Verify that the file contains the dummyFileURL.
	filePath := strings.TrimPrefix(urlStr, "file://")
	content, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("failed to read file: %v", err)
	}
	if !strings.Contains(string(content), dummyFileURL) {
		t.Errorf("Expected file content to contain %q, got %q", dummyFileURL, string(content))
	}
}

// TestProcessTmplWithDatasourcesGomplate tests that a static template is rendered correctly.
// Here we use a simple template with no dynamic interpolation.
func TestProcessTmplWithDatasourcesGomplate(t *testing.T) {
	mergedData := map[string]interface{}{
		// No variables to interpolate.
	}
	tmpl := "Static Content"
	result, err := ProcessTmplWithDatasourcesGomplate("test", tmpl, mergedData, false)
	if err != nil {
		t.Fatalf("ProcessTmplWithDatasourcesGomplate returned error: %v", err)
	}
	// For a static template with no interpolation, the output should equal the template.
	if result != tmpl {
		t.Errorf("Expected result to be %q, got %q", tmpl, result)
	}
}
