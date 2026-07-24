package oci

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/google/go-containerregistry/pkg/v1/remote/transport"
	"github.com/google/uuid"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/filesystem"
	log "github.com/cloudposse/atmos/pkg/logger" // Charmbracelet structured logger
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/retry"
	"github.com/cloudposse/atmos/pkg/schema"
)

var ErrNoLayers = errors.New("the OCI image does not have any layers")

const (
	targetArtifactType = "application/vnd.atmos.component.terraform.v1+tar+gzip" // Target artifact type for Atmos components.

	// OCI layer downloads are served from a separate blob host and can suffer
	// short-lived connection resets independently of registry authentication.
	// Keep retries bounded so callers' existing OCI download deadlines remain
	// authoritative.
	ociLayerRetryMaxAttempts  = 3
	ociLayerRetryInitialDelay = time.Second
	ociLayerRetryMaxDelay     = 4 * time.Second
)

// opentofuModulePkgArtifactType is the OCI artifactType OpenTofu's native
// "install modules from OCI registries" feature uses to distribute a module
// (see https://opentofu.org). Its single layer is a ZIP archive
// (zipLayerMediaType), not a tar+gzip stream.
const opentofuModulePkgArtifactType = "application/vnd.opentofu.modulepkg"

// zipLayerMediaType is the OCI layer media type OpenTofu's module-package
// format declares for its ZIP-archive layer. Any other layer media type is
// extracted as a tar stream (extractTarball), preserving existing behavior
// for Atmos's own artifacts and generic OCI images.
const zipLayerMediaType = "archive/zip"

var defaultFileSystem = filesystem.NewOSFileSystem()

// remoteGet is the package-level indirection over remote.Get used by pullImage.
// Tests override this to simulate registry responses without spinning up an
// httptest server. Production code must not reassign it.
var remoteGet = remote.Get

// ResolvedImage is the immutable registry identity selected for an OCI source.
// Digest is the descriptor digest for the selected manifest; it is suitable for
// locks and SBOM provenance, unlike the mutable declared tag/reference.
type ResolvedImage struct {
	Reference string
	Digest    string
	MediaType string
}

// ResolveImage authenticates and resolves an OCI reference without extracting
// layers. It is the public provenance boundary for OCI consumers.
func ResolveImage(ctx context.Context, atmosConfig *schema.AtmosConfiguration, imageName string) (*ResolvedImage, error) {
	defer perf.Track(atmosConfig, "oci.ResolveImage")()

	ref, err := name.ParseReference(imageName)
	if err != nil {
		return nil, errors.Join(errUtils.ErrInvalidImageReference, err)
	}
	descriptor, err := pullImage(ctx, atmosConfig, ref)
	if err != nil {
		return nil, err
	}
	return &ResolvedImage{Reference: ref.Name(), Digest: descriptor.Digest.String(), MediaType: string(descriptor.MediaType)}, nil
}

// ProcessImage pulls an OCI image and extracts its layers to the specified
// destination directory. The context bounds the pull (registry auth plus
// manifest/layer fetch) -- callers should pass one with a deadline, matching
// the timeout the go-getter download path already enforces.
func ProcessImage(ctx context.Context, atmosConfig *schema.AtmosConfiguration, imageName string, destDir string) error {
	defer perf.Track(atmosConfig, "oci.ProcessImage")()

	return processImageWithFS(ctx, atmosConfig, imageName, destDir, defaultFileSystem)
}

// processImageWithFS processes an OCI image using a FileSystem implementation.
func processImageWithFS(ctx context.Context, atmosConfig *schema.AtmosConfiguration, imageName string, destDir string, fs filesystem.FileSystem) error {
	tempDir, err := fs.MkdirTemp("", uuid.New().String())
	if err != nil {
		return errors.Join(errUtils.ErrCreateTempDirectory, err)
	}
	defer func() {
		if err := fs.RemoveAll(tempDir); err != nil {
			log.Debug("Failed to remove temp directory", "path", tempDir, "error", err)
		}
	}()

	ref, err := name.ParseReference(imageName)
	if err != nil {
		log.Error("Failed to parse OCI image reference", "image", imageName, "error", err)
		return errors.Join(errUtils.ErrInvalidImageReference, err)
	}

	descriptor, err := pullImage(ctx, atmosConfig, ref)
	if err != nil {
		// pullImage already wraps the error with errUtils.ErrPullImage via the
		// builder, so errors.Is(err, ErrPullImage) is true. Returning it directly
		// preserves the rich hints/context without double-wrapping the sentinel.
		return err
	}

	img, err := descriptor.Image()
	if err != nil {
		log.Error("Failed to get image descriptor", "image", imageName, "error", err)
		return fmt.Errorf("%w '%s': %s", errUtils.ErrGetImageDescriptor, imageName, err)
	}

	checkArtifactType(descriptor, imageName)

	layers, err := img.Layers()
	if err != nil {
		log.Error("Failed to retrieve layers from OCI image", "image", imageName, "error", err)
		return errors.Join(errUtils.ErrGetImageLayers, err)
	}

	if len(layers) == 0 {
		log.Warn("OCI image has no layers", "image", imageName)
		return ErrNoLayers
	}

	for i, layer := range layers {
		if err := processLayerWithRetry(ctx, layer, i, destDir, defaultOCILayerRetryConfig()); err != nil {
			return errors.Join(
				errUtils.ErrProcessLayer,
				fmt.Errorf("layer %d: %w", i, err),
			)
		}
	}

	return nil
}

