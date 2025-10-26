package exec

import (
	"encoding/json"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// Unix-specific test moved to template_utils_unix_test.go:
// - TestCreateTempDirectory

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
	// Parse the URL properly to handle both Windows and Unix paths.
	parsedURL, err := url.Parse(urlStr)
	if err != nil {
		t.Fatalf("failed to parse URL %s: %v", urlStr, err)
	}
	// Convert the URL to a file path.
	filePath := parsedURL.Path
	if runtime.GOOS == "windows" {
		// On Windows, file URLs can have two forms:
		// 1. file://C:/temp/file.json (no leading slash in Path)
		// 2. file:///C:/temp/file.json (leading slash in Path)
		// If there's a leading slash followed by a drive letter, remove the slash.
		if len(filePath) > 2 && filePath[0] == '/' && filePath[2] == ':' {
			filePath = filePath[1:]
		}
		// If the path doesn't have a drive letter, it's likely the URL was
		// constructed incorrectly. Check if Host contains the drive letter.
		if parsedURL.Host != "" && (len(filePath) < 2 || filePath[1] != ':') {
			// Combine host and path for Windows file URLs
			filePath = parsedURL.Host + filePath
		}
		// Convert forward slashes to backslashes for Windows
		filePath = filepath.FromSlash(filePath)
	}
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
	// Parse the URL properly to handle both Windows and Unix paths.
	parsedURL, err := url.Parse(urlStr)
	if err != nil {
		t.Fatalf("failed to parse URL %s: %v", urlStr, err)
	}
	// Convert the URL to a file path.
	filePath := parsedURL.Path
	if runtime.GOOS == "windows" {
		// On Windows, file URLs can have two forms:
		// 1. file://C:/temp/file.json (no leading slash in Path)
		// 2. file:///C:/temp/file.json (leading slash in Path)
		// If there's a leading slash followed by a drive letter, remove the slash.
		if len(filePath) > 2 && filePath[0] == '/' && filePath[2] == ':' {
			filePath = filePath[1:]
		}
		// If the path doesn't have a drive letter, it's likely the URL was
		// constructed incorrectly. Check if Host contains the drive letter.
		if parsedURL.Host != "" && (len(filePath) < 2 || filePath[1] != ':') {
			// Combine host and path for Windows file URLs
			filePath = parsedURL.Host + filePath
		}
		// Convert forward slashes to backslashes for Windows
		filePath = filepath.FromSlash(filePath)
	}
	content, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("failed to read file: %v", err)
	}
	if !strings.Contains(string(content), dummyFileURL) {
		t.Errorf("Expected file content to contain %q, got %q", dummyFileURL, string(content))
	}
}

// TestProcessTmplWithDatasourcesGomplate tests that a static template is rendered correctly.
func TestProcessTmplWithDatasourcesGomplate(t *testing.T) {
	// Test case 1: Static content with no interpolation
	mergedData := map[string]interface{}{
		// No variables to interpolate.
	}
	tmpl := "Static Content"
	result, err := ProcessTmplWithDatasourcesGomplate(nil, "test", tmpl, mergedData, false)
	if err != nil {
		t.Fatalf("ProcessTmplWithDatasourcesGomplate returned error: %v", err)
	}
	// For a static template with no interpolation, the output should equal the template.
	if result != tmpl {
		t.Errorf("Expected result to be %q, got %q", tmpl, result)
	}

	// Test case 2: Template with variable interpolation
	mergedData = map[string]interface{}{
		"name": "Atmos",
	}
	tmpl = " {{- $data := (ds \"config\") -}}\n\nHello {{ $data.name | default \"Project Title\" }}!"

	result, err = ProcessTmplWithDatasourcesGomplate(nil, "test", tmpl, mergedData, false)
	if err != nil {
		t.Fatalf("ProcessTmplWithDatasourcesGomplate returned error: %v", err)
	}
	expected := "Hello Atmos!"
	if result != expected {
		t.Errorf("Expected result to be %q, got %q", expected, result)
	}

	// Test case 3: Nested variable access
	mergedData = map[string]interface{}{
		"config": map[string]interface{}{
			"version": "1.0.0",
		},
	}
	tmpl = "{{- $data := (ds \"config\") -}}\n\nVersion: {{ $data.config.version }}"

	result, err = ProcessTmplWithDatasourcesGomplate(nil, "test", tmpl, mergedData, false)
	if err != nil {
		t.Fatalf("ProcessTmplWithDatasourcesGomplate returned error: %v", err)
	}
	expected = "Version: 1.0.0"
	if result != expected {
		t.Errorf("Expected result to be %q, got %q", expected, result)
	}
}
