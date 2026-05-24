package exec

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"strings"
	"testing"

	cockroachErrors "github.com/cockroachdb/errors"
	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/google/go-containerregistry/pkg/v1/remote/transport"
	"github.com/google/go-containerregistry/pkg/v1/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
	err := processOciImage(atmosConfig, "invalid::image//name", "/tmp/dest")

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
	removeTempDir(tempDir)

	// Verify directory was removed.
	_, err = os.Stat(tempDir)
	assert.True(t, os.IsNotExist(err))
}

// TestRemoveTempDir_NonExistent tests removeTempDir with non-existent directory.
func TestRemoveTempDir_NonExistent(t *testing.T) {
	// This should not panic when removing a non-existent directory.
	// Use defer/recover to verify no panic occurs.
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("removeTempDir panicked on non-existent directory: %v", r)
		}
	}()

	removeTempDir("/nonexistent/directory/path")

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

// pullImageShim records calls and returns a programmed sequence of results from
// remote.Get. Tests reassign the package-level remoteGet to one of these shims
// and restore it on teardown.
type pullImageShim struct {
	results []pullImageResult
	calls   int
}

type pullImageResult struct {
	descriptor *remote.Descriptor
	err        error
}

func (s *pullImageShim) get(_ name.Reference, _ ...remote.Option) (*remote.Descriptor, error) {
	idx := s.calls
	s.calls++
	if idx >= len(s.results) {
		// Defensive: an unexpected extra call should fail the test loudly.
		return nil, fmt.Errorf("pullImageShim: unexpected call %d (only %d programmed)", idx+1, len(s.results))
	}
	r := s.results[idx]
	return r.descriptor, r.err
}

// installPullImageShim swaps remoteGet for the duration of the test.
func installPullImageShim(t *testing.T, shim *pullImageShim) {
	t.Helper()
	original := remoteGet
	remoteGet = shim.get
	t.Cleanup(func() { remoteGet = original })
}

// ghcrConfigWithCreds returns a config that causes getGHCRAuth to resolve a
// non-anonymous Basic auth for ghcr.io.
func ghcrConfigWithCreds() *schema.AtmosConfiguration {
	return &schema.AtmosConfiguration{
		Settings: schema.AtmosSettings{
			AtmosGithubToken: "ghp_test_token",
			GithubUsername:   "tester",
		},
	}
}

// mustParseRef parses ref or fails the test. Uses a public reference so the
// shim never actually hits the network even if a test misconfigures remoteGet.
func mustParseRef(t *testing.T, ref string) name.Reference {
	t.Helper()
	r, err := name.ParseReference(ref)
	require.NoError(t, err)
	return r
}

func TestPullImage_AnonymousFallback_403(t *testing.T) {
	shim := &pullImageShim{
		results: []pullImageResult{
			{err: &transport.Error{StatusCode: 403}},
			{descriptor: &remote.Descriptor{}},
		},
	}
	installPullImageShim(t, shim)

	ref := mustParseRef(t, "ghcr.io/cloudposse/atmos/tests/fixtures/components/terraform/mock:v0")
	desc, err := pullImage(ghcrConfigWithCreds(), ref)
	require.NoError(t, err)
	assert.NotNil(t, desc)
	assert.Equal(t, 2, shim.calls, "expected one authed attempt followed by one anonymous retry")
}

func TestPullImage_AnonymousFallback_401(t *testing.T) {
	shim := &pullImageShim{
		results: []pullImageResult{
			{err: &transport.Error{StatusCode: 401}},
			{descriptor: &remote.Descriptor{}},
		},
	}
	installPullImageShim(t, shim)

	ref := mustParseRef(t, "ghcr.io/cloudposse/atmos/tests/fixtures/components/terraform/mock:v0")
	desc, err := pullImage(ghcrConfigWithCreds(), ref)
	require.NoError(t, err)
	assert.NotNil(t, desc)
	assert.Equal(t, 2, shim.calls)
}

func TestPullImage_AnonymousFallback_DeniedString(t *testing.T) {
	// Token-endpoint denials surface as plain errors (not *transport.Error) with
	// "DENIED" in the message. The retry path must still kick in.
	shim := &pullImageShim{
		results: []pullImageResult{
			{err: errors.New("GET https://ghcr.io/token: DENIED: insufficient_scope")},
			{descriptor: &remote.Descriptor{}},
		},
	}
	installPullImageShim(t, shim)

	ref := mustParseRef(t, "ghcr.io/cloudposse/atmos/tests/fixtures/components/terraform/mock:v0")
	desc, err := pullImage(ghcrConfigWithCreds(), ref)
	require.NoError(t, err)
	assert.NotNil(t, desc)
	assert.Equal(t, 2, shim.calls)
}