// pullImage pulls an OCI image from the specified reference and returns its descriptor.
// Authentication precedence:
// 1. User's Docker credentials (~/.docker/config.json via DefaultKeychain) - highest precedence
// 2. ATMOS_GITHUB_TOKEN or GITHUB_TOKEN environment variables (for ghcr.io only)
// 3. Anonymous authentication - fallback.
func pullImage(ctx context.Context, atmosConfig *schema.AtmosConfiguration, ref name.Reference) (*remote.Descriptor, error) {
	var authMethod authn.Authenticator
	var authSource string

	registry := ref.Context().Registry.Name()

	// First, try to use credentials from the user's Docker config.
	// This allows users to authenticate with `docker login` and have those credentials respected.
	keychainAuth, err := authn.DefaultKeychain.Resolve(ref.Context())
	if err != nil {
		log.Debug("DefaultKeychain resolution failed, will try other auth methods", "error", err)
	} else if keychainAuth != authn.Anonymous {
		// User has credentials configured for this registry - highest precedence.
		authMethod = keychainAuth
		authSource = "Docker keychain (~/.docker/config.json)"
	}

	// If no user credentials, try environment variable token injection for ghcr.io.
	if authMethod == nil && strings.EqualFold(registry, "ghcr.io") {
		authMethod, authSource = GHCRAuth(atmosConfig)
	}

	// Fall back to anonymous authentication if no credentials found.
	if authMethod == nil {
		authMethod = authn.Anonymous
		authSource = "anonymous"
	}

	log.Info("Authenticating to OCI registry", "registry", registry, "method", authSource)

	descriptor, err := remoteGet(ref, remote.WithAuth(authMethod), remote.WithContext(ctx))
	if err == nil {
		return descriptor, nil
	}

	// If credentials were rejected (401/403/DENIED) and we used non-anonymous auth,
	// retry once with anonymous to recover public-image pulls when the configured
	// credentials lack the required scope. Non-auth errors (DNS, TLS, timeouts,
	// 5xx) skip retry — they need a different remediation.
	if authMethod != authn.Anonymous && isOCIAuthRejection(err) {
		anonDescriptor, anonErr := remoteGet(ref, remote.WithAuth(authn.Anonymous), remote.WithContext(ctx))
		if anonErr == nil {
			log.Warn("OCI auth rejected, succeeded with anonymous fallback",
				"registry", registry, "auth_attempted", authSource)
			return anonDescriptor, nil
		}
		// Anonymous also failed; fall through and report the original authed
		// error, which carries the more diagnostic status/body for scope problems.
	}

	log.Error("Failed to pull OCI image", "image", ref.Name(), "registry", registry, "auth", authSource, "error", err)
	return nil, buildPullImageError(err, ref, registry, authSource)
}

// buildPullImageError wraps a remote.Get failure with the project's enriched
// error builder: sentinel ErrPullImage, structured context (image, registry,
// auth_attempted, status when available), and three self-contained hints
// (each renders as its own lightbulb line).
func buildPullImageError(cause error, ref name.Reference, registry, authSource string) error {
	builder := errUtils.Build(errUtils.ErrPullImage).
		WithCause(cause).
		WithContext("image", ref.Name()).
		WithContext("registry", registry).
		WithContext("auth_attempted", authSource).
		WithHint("If pulling from ghcr.io in GitHub Actions, grant the workflow 'packages: read' permission.").
		WithHint("Set ATMOS_GITHUB_USERNAME to override the default 'GITHUB_ACTOR' identity used for ghcr.io auth.").
		WithHint("For public images, remove stale credentials for this registry from ~/.docker/config.json (or run 'docker logout ghcr.io').")

	var transportErr *transport.Error
	if errors.As(cause, &transportErr) {
		// Stringify the status here: the ErrorBuilder's SafeDetails formatter
		// always formats context values with %s, so passing an int yields a
		// malformed "status=%!s(int=403)" payload.
		builder = builder.WithContext("status", strconv.Itoa(transportErr.StatusCode))
	}

	return builder.Err()
}

