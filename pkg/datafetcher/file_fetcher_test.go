package datafetcher

import (
	"os"
	"testing"
)

func TestFileFetcher(t *testing.T) {
	t.Run("FetchData", func(t *testing.T) {
		t.Run("should return data when file is read", func(t *testing.T) {
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

			// Test the FileFetcher
			fetcher := fileFetcher{}
			data, err := fetcher.FetchData(tmpFile.Name())
			if err != nil {
				t.Fatalf("Failed to fetch data: %v", err)
			}

			// Verify the data matches what we wrote
			if string(data) != string(expectedData) {
				t.Fatalf("Expected data %q, got %q", expectedData, data)
			}
		})
		t.Run("should return error when failed to read file", func(t *testing.T) {
			// Now test the FileFetcher
			fetcher := fileFetcher{}
			_, err := fetcher.FetchData("nonexistentfile")
			if err == nil {
				t.Fatalf("Expected an error, got nil")
			}
		})
	})
}
