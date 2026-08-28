package oci

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

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
	"github.com/cloudposse/atmos/pkg/oci/ocitest"
	"github.com/cloudposse/atmos/pkg/schema"
)

// MockLayer implements v1.Layer for testing.
type MockLayer struct {
	digestVal         v1.Hash
	sizeVal           int64
	uncompressedErr   error
	compressedErr     error
	mediaTypeVal      types.MediaType
	uncompressedData  []byte
	uncompressedErrs  []error
	uncompressedCalls int
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
	call := m.uncompressedCalls
	m.uncompressedCalls++
	if call < len(m.uncompressedErrs) && m.uncompressedErrs[call] != nil {
		return nil, m.uncompressedErrs[call]
	}
	if m.uncompressedErr != nil {
		return nil, m.uncompressedErr
	}
	return io.NopCloser(bytes.NewReader(m.uncompressedData)), nil
}

func (m *MockLayer) Size() (int64, error) {
	return m.sizeVal, nil
}

func (m *MockLayer) MediaType() (types.MediaType, error) {
	if m.mediaTypeVal != "" {
		return m.mediaTypeVal, nil
	}
	return types.DockerLayer, nil
}

// TestProcessOciImage_InvalidReference tests error handling for invalid image references.
func TestProcessOciImage_InvalidReference(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{}

	// Test with invalid image reference.
	err := ProcessImage(context.Background(), atmosConfig, "invalid::image//name", "/tmp/dest")

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

func TestProcessLayerWithRetry_TransientErrorEventuallySucceeds(t *testing.T) {
	dest := t.TempDir()
	tarBuf := writeTestTar(t, map[string]string{"main.tf": "# recovered download\n"})
	mockLayer := &MockLayer{
		digestVal:        v1.Hash{Algorithm: "sha256", Hex: "retry123"},
		uncompressedErrs: []error{&net.OpError{Op: "read", Err: errors.New("connection reset")}},
		uncompressedData: tarBuf.Bytes(),
	}

	err := processLayerWithRetry(context.Background(), mockLayer, 0, dest, testOCILayerRetryConfig())
	require.NoError(t, err)
	assert.Equal(t, 2, mockLayer.uncompressedCalls)

	content, err := os.ReadFile(filepath.Join(dest, "main.tf"))
	require.NoError(t, err)
	assert.Equal(t, "# recovered download\n", string(content))
}

func TestProcessLayerWithRetry_DoesNotRetryNonNetworkFailure(t *testing.T) {
	mockLayer := &MockLayer{
		digestVal:       v1.Hash{Algorithm: "sha256", Hex: "invalid123"},
		uncompressedErr: errors.New("invalid gzip header"),
	}

	err := processLayerWithRetry(context.Background(), mockLayer, 0, t.TempDir(), testOCILayerRetryConfig())
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrLayerDecompression)
	assert.Equal(t, 1, mockLayer.uncompressedCalls)
}

func TestProcessLayerWithRetry_DoesNotRetryAuthenticationFailure(t *testing.T) {
	mockLayer := &MockLayer{
		digestVal:       v1.Hash{Algorithm: "sha256", Hex: "auth123"},
		uncompressedErr: &transport.Error{StatusCode: 403},
	}

	err := processLayerWithRetry(context.Background(), mockLayer, 0, t.TempDir(), testOCILayerRetryConfig())
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrLayerDecompression)
	assert.Equal(t, 1, mockLayer.uncompressedCalls)
}

func TestProcessLayerWithRetry_DoesNotRetryExpiredContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	mockLayer := &MockLayer{
		digestVal:       v1.Hash{Algorithm: "sha256", Hex: "cancelled123"},
		uncompressedErr: &net.OpError{Op: "read", Err: errors.New("connection reset")},
	}

	err := processLayerWithRetry(ctx, mockLayer, 0, t.TempDir(), testOCILayerRetryConfig())
	require.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled)
	assert.Equal(t, 0, mockLayer.uncompressedCalls)
}

func testOCILayerRetryConfig() *schema.RetryConfig {
	maxAttempts := ociLayerRetryMaxAttempts
	initialDelay := time.Duration(0)
	return &schema.RetryConfig{
		MaxAttempts:     &maxAttempts,
		BackoffStrategy: schema.BackoffExponential,
		InitialDelay:    &initialDelay,
	}
}