// isOCIAuthRejection reports whether err signals a registry auth rejection that
// is safe to retry anonymously: HTTP 401/403, or a token-endpoint error whose
// message contains "DENIED" (which is not always a *transport.Error).
func isOCIAuthRejection(err error) bool {
	if err == nil {
		return false
	}
	var transportErr *transport.Error
	if errors.As(err, &transportErr) {
		if transportErr.StatusCode == http.StatusUnauthorized ||
			transportErr.StatusCode == http.StatusForbidden {
			return true
		}
	}
	return strings.Contains(err.Error(), "DENIED")
}

func processLayer(layer v1.Layer, index int, destDir string) error {
	layerDesc, err := layer.Digest()
	if err != nil {
		log.Warn("Skipping layer with invalid digest", "index", index, "error", err)
		return nil
	}

	uncompressed, err := layer.Uncompressed()
	if err != nil {
		return errors.Join(errUtils.ErrLayerDecompression, err)
	}
	defer uncompressed.Close()

	extract := extractTarball
	if mediaType, mtErr := layer.MediaType(); mtErr == nil && mediaType == zipLayerMediaType {
		extract = extractZip
	}

	if err := extract(uncompressed, destDir); err != nil {
		log.Error("Layer extraction failed", "index", index, "digest", layerDesc, "error", err)
		return errors.Join(errUtils.ErrLayerExtraction, err)
	}

	return nil
}

// processLayerWithRetry retries only transient failures while opening an OCI
// layer stream. Archive extraction failures are deliberately not retried: they
// may reflect invalid image content and can leave partial files in destDir.
func processLayerWithRetry(ctx context.Context, layer v1.Layer, index int, destDir string, retryConfig *schema.RetryConfig) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	attempts := 0
	err := retry.WithPredicate(ctx, retryConfig, func() error {
		attempts++
		err := processLayer(layer, index, destDir)
		if err != nil && ctx.Err() == nil && isRetryableOCILayerError(err) && attempts < ociLayerRetryMaxAttempts {
			// Do not include the underlying error here: OCI blob errors can contain
			// signed URLs. The layer index is enough to correlate the retry with the
			// final error if all attempts fail.
			log.Warn("Retrying OCI layer download after transient failure", "index", index, "attempt", attempts)
		}
		return err
	}, func(err error) bool {
		return ctx.Err() == nil && isRetryableOCILayerError(err)
	})
	if err != nil {
		log.Error("OCI layer processing failed", "index", index, "retryable", isRetryableOCILayerError(err))
	}
	return err
}

func defaultOCILayerRetryConfig() *schema.RetryConfig {
	maxAttempts := ociLayerRetryMaxAttempts
	initialDelay := ociLayerRetryInitialDelay
	maxDelay := ociLayerRetryMaxDelay
	return &schema.RetryConfig{
		MaxAttempts:     &maxAttempts,
		BackoffStrategy: schema.BackoffExponential,
		InitialDelay:    &initialDelay,
		MaxDelay:        &maxDelay,
	}
}

// isRetryableOCILayerError excludes registry status/authentication failures
// and malformed archives. The former require different credentials while the
// latter will not be fixed by downloading the same bytes again.
func isRetryableOCILayerError(err error) bool {
	if err == nil || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	if !errors.Is(err, errUtils.ErrLayerDecompression) {
		return false
	}
	if errors.Is(err, io.ErrUnexpectedEOF) || errors.Is(err, net.ErrClosed) {
		return true
	}

	var networkErr net.Error
	return errors.As(err, &networkErr)
}

// checkArtifactType checks and logs artifact type mismatches.
func checkArtifactType(descriptor *remote.Descriptor, imageName string) {
	manifest, err := parseOCIManifest(bytes.NewReader(descriptor.Manifest))
	if err != nil {
		log.Error("Failed to parse OCI manifest", "image", imageName, "error", err)
		return
	}
	switch manifest.ArtifactType {
	case targetArtifactType, opentofuModulePkgArtifactType:
		// Recognized and supported artifact type; nothing to warn about.
	default:
		log.Warn("OCI image does not match a recognized artifact type", "image", imageName, "artifactType", manifest.ArtifactType)
	}
}

// parseOCIManifest reads and decodes an OCI manifest from a JSON reader.
func parseOCIManifest(manifestBytes io.Reader) (*ocispec.Manifest, error) {
	var manifest ocispec.Manifest
	if err := json.NewDecoder(manifestBytes).Decode(&manifest); err != nil {
		return nil, err
	}

	return &manifest, nil
}
