package datafetcher

import (
	"errors"
	"os"
	"testing"

	"github.com/cloudposse/atmos/pkg/schema"
)

func TestAtmosFetcher(t *testing.T) {
	// Test AtmosFetcher with valid key
	tests := []struct {
		name                string
		source              string
		dataShouldBePresent bool
		err                 error
	}{
		{"Valid key should work", "atmos://schema/atmos/manifest/1.0", true, nil},
		{"Invalid key should not work", "atmos://unknown", false, ErrAtmosSchemaNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.source, func(t *testing.T) {
			fetcher := &atmosFetcher{}
			data, err := fetcher.FetchData(tt.source)
			if !errors.Is(err, tt.err) {
				t.Errorf("Expected error %v, got %v", tt.err, err)
			}
			if tt.dataShouldBePresent {
				if len(data) == 0 {
					t.Errorf("Expected data to be present, got empty data")
				}
			}
		})
	}
}

func TestGetDataFetcher(t *testing.T) {
	// Test URL fetcher
	dataFetcher := NewDataFetcher(&schema.AtmosConfiguration{})
	_, err := dataFetcher.GetData("https://atmos.tools/schemas/atmos/atmos-manifest/1.0/atmos-manifest.json")
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	// Test Atmos fetcher
	_, err = dataFetcher.getDataFetcher("atmos://schema/atmos/manifest/1.0")
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
	_, err = dataFetcher.GetData(tmpFile.Name())
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	// Test inline JSON fetcher
	_, err = dataFetcher.GetData(`{"key": "value"}`)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	// Test unsupported source
	_, err = dataFetcher.GetData("unsupported://source")
	if !errors.Is(err, ErrUnsupportedSource) {
		t.Fatalf("Expected unsupported source error, got %v", err)
	}
}