// TestProcessLayer_ZipMediaType tests that a layer declaring the OpenTofu
// module-package media type ("archive/zip") is extracted as a zip archive
// instead of a tar stream. Regression test for
// https://github.com/cloudposse/atmos/issues/2716 following up: OpenTofu's
// native OCI module-package format failed with "archive/tar: invalid tar
// header" because processLayer always assumed a tar+gzip layer.
func TestProcessLayer_ZipMediaType(t *testing.T) {
	dest := t.TempDir()
	zipBuf := writeTestZip(t, map[string]string{"main.tf": "# from zip layer\n"})

	mockLayer := &MockLayer{
		digestVal:        v1.Hash{Algorithm: "sha256", Hex: "zip123"},
		mediaTypeVal:     zipLayerMediaType,
		uncompressedData: zipBuf.Bytes(),
	}

	err := processLayer(mockLayer, 0, dest)
	require.NoError(t, err)

	content, err := os.ReadFile(filepath.Join(dest, "main.tf"))
	require.NoError(t, err)
	assert.Contains(t, string(content), "from zip layer")
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
	err := processImageWithFS(context.Background(), atmosConfig, "test/image:latest", "/tmp/dest", mockFS)

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

// ghcrConfigWithCreds returns a config that causes GHCRAuth to resolve a
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
	desc, err := pullImage(context.Background(), ghcrConfigWithCreds(), ref)
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
	desc, err := pullImage(context.Background(), ghcrConfigWithCreds(), ref)
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
	desc, err := pullImage(context.Background(), ghcrConfigWithCreds(), ref)
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
	_, err := pullImage(context.Background(), ghcrConfigWithCreds(), ref)
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
	_, err := pullImage(context.Background(), ghcrConfigWithCreds(), ref)
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
	_, err := pullImage(context.Background(), ghcrConfigWithCreds(), ref)
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
	_, err := pullImage(context.Background(), &schema.AtmosConfiguration{}, ref)
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
	_, err := pullImage(context.Background(), ghcrConfigWithCreds(), ref)
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

// TestResolveImage_Success resolves a real (in-process) OCI manifest and
// asserts the returned identity: a resolvable reference name and a
// content-addressable "sha256:..." digest suitable for locks/SBOM.
func TestResolveImage_Success(t *testing.T) {
	imageRef := ocitest.NewRegistry(t, "resolve/success:v1", map[string]string{
		"main.tf": "# resolve target\n",
	})

	resolved, err := ResolveImage(context.Background(), &schema.AtmosConfiguration{}, imageRef)
	require.NoError(t, err)
	require.NotNil(t, resolved)
	assert.Equal(t, imageRef, resolved.Reference)
	assert.Regexp(t, `^sha256:[a-f0-9]{64}$`, resolved.Digest)
	assert.NotEmpty(t, resolved.MediaType)
}

// TestResolveImage_InvalidReference asserts a malformed image reference is
// rejected before any network call, wrapped in ErrInvalidImageReference.
func TestResolveImage_InvalidReference(t *testing.T) {
	resolved, err := ResolveImage(context.Background(), &schema.AtmosConfiguration{}, "invalid::image//name")
	require.Error(t, err)
	assert.Nil(t, resolved)
	assert.True(t, errors.Is(err, errUtils.ErrInvalidImageReference))
}

// TestResolveImage_PullError asserts a registry-level failure during
// resolution surfaces as an error (wrapping ErrPullImage) with a nil result,
// rather than a partially populated ResolvedImage.
func TestResolveImage_PullError(t *testing.T) {
	shim := &pullImageShim{
		results: []pullImageResult{
			{err: &transport.Error{StatusCode: 500}},
		},
	}
	installPullImageShim(t, shim)

	ref := "registry.example.com/myimage:v1"
	resolved, err := ResolveImage(context.Background(), &schema.AtmosConfiguration{}, ref)
	require.Error(t, err)
	assert.Nil(t, resolved)
	assert.True(t, errors.Is(err, errUtils.ErrPullImage))
}

// TestProcessImageWithFS_Success exercises the full pull->extract success
// path against a real (in-process) registry: manifest resolution, artifact
// type check, layer retrieval, and extraction all succeed and the layer's
// files land in destDir.
func TestProcessImageWithFS_Success(t *testing.T) {
	imageRef := ocitest.NewRegistry(t, "process/success:v1", map[string]string{
		"main.tf": "# process target\n",
	})

	dest := t.TempDir()
	err := processImageWithFS(context.Background(), &schema.AtmosConfiguration{}, imageRef, dest, defaultFileSystem)
	require.NoError(t, err)

	content, err := os.ReadFile(filepath.Join(dest, "main.tf"))
	require.NoError(t, err)
	assert.Equal(t, "# process target\n", string(content))
}

// TestProcessImageWithFS_NoLayers asserts a manifest that resolves but has
// zero layers fails with the dedicated ErrNoLayers sentinel, distinct from a
// network/auth failure.
func TestProcessImageWithFS_NoLayers(t *testing.T) {
	imageRef := ocitest.NewEmptyRegistry(t, "process/empty:v1")

	err := processImageWithFS(context.Background(), &schema.AtmosConfiguration{}, imageRef, t.TempDir(), defaultFileSystem)
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrNoLayers))
}

