package datafetcher

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/cloudposse/atmos/pkg/schema"
)

// TestGetDataFetcher_NonExistentFile tests getDataFetcher when file doesn't exist.
func TestGetDataFetcher_NonExistentFile(t *testing.T) {
	dataFetcher := NewDataFetcher(&schema.AtmosConfiguration{})

	// Test with a file path that doesn't exist
	_, err := dataFetcher.GetData("/nonexistent/path/to/file.json")

	assert.Error(t, err)
	assert.True(t, errors.Is(err, ErrUnsupportedSource), "Expected ErrUnsupportedSource for non-existent file")
}

// TestGetDataFetcher_AllSourceTypes tests all source type branches.
func TestGetDataFetcher_AllSourceTypes(t *testing.T) {
	tests := []struct {
		name          string
		source        string
		expectError   bool
		expectedError error
	}{
		{
			name:        "HTTP URL",
			source:      "http://example.com/schema.json",
			expectError: false, // May fail to fetch, but getDataFetcher should succeed
		},
		{
			name:        "HTTPS URL",
			source:      "https://example.com/schema.json",
			expectError: false, // May fail to fetch, but getDataFetcher should succeed
		},
		{
			name:        "Atmos schema",
			source:      "atmos://schema/atmos/manifest/1.0",
			expectError: false,
		},
		{
			name:        "Inline JSON",
			source:      `{"key": "value"}`,
			expectError: false,
		},
		{
			name:          "Non-existent file path",
			source:        "/nonexistent/file/path.json",
			expectError:   true,
			expectedError: ErrUnsupportedSource,
		},
		{
			name:          "Unsupported scheme without file check",
			source:        "ftp://example.com/file",
			expectError:   true,
			expectedError: ErrUnsupportedSource,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			df := NewDataFetcher(&schema.AtmosConfiguration{})

			fetcher, err := df.getDataFetcher(tt.source)

			if tt.expectError {
				assert.Error(t, err)
				if tt.expectedError != nil {
					assert.True(t, errors.Is(err, tt.expectedError))
				}
				assert.Nil(t, fetcher)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, fetcher)
			}
		})
	}
}

// TestGetData_UnsupportedSourcePropagation tests error propagation from getDataFetcher.
func TestGetData_UnsupportedSourcePropagation(t *testing.T) {
	dataFetcher := NewDataFetcher(&schema.AtmosConfiguration{})

	tests := []struct {
		name   string
		source string
	}{
		{
			name:   "Non-existent file",
			source: "/tmp/this-file-should-not-exist-12345.json",
		},
		{
			name:   "Unsupported protocol",
			source: "gopher://example.com/resource",
		},
		{
			name:   "Random unsupported string",
			source: "some-random-string-without-markers",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := dataFetcher.GetData(tt.source)

			assert.Error(t, err)
			assert.True(t, errors.Is(err, ErrUnsupportedSource))
		})
	}
}

// TestNewDataFetcher tests the constructor.
func TestNewDataFetcher(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{}
	fetcher := NewDataFetcher(atmosConfig)

	assert.NotNil(t, fetcher)
	assert.NotNil(t, fetcher.fileDownloader)
	assert.Equal(t, atmosConfig, fetcher.atmosConfig)
}
