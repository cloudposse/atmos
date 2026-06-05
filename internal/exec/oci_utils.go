package exec

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

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
	"github.com/cloudposse/atmos/pkg/schema"
)

var ErrNoLayers = errors.New("the OCI image does not have any layers")

const (
	targetArtifactType = "application/vnd.atmos.component.terraform.v1+tar+gzip" // Target artifact type for Atmos components
)

var defaultOCIFileSystem = filesystem.NewOSFileSystem()

// remoteGet is the package-level indirection over remote.Get used by pullImage.
// Tests override this to simulate registry responses without spinning up an
// httptest server. Production code must not reassign it.
var remoteGet = remote.Get

// processOciImage processes an OCI image and extracts its layers to the specified destination directory.
func processOciImage(atmosConfig *schema.AtmosConfiguration, imageName string, destDir string) error {
	return processOciImageWithFS(atmosConfig, imageName, destDir, defaultOCIFileSystem)
}

// processOciImageWithFS processes an OCI image using a FileSystem implementation.
func processOciImageWithFS(atmosConfig *schema.AtmosConfiguration, imageName string, destDir string, fs filesystem.FileSystem) error {
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

	descriptor, err := pullImage(atmosConfig, ref)
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
		if err := processLayer(layer, i, destDir); err != nil {
			return fmt.Errorf("%w %d: %s", errUtils.ErrProcessLayer, i, err)
		}
	}

	return nil
}

// pullImage pulls an OCI image from the specified reference and returns its descriptor.
// Authentication precedence:
// 1. User's Docker credentials (~/.docker/config.json via DefaultKeychain) - highest precedence
// 2. ATMOS_GITHUB_TOKEN or GITHUB_TOKEN environment variables (for ghcr.io only)
// 3. Anonymous authentication - fallback.
func pullImage(atmosConfig *schema.AtmosConfiguration, ref name.Reference) (*remote.Descriptor, error) {
	var authMethod authn.Authenticator
	var authSource string

	registry := ref.Context().Registry.Name()

	// First, try to use credentials from the user's Docker config.
	// This allows users to authenticate with `docker login` and have those credentials respected.
	keychainAuth, err := authn.DefaultKeychain.Resolve(ref.Context())
	if err != nil {
		log.Debug("DefaultKeychain resolution failed, will try other auth methods", "error", err)
	} else if keychainAuth != authn.Anonymous {
		// User has credentials configured for this registry - highest precedence
		authMethod = keychainAuth
		authSource = "Docker keychain (~/.docker/config.json)"
	}

	// If no user credentials, try environment variable token injection for ghcr.io
	if authMethod == nil && strings.EqualFold(registry, "ghcr.io") {
		authMethod, authSource = getGHCRAuth(atmosConfig)
	}

	// Fall back to anonymous authentication if no credentials found
	if authMethod == nil {
		authMethod = authn.Anonymous
		authSource = "anonymous"
	}

	log.Info("Authenticating to OCI registry", "registry", registry, "method", authSource)

	descriptor, err := remoteGet(ref, remote.WithAuth(authMethod))
	if err == nil {
		return descriptor, nil
	}

	// If credentials were rejected (401/403/DENIED) and we used non-anonymous auth,
	// retry once with anonymous to recover public-image pulls when the configured
	// credentials lack the required scope. Non-auth errors (DNS, TLS, timeouts,
	// 5xx) skip retry — they need a different remediation.
	if authMethod != authn.Anonymous && isOCIAuthRejection(err) {
		anonDescriptor, anonErr := remoteGet(ref, remote.WithAuth(authn.Anonymous))
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

// getGHCRAuth returns authentication credentials for GitHub Container Registry (ghcr.io).
// It tries ATMOS_GITHUB_TOKEN first, then falls back to GITHUB_TOKEN.
// Requires github_username to be configured for authentication.
func getGHCRAuth(atmosConfig *schema.AtmosConfiguration) (authn.Authenticator, string) {
	atmosToken := strings.TrimSpace(atmosConfig.Settings.AtmosGithubToken)
	githubToken := strings.TrimSpace(atmosConfig.Settings.GithubToken)
	githubUsername := strings.TrimSpace(atmosConfig.Settings.GithubUsername)

	var token string
	var tokenSource string

	if atmosToken != "" {
		token = atmosToken
		tokenSource = "ATMOS_GITHUB_TOKEN"
	} else if githubToken != "" {
		token = githubToken
		tokenSource = "GITHUB_TOKEN"
	}

	if token == "" {
		return nil, ""
	}

	// GHCR requires a username; use configured github_username.
	username := githubUsername
	if username == "" {
		// No safe implicit fallback here; return nil to allow caller to choose anon/fail.
		log.Warn("GHCR token found but no username provided; set settings.github_username or ATMOS_GITHUB_USERNAME/GITHUB_ACTOR.")
		return nil, ""
	}

	authMethod := &authn.Basic{
		Username: username,
		Password: token,
	}
	authSource := fmt.Sprintf("environment variable (%s with username %s)", tokenSource, username)

	return authMethod, authSource
}

// processLayer processes a single OCI layer and extracts its contents to the specified destination directory.
func processLayer(layer v1.Layer, index int, destDir string) error {
	layerDesc, err := layer.Digest()
	if err != nil {
		log.Warn("Skipping layer with invalid digest", "index", index, "error", err)
		return nil
	}

	uncompressed, err := layer.Uncompressed()
	if err != nil {
		log.Error("Layer decompression failed", "index", index, "digest", layerDesc, "error", err)
		return errors.Join(errUtils.ErrLayerDecompression, err)
	}
	defer uncompressed.Close()

	if err := extractTarball(uncompressed, destDir); err != nil {
		log.Error("Layer extraction failed", "index", index, "digest", layerDesc, "error", err)
		return errors.Join(errUtils.ErrTarballExtraction, err)
	}

	return nil
}

// checkArtifactType to check and log artifact type mismatches .
func checkArtifactType(descriptor *remote.Descriptor, imageName string) {
	manifest, err := parseOCIManifest(bytes.NewReader(descriptor.Manifest))
	if err != nil {
		log.Error("Failed to parse OCI manifest", "image", imageName, "error", err)
		return
	}
	if manifest.ArtifactType != targetArtifactType {
		// log that don't match the target artifact type
		log.Warn("OCI image does not match the target artifact type", "image", imageName, "artifactType", manifest.ArtifactType)
	}
}

// ParseOCIManifest reads and decodes an OCI manifest from a JSON file.
func parseOCIManifest(manifestBytes io.Reader) (*ocispec.Manifest, error) {
	var manifest ocispec.Manifest
	if err := json.NewDecoder(manifestBytes).Decode(&manifest); err != nil {
		return nil, err
	}

	return &manifest, nil
}
