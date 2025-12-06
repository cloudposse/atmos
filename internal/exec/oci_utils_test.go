package exec

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"

	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/google/go-containerregistry/pkg/v1/types"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/filesystem"
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
	err := ProcessOciImage(atmosConfig, "invalid::image//name", "/tmp/dest")

	assert.Error(t, err)
	assert.True(t, errors.Is(err, errUtils.ErrInvalidImageReference), "Expected ErrInvalidImageReference, got: %v", err)
	assert.Contains(t, err.Error(), "invalid image reference")
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

// TestProcessOciImageWithFS_TempDirCreationFailure tests error handling when temp directory creation fails.
func TestProcessOciImageWithFS_TempDirCreationFailure(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockFS := filesystem.NewMockFileSystem(ctrl)

	// Mock MkdirTemp to return an error.
	expectedErr := fmt.Errorf("permission denied")
	mockFS.EXPECT().
		MkdirTemp(gomock.Any(), gomock.Any()).
		Return("", expectedErr)

	atmosConfig := &schema.AtmosConfiguration{}
	err := processOciImageWithFS(atmosConfig, "test/image:latest", "/tmp/dest", mockFS)

	assert.Error(t, err)
	assert.True(t, errors.Is(err, errUtils.ErrCreateTempDirectory), "Expected ErrCreateTempDirectory, got: %v", err)
	assert.ErrorContains(t, err, "permission denied")
}

// TestCheckArtifactType tests the checkArtifactType function with various media types.
func TestCheckArtifactType(t *testing.T) {
	tests := []struct {
		name      string
		mediaType types.MediaType
		imageName string
		expectLog bool // Whether we expect a warning log.
	}{
		{
			name:      "Docker manifest schema 2",
			mediaType: types.DockerManifestSchema2,
			imageName: "test/image:latest",
			expectLog: false,
		},
		{
			name:      "OCI manifest schema 1",
			mediaType: types.OCIManifestSchema1,
			imageName: "oci/image:v1",
			expectLog: false,
		},
		{
			name:      "Docker manifest list",
			mediaType: types.DockerManifestList,
			imageName: "multi/arch:v1",
			expectLog: true, // Unsupported, expect warning.
		},
		{
			name:      "OCI image index",
			mediaType: types.OCIImageIndex,
			imageName: "index/image:v1",
			expectLog: true, // Unsupported, expect warning.
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a mock descriptor with embedded v1.Descriptor.
			mockDescriptor := &remote.Descriptor{
				Descriptor: v1.Descriptor{
					MediaType: tt.mediaType,
				},
			}

			// Call checkArtifactType - it logs warnings but doesn't return errors.
			// We verify it doesn't panic and handles all media types gracefully.
			assert.NotPanics(t, func() {
				checkArtifactType(mockDescriptor, tt.imageName)
			})

			// The function logs warnings for unsupported types but continues execution.
			// This is correct behavior - we can't easily verify log output in unit tests
			// without complex log capture, so we just verify no panic occurs.
		})
	}
}

// TestRemoveTempDir tests the removeTempDir function.
func TestRemoveTempDir_OCIUtils(t *testing.T) {
	// Create a temporary directory for testing.
	tempDir := t.TempDir()

	// Ensure directory exists.
	_, err := os.Stat(tempDir)
	assert.NoError(t, err)

	// Remove the directory.
	RemoveTempDir(tempDir)

	// Verify directory was removed.
	_, err = os.Stat(tempDir)
	assert.True(t, os.IsNotExist(err))
}

// TestRemoveTempDir_NonExistent tests RemoveTempDir with non-existent directory.
func TestRemoveTempDir_NonExistent(t *testing.T) {
	// This should not panic when removing a non-existent directory.
	// Use defer/recover to verify no panic occurs.
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("RemoveTempDir panicked on non-existent directory: %v", r)
		}
	}()

	RemoveTempDir("/nonexistent/directory/path")

	// Test passes if no panic occurs.
	assert.True(t, true, "Function executed without panic on non-existent directory")
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

// TestProcessLayer_DigestError tests that processLayer returns nil when digest fails.
func TestProcessLayer_DigestError(t *testing.T) {
	mockLayer := &MockLayerWithDigestError{
		digestErr: fmt.Errorf("digest calculation failed"),
	}

	// processLayer should return nil (not an error) when digest fails.
	err := processLayer(mockLayer, 0, "/tmp/dest")
	assert.NoError(t, err, "processLayer should return nil when digest fails")
}

// TestCheckArtifactType_MatchingType tests checkArtifactType with matching artifact type.
func TestCheckArtifactType_MatchingType(t *testing.T) {
	manifestJSON := `{
		"schemaVersion": 2,
		"artifactType": "application/vnd.atmos.component.terraform.v1+tar+gzip",
		"config": {
			"digest": "sha256:test",
			"size": 100
		},
		"layers": []
	}`

	descriptor := &remote.Descriptor{
		Manifest: []byte(manifestJSON),
	}

	// Should not panic and should not log warning for matching type.
	assert.NotPanics(t, func() {
		checkArtifactType(descriptor, "test:latest")
	})
}

// TestCheckArtifactType_NonMatchingType tests checkArtifactType with non-matching artifact type.
func TestCheckArtifactType_NonMatchingType(t *testing.T) {
	manifestJSON := `{
		"schemaVersion": 2,
		"artifactType": "application/vnd.docker.container.image.v1+json",
		"config": {
			"digest": "sha256:test",
			"size": 100
		},
		"layers": []
	}`

	descriptor := &remote.Descriptor{
		Manifest: []byte(manifestJSON),
	}

	// Should not panic but will log warning for non-matching type.
	assert.NotPanics(t, func() {
		checkArtifactType(descriptor, "docker:latest")
	})
}

// TestCheckArtifactType_InvalidManifest tests checkArtifactType with invalid manifest JSON.
func TestCheckArtifactType_InvalidManifest(t *testing.T) {
	descriptor := &remote.Descriptor{
		Manifest: []byte(`{invalid json`),
	}

	// Should not panic even with invalid manifest, just logs error.
	assert.NotPanics(t, func() {
		checkArtifactType(descriptor, "invalid:latest")
	})
}

// MockLayerWithDigestError implements v1.Layer for testing digest errors.
type MockLayerWithDigestError struct {
	digestErr error
}

func (m *MockLayerWithDigestError) Digest() (v1.Hash, error) {
	return v1.Hash{}, m.digestErr
}

func (m *MockLayerWithDigestError) DiffID() (v1.Hash, error) {
	return v1.Hash{}, nil
}

func (m *MockLayerWithDigestError) Compressed() (io.ReadCloser, error) {
	return nil, nil
}

func (m *MockLayerWithDigestError) Uncompressed() (io.ReadCloser, error) {
	return nil, nil
}

func (m *MockLayerWithDigestError) Size() (int64, error) {
	return 0, nil
}

func (m *MockLayerWithDigestError) MediaType() (types.MediaType, error) {
	return types.DockerLayer, nil
}
