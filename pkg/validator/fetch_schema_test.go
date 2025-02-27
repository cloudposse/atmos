package validator

import (
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/cloudposse/atmos/pkg/schema"
)

func TestURLFetcher(t *testing.T) {
	// Test with a valid URL (mocking the HTTP response)
	fetcher := &URLFetcher{URL: "https://atmos.tools/schemas/atmos/atmos-manifest/1.0/atmos-manifest.json"}
	data, err := fetcher.Fetch()
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	if !strings.Contains(string(data), "JSON Schema for Atmos Stack Manifest files") {
		t.Errorf("Expected atmos schema, got %s", data)
	}
}

func TestFileFetcher(t *testing.T) {
	// Create a temporary file for the test
	tmpFile, err := os.CreateTemp("", "testfile-")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name()) // Ensure the file is deleted after the test
	// Use file (e.g., file.Name() to get the name, file.Write() to write to it, etc.)
	defer tmpFile.Close()

	// Write some test data into the temporary file
	expectedData := []byte("File content")
	if _, err := tmpFile.Write(expectedData); err != nil {
		t.Fatalf("Failed to write to temp file: %v", err)
	}

	// Close the file before reading it
	if err := tmpFile.Close(); err != nil {
		t.Fatalf("Failed to close temp file: %v", err)
	}

	// Now test the FileFetcher
	fetcher := &FileFetcher{FilePath: tmpFile.Name()}
	data, err := fetcher.Fetch()
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	if string(data) != string(expectedData) {
		t.Errorf("Expected '%s', got '%s'", expectedData, data)
	}
}

func TestAtmosFetcher(t *testing.T) {
	// Test AtmosFetcher with valid key
	fetcher := &AtmosFetcher{Key: "atmos://config"}
	data, err := fetcher.Fetch()
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	if string(data) != "This is Atmos configuration" {
		t.Errorf("Expected Atmos configuration, got %s", data)
	}

	// Test AtmosFetcher with invalid key
	fetcher = &AtmosFetcher{Key: "atmos://unknown"}
	_, err = fetcher.Fetch()
	if !errors.Is(err, ErrAtmosSchemaNotFound) {
		t.Errorf("Expected 'atmos key not found' error, got %v", err)
	}
}

func TestGetDataFetcher(t *testing.T) {
	// Test URL fetcher
	dataFetcher := NewDataFetcher()
	_, err := dataFetcher.GetData(&schema.AtmosConfiguration{}, "https://atmos.tools/schemas/atmos/atmos-manifest/1.0/atmos-manifest.json")
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	// Test Atmos fetcher
	_, err = dataFetcher.getDataFetcher(&schema.AtmosConfiguration{}, "atmos://config")
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	// Create a temporary file for the test
	tmpFile, err := os.CreateTemp("", "testfile-")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name()) // Ensure the file is deleted after the test
	// Use file (e.g., file.Name() to get the name, file.Write() to write to it, etc.)
	defer tmpFile.Close()

	// Write some test data into the temporary file
	expectedData := []byte("File content")
	if _, err := tmpFile.Write(expectedData); err != nil {
		t.Fatalf("Failed to write to temp file: %v", err)
	}

	// Close the file before reading it
	if err := tmpFile.Close(); err != nil {
		t.Fatalf("Failed to close temp file: %v", err)
	}

	// Test File fetcher
	_, err = dataFetcher.GetData(&schema.AtmosConfiguration{}, tmpFile.Name())
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
}
