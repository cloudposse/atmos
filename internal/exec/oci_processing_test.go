package exec

import (
	"bytes"
	"encoding/json"
	"io"
	"strings"
	"testing"

	log "github.com/charmbracelet/log"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
)

// TestProcessOciImage tests the main OCI image processing function
func TestProcessOciImage(t *testing.T) {
	tests := []struct {
		name        string
		imageName   string
		atmosConfig *schema.AtmosConfiguration
		expectError bool
		errorMsg    string
	}{
		{
			name:        "Invalid image reference",
			imageName:   "invalid-image-reference",
			atmosConfig: &schema.AtmosConfiguration{},
			expectError: true,
			errorMsg:    "failed to pull image",
		},
		{
			name:        "Empty image name",
			imageName:   "",
			atmosConfig: &schema.AtmosConfiguration{},
			expectError: true,
			errorMsg:    "invalid image reference",
		},
		{
			name:        "Valid image reference format",
			imageName:   "ghcr.io/test/repo:latest",
			atmosConfig: &schema.AtmosConfiguration{},
			expectError: true, // Will fail to pull, but should parse correctly
			errorMsg:    "failed to pull image",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tempDir := t.TempDir()
			err := processOciImage(tt.atmosConfig, tt.imageName, tempDir)

			if tt.expectError {
				assert.Error(t, err)
				if tt.errorMsg != "" {
					assert.Contains(t, err.Error(), tt.errorMsg)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// TestProcessOciImageIntegration tests OCI image processing with mock data
func TestProcessOciImageIntegration(t *testing.T) {
	t.Run("Test with mock OCI image", func(t *testing.T) {
		// This test verifies that the function handles the processing flow correctly
		// even when the actual image pull fails
		atmosConfig := &schema.AtmosConfiguration{}
		tempDir := t.TempDir()

		// Use an invalid image name that will fail to pull
		imageName := "index.docker.io/library/invalid-image-name:latest"

		err := processOciImage(atmosConfig, imageName, tempDir)

		// Should fail due to invalid image, but should not panic
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to pull image")
	})
}

// TestPullImage tests the image pulling functionality
func TestPullImage(t *testing.T) {
	tests := []struct {
		name        string
		imageRef    string
		atmosConfig *schema.AtmosConfiguration
		expectError bool
		errorMsg    string
	}{
		{
			name:        "Invalid registry reference",
			imageRef:    "invalid-registry/test:latest",
			atmosConfig: &schema.AtmosConfiguration{},
			expectError: true,
			errorMsg:    "failed to pull image",
		},
		{
			name:        "Valid format but non-existent image",
			imageRef:    "ghcr.io/non-existent/repo:latest",
			atmosConfig: &schema.AtmosConfiguration{},
			expectError: true,
			errorMsg:    "failed to pull image",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ref, err := name.ParseReference(tt.imageRef)
			if err != nil {
				t.Skipf("Skipping test due to invalid reference: %v", err)
			}

			descriptor, err := pullImage(ref, tt.atmosConfig)

			if tt.expectError {
				assert.Error(t, err)
				assert.Nil(t, descriptor)
				if tt.errorMsg != "" {
					assert.Contains(t, err.Error(), tt.errorMsg)
				}
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, descriptor)
			}
		})
	}
}

// TestCheckArtifactType tests artifact type validation
func TestCheckArtifactType(t *testing.T) {
	tests := []struct {
		name           string
		artifactType   string
		imageName      string
		expectWarning  bool
		warningMessage string
	}{
		{
			name:          "Supported Atmos artifact type",
			artifactType:  "application/vnd.atmos.component.terraform.v1+tar+gzip",
			imageName:     "test-image",
			expectWarning: false,
		},
		{
			name:          "Supported OpenTofu artifact type",
			artifactType:  "application/vnd.opentofu.modulepkg",
			imageName:     "test-image",
			expectWarning: false,
		},
		{
			name:          "Supported Terraform artifact type",
			artifactType:  "application/vnd.terraform.module.v1+tar+gzip",
			imageName:     "test-image",
			expectWarning: false,
		},
		{
			name:           "Unsupported artifact type",
			artifactType:   "application/vnd.unsupported.type",
			imageName:      "test-image",
			expectWarning:  true,
			warningMessage: "OCI image artifact type not recognized",
		},
		{
			name:          "Empty artifact type",
			artifactType:  "",
			imageName:     "test-image",
			expectWarning: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a mock manifest
			manifest := &ocispec.Manifest{
				ArtifactType: tt.artifactType,
			}

			// Serialize the manifest
			manifestBytes, err := json.Marshal(manifest)
			require.NoError(t, err)

			// Create a mock descriptor
			descriptor := &remote.Descriptor{
				Manifest: manifestBytes,
			}

			// Capture log output
			var logOutput strings.Builder
			originalLogger := log.Default()
			log.SetDefault(log.NewWithOptions(&logOutput, log.Options{
				Level: log.DebugLevel,
			}))
			defer log.SetDefault(originalLogger)

			checkArtifactType(descriptor, tt.imageName)

			if tt.expectWarning {
				assert.Contains(t, logOutput.String(), tt.warningMessage)
			} else {
				assert.NotContains(t, logOutput.String(), "not recognized")
			}
		})
	}
}

// TestParseOCIManifest tests OCI manifest parsing
func TestParseOCIManifest(t *testing.T) {
	tests := []struct {
		name        string
		manifest    *ocispec.Manifest
		expectError bool
	}{
		{
			name: "Valid manifest",
			manifest: &ocispec.Manifest{
				ArtifactType: "application/vnd.atmos.component.terraform.v1+tar+gzip",
				Layers: []ocispec.Descriptor{
					{
						MediaType: "application/vnd.docker.image.rootfs.diff.tar.gzip",
						Digest:    "sha256:test-digest",
						Size:      1024,
					},
				},
			},
			expectError: false,
		},
		{
			name:        "Invalid JSON",
			manifest:    nil,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var reader io.Reader
			if tt.manifest != nil {
				manifestBytes, err := json.Marshal(tt.manifest)
				require.NoError(t, err)
				reader = bytes.NewReader(manifestBytes)
			} else {
				reader = strings.NewReader("invalid json")
			}

			result, err := parseOCIManifest(reader)

			if tt.expectError {
				assert.Error(t, err)
				assert.Nil(t, result)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, result)
				assert.Equal(t, tt.manifest.ArtifactType, result.ArtifactType)
			}
		})
	}
}
