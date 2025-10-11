package exec

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"

	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/types"
	"github.com/stretchr/testify/assert"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

// MockLayer implements v1.Layer for testing.
type MockLayer struct {
	digestVal       v1.Hash
	sizeVal         int64
	uncompressedErr error
	compressedErr   error
}

func (m *MockLayer) Digest() (v1.Hash, error) {
	return m.digestVal, nil
}

func (m *MockLayer) DiffID() (v1.Hash, error) {
	return v1.Hash{}, nil
}

func (m *MockLayer) Compressed() (io.ReadCloser, error) {
	return nil, m.compressedErr
}

func (m *MockLayer) Uncompressed() (io.ReadCloser, error) {
	if m.uncompressedErr != nil {
		return nil, m.uncompressedErr
	}
	return nil, nil
}

func (m *MockLayer) Size() (int64, error) {
	return m.sizeVal, nil
}

func (m *MockLayer) MediaType() (types.MediaType, error) {
	return types.DockerLayer, nil
}

// TestProcessOciImage_InvalidReference tests error handling for invalid image references.
func TestProcessOciImage_InvalidReference(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{}

	// Test with invalid image reference.
	err := processOciImage(atmosConfig, "invalid::image//name", "/tmp/dest")

	assert.Error(t, err)
	assert.True(t, errors.Is(err, errUtils.ErrInvalidImageReference), "Expected ErrInvalidImageReference, got: %v", err)
	assert.Contains(t, err.Error(), "invalid image reference")
}

// TestProcessOciImage_TempDirCreationFailure tests error handling when temp directory creation fails.
func TestProcessOciImage_TempDirCreationFailure(t *testing.T) {
	// This test would require mocking os.MkdirTemp which is challenging without dependency injection.
	// We'll skip this test as it requires complex mocking.
	t.Skip("Requires complex mocking of os.MkdirTemp")
}

// TestProcessLayer_DecompressionError tests error handling when layer decompression fails.
func TestProcessLayer_DecompressionError(t *testing.T) {
	mockLayer := &MockLayer{
		digestVal:       v1.Hash{Algorithm: "sha256", Hex: "abc123"},
		sizeVal:         1024,
		uncompressedErr: fmt.Errorf("decompression failed"),
	}

	err := processLayer(mockLayer, 0, "/tmp/dest")

	assert.Error(t, err)
	assert.True(t, errors.Is(err, errUtils.ErrLayerDecompression), "Expected ErrLayerDecompression, got: %v", err)
	assert.Contains(t, err.Error(), "layer decompression")
}

// TestCheckArtifactType tests the checkArtifactType function with various media types.
func TestCheckArtifactType(t *testing.T) {
	tests := []struct {
		name      string
		mediaType types.MediaType
		imageName string
	}{
		{
			name:      "Docker image",
			mediaType: types.DockerManifestSchema2,
			imageName: "test/image:latest",
		},
		{
			name:      "OCI image",
			mediaType: types.OCIManifestSchema1,
			imageName: "oci/image:v1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Since checkArtifactType requires a remote.Descriptor,
			// and we can't easily create one for testing, we'll skip this test.
			t.Skipf("checkArtifactType requires remote.Descriptor which is hard to mock")
		})
	}
}

// TestRemoveTempDir tests the removeTempDir function.
func TestRemoveTempDir_OCIUtils(t *testing.T) {
	// Create a temporary directory for testing.
	tempDir, err := os.MkdirTemp("", "test-remove-")
	assert.NoError(t, err)

	// Ensure directory exists.
	_, err = os.Stat(tempDir)
	assert.NoError(t, err)

	// Remove the directory.
	removeTempDir(tempDir)

	// Verify directory was removed.
	_, err = os.Stat(tempDir)
	assert.True(t, os.IsNotExist(err))
}

// TestRemoveTempDir_NonExistent tests removeTempDir with non-existent directory.
func TestRemoveTempDir_NonExistent(t *testing.T) {
	// This should not panic, just log an error.
	removeTempDir("/nonexistent/directory/path")
	// Test passes if no panic occurs.
}

// TestParseOCIManifest tests the parseOCIManifest function.
func TestParseOCIManifest(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		expectError bool
	}{
		{
			name: "Valid OCI manifest",
			input: `{
				"schemaVersion": 2,
				"mediaType": "application/vnd.oci.image.manifest.v1+json",
				"config": {
					"mediaType": "application/vnd.oci.image.config.v1+json",
					"digest": "sha256:abc123",
					"size": 1024
				},
				"layers": [
					{
						"mediaType": "application/vnd.oci.image.layer.v1.tar+gzip",
						"digest": "sha256:layer1",
						"size": 2048
					}
				]
			}`,
			expectError: false,
		},
		{
			name: "Minimal valid manifest",
			input: `{
				"schemaVersion": 2
			}`,
			expectError: false,
		},
		{
			name:        "Invalid JSON",
			input:       `{"schemaVersion": 2,`,
			expectError: true,
		},
		{
			name:        "Empty JSON",
			input:       `{}`,
			expectError: false,
		},
		{
			name:        "Invalid structure",
			input:       `"just a string"`,
			expectError: true,
		},
		{
			name:        "Array instead of object",
			input:       `[1, 2, 3]`,
			expectError: true,
		},
		{
			name:        "Empty string",
			input:       ``,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader := strings.NewReader(tt.input)
			manifest, err := parseOCIManifest(reader)

			if tt.expectError {
				assert.Error(t, err)
				assert.Nil(t, manifest)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, manifest)
			}
		})
	}
}