func TestPullImage_NoFallback_DeadlineExceeded(t *testing.T) {
	shim := &pullImageShim{
		results: []pullImageResult{
			{err: context.DeadlineExceeded},
		},
	}
	installPullImageShim(t, shim)

	ref := mustParseRef(t, "ghcr.io/cloudposse/atmos/tests/fixtures/components/terraform/mock:v0")
	_, err := pullImage(ghcrConfigWithCreds(), ref)
	require.Error(t, err)
	assert.Equal(t, 1, shim.calls, "deadline-exceeded must not trigger anonymous retry")
	assert.True(t, errors.Is(err, errUtils.ErrPullImage), "error should wrap ErrPullImage")
	assert.True(t, errors.Is(err, context.DeadlineExceeded), "original cause must be preserved")
}

func TestPullImage_NoFallback_500(t *testing.T) {
	shim := &pullImageShim{
		results: []pullImageResult{
			{err: &transport.Error{StatusCode: 500}},
		},
	}
	installPullImageShim(t, shim)

	ref := mustParseRef(t, "ghcr.io/cloudposse/atmos/tests/fixtures/components/terraform/mock:v0")
	_, err := pullImage(ghcrConfigWithCreds(), ref)
	require.Error(t, err)
	assert.Equal(t, 1, shim.calls, "5xx must not trigger anonymous retry")
	assert.True(t, errors.Is(err, errUtils.ErrPullImage))
}

func TestPullImage_NoFallback_NetOpError(t *testing.T) {
	shim := &pullImageShim{
		results: []pullImageResult{
			{err: &net.OpError{Op: "dial", Err: errors.New("no such host")}},
		},
	}
	installPullImageShim(t, shim)

	ref := mustParseRef(t, "ghcr.io/cloudposse/atmos/tests/fixtures/components/terraform/mock:v0")
	_, err := pullImage(ghcrConfigWithCreds(), ref)
	require.Error(t, err)
	assert.Equal(t, 1, shim.calls, "DNS-style errors must not trigger anonymous retry")
	assert.True(t, errors.Is(err, errUtils.ErrPullImage))
}

func TestPullImage_NoRetry_WhenAlreadyAnonymous(t *testing.T) {
	// Empty config + non-ghcr.io registry yields anonymous auth; a 403 must not
	// retry (would be infinite loop / duplicate attempt).
	shim := &pullImageShim{
		results: []pullImageResult{
			{err: &transport.Error{StatusCode: 403}},
		},
	}
	installPullImageShim(t, shim)

	ref := mustParseRef(t, "registry.example.com/myimage:v1")
	_, err := pullImage(&schema.AtmosConfiguration{}, ref)
	require.Error(t, err)
	assert.Equal(t, 1, shim.calls, "anonymous 403 must not retry anonymously again")
	assert.True(t, errors.Is(err, errUtils.ErrPullImage))
}

func TestPullImage_RichError_ContextAndHints(t *testing.T) {
	// Both attempts fail with 403 — final error must be the rich-builder error.
	shim := &pullImageShim{
		results: []pullImageResult{
			{err: &transport.Error{StatusCode: 403}},
			{err: &transport.Error{StatusCode: 403}},
		},
	}
	installPullImageShim(t, shim)

	ref := mustParseRef(t, "ghcr.io/cloudposse/atmos/tests/fixtures/components/terraform/mock:v0")
	_, err := pullImage(ghcrConfigWithCreds(), ref)
	require.Error(t, err)
	assert.Equal(t, 2, shim.calls, "expected authed + anonymous-retry attempts before failing")

	// Sentinel and cause preserved.
	assert.True(t, errors.Is(err, errUtils.ErrPullImage), "should wrap ErrPullImage sentinel")
	var transportErr *transport.Error
	assert.True(t, errors.As(err, &transportErr), "should preserve *transport.Error cause")
	if transportErr != nil {
		assert.Equal(t, 403, transportErr.StatusCode)
	}

	// All three required hints are attached, each self-contained.
	hints := cockroachErrors.GetAllHints(err)
	joinedHints := strings.Join(hints, "\n")
	assert.Contains(t, joinedHints, "packages: read", "hint about GitHub Actions permissions missing")
	assert.Contains(t, joinedHints, "ATMOS_GITHUB_USERNAME", "hint about username override missing")
	assert.Contains(t, joinedHints, "docker logout ghcr.io", "hint about stale Docker credentials missing")

	// Structured context captured for verbose render / Sentry.
	var contextBlob string
	for _, payload := range cockroachErrors.GetAllSafeDetails(err) {
		for _, d := range payload.SafeDetails {
			contextBlob += d + " "
		}
	}
	assert.Contains(t, contextBlob, "image=")
	assert.Contains(t, contextBlob, "registry=ghcr.io")
	assert.Contains(t, contextBlob, "auth_attempted=")
	assert.Contains(t, contextBlob, "status=403")
}