// TestProcessImageWithFS_LayerProcessingFailure asserts that when a layer
// fails to process (non-retryable decompression failure), the loop stops and
// returns an error wrapping both ErrProcessLayer and the underlying cause,
// with the layer index in the message.
func TestProcessImageWithFS_LayerProcessingFailure(t *testing.T) {
	imageRef := ocitest.NewBrokenLayerRegistry(t, "process/broken:v1")

	err := processImageWithFS(context.Background(), &schema.AtmosConfiguration{}, imageRef, t.TempDir(), defaultFileSystem)
	require.Error(t, err)
	assert.True(t, errors.Is(err, errUtils.ErrProcessLayer))
	assert.True(t, errors.Is(err, errUtils.ErrLayerDecompression))
	assert.Contains(t, err.Error(), "layer 0")
}

// TestDefaultOCILayerRetryConfig asserts the production retry configuration
// used by processImageWithFS has the documented bounded attempts and delays,
// so a regression here (e.g. accidentally unbounded retries) is caught.
func TestDefaultOCILayerRetryConfig(t *testing.T) {
	cfg := defaultOCILayerRetryConfig()
	require.NotNil(t, cfg)
	require.NotNil(t, cfg.MaxAttempts)
	assert.Equal(t, ociLayerRetryMaxAttempts, *cfg.MaxAttempts)
	require.NotNil(t, cfg.InitialDelay)
	assert.Equal(t, ociLayerRetryInitialDelay, *cfg.InitialDelay)
	require.NotNil(t, cfg.MaxDelay)
	assert.Equal(t, ociLayerRetryMaxDelay, *cfg.MaxDelay)
	assert.Equal(t, schema.BackoffExponential, cfg.BackoffStrategy)
}

// TestIsRetryableOCILayerError covers the pure-decision branches of
// isRetryableOCILayerError directly: nil/context errors are never retryable,
// errors unrelated to layer decompression are never retryable, and a
// decompression failure wrapping a definitively transient cause (unexpected
// EOF, closed connection) is retryable even when it doesn't implement
// net.Error.
func TestIsRetryableOCILayerError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{name: "nil error", err: nil, want: false},
		{name: "context canceled", err: context.Canceled, want: false},
		{name: "context deadline exceeded", err: context.DeadlineExceeded, want: false},
		{name: "unrelated error is not retryable", err: errors.New("some other failure"), want: false},
		{
			name: "decompression failure wrapping unexpected EOF is retryable",
			err:  errors.Join(errUtils.ErrLayerDecompression, io.ErrUnexpectedEOF),
			want: true,
		},
		{
			name: "decompression failure wrapping closed network connection is retryable",
			err:  errors.Join(errUtils.ErrLayerDecompression, net.ErrClosed),
			want: true,
		},
		{
			name: "decompression failure wrapping non-network cause is not retryable",
			err:  errors.Join(errUtils.ErrLayerDecompression, errors.New("invalid gzip header")),
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, isRetryableOCILayerError(tt.err))
		})
	}
}
