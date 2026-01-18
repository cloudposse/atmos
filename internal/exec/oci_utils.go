package exec

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	log "github.com/charmbracelet/log" // Charmbracelet structured logger
	"github.com/spf13/viper"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/google/uuid"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/filesystem"
	log "github.com/cloudposse/atmos/pkg/logger" // Charmbracelet structured logger
	"github.com/cloudposse/atmos/pkg/schema"
)

var (
	ErrNoLayers                   = errors.New("the OCI image does not have any layers")
	errIllegalZipFilePath         = errors.New("illegal file path in ZIP")
	errFailedToCreateDirectory    = errors.New("failed to create directory")
	errFailedToOpenZipFile        = errors.New("failed to open file in ZIP")
	errFailedToCreateFile         = errors.New("failed to create file")
	errFailedToCopyFile           = errors.New("failed to copy file")
	errFailedToReadZipData        = errors.New("failed to read ZIP data")
	errFailedToCreateZipReader    = errors.New("failed to create ZIP reader")
	errFailedToResolveDestination = errors.New("failed to resolve destination directory")
	errZipSizeExceeded            = errors.New("ZIP file size exceeds maximum allowed size")
	errNoAuthenticationFound      = errors.New("no authentication found")
)

const (
	targetArtifactType = "application/vnd.atmos.component.terraform.v1+tar+gzip" // Target artifact type for Atmos components
	// Additional supported artifact types
	opentofuArtifactType  = "application/vnd.opentofu.modulepkg"           // OpenTofu module package
	terraformArtifactType = "application/vnd.terraform.module.v1+tar+gzip" // Terraform module package
	logFieldRegistry      = "registry"
	logFieldIndex         = "index"
	logFieldDigest        = "digest"
	logFieldError         = "error"
	defaultDirMode        = 0o755             // Default directory permissions (rwxr-xr-x)
	maxZipSize            = 100 * 1024 * 1024 // Maximum ZIP file size: 100MB (prevents decompression bomb)
)

// bindEnv binds environment variables to Viper with fallback support.
func bindEnv(v *viper.Viper, key string, envVars ...string) {
	if err := v.BindEnv(append([]string{key}, envVars...)...); err != nil {
		log.Debug("Failed to bind environment variable", "key", key, "envVars", envVars, logFieldError, err)
	}
}
)

var defaultOCIFileSystem = filesystem.NewOSFileSystem()

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
		log.Error("Failed to parse OCI image reference", "image", imageName, logFieldError, err)
		return fmt.Errorf("invalid image reference: %w", err)
	}

	descriptor, err := pullImage(ref, atmosConfig)
	if err != nil {
		return errors.Join(errUtils.ErrPullImage, err)
	}

	img, err := descriptor.Image()
	if err != nil {
		log.Error("Failed to get image descriptor", "image", imageName, logFieldError, err)
		return fmt.Errorf("cannot get a descriptor for the OCI image '%s': %w", imageName, err)
	}

	checkArtifactType(descriptor, imageName)

	layers, err := img.Layers()
	if err != nil {
		log.Error("Failed to retrieve layers from OCI image", "image", imageName, logFieldError, err)
		return fmt.Errorf("failed to get image layers: %w", err)
	}

	if len(layers) == 0 {
		log.Warn("OCI image has no layers", "image", imageName)
		return ErrNoLayers
	}

	successfulLayers := 0
	for i, layer := range layers {
		if err := processLayer(layer, i, destDir); err != nil {
			log.Warn("Failed to process layer", logFieldIndex, i, logFieldError, err)
			continue // Continue with other layers instead of failing completely
		}
		successfulLayers++
	}

	// Check if any files were actually extracted
	files, err := os.ReadDir(destDir)
	switch {
	case err != nil:
		log.Warn("Could not read destination directory", "dir", destDir, logFieldError, err)
	case len(files) == 0:
		log.Warn("No files were extracted to destination directory", "dir", destDir, "totalLayers", len(layers), "successfulLayers", successfulLayers)
	default:
		log.Debug("Successfully extracted files", "dir", destDir, "fileCount", len(files), "totalLayers", len(layers), "successfulLayers", successfulLayers)
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

	log.Debug("Authenticating to OCI registry", "registry", registry, "method", authSource)

	descriptor, err := remote.Get(ref, remote.WithAuth(authMethod))
	if err != nil {
		log.Error("Failed to pull OCI image", "image", ref.Name(), "registry", registry, "auth", authSource, "error", err)
		return nil, fmt.Errorf("failed to pull image '%s': %w", ref.Name(), err)
	}

	return descriptor, nil
}

// tryGitHubAuth attempts GitHub Container Registry authentication.
func tryGitHubAuth(registry string, atmosConfig *schema.AtmosConfiguration) (authn.Authenticator, error) {
	if !strings.EqualFold(registry, "ghcr.io") {
		return nil, fmt.Errorf("not a GitHub registry")
	}

	auth, err := getGitHubAuth(registry, atmosConfig)
	if err == nil {
		log.Debug("Using GitHub authentication", logFieldRegistry, registry)
	}
	return auth, err
}

// tryDockerAuth attempts Docker credential helper authentication.
func tryDockerAuth(registry string, atmosConfig *schema.AtmosConfiguration) (authn.Authenticator, error) {
	auth, err := getDockerAuth(registry, atmosConfig)
	if err == nil {
		log.Debug("Using Docker config authentication", logFieldRegistry, registry)
	}
	return auth, err
}

// tryEnvironmentAuth attempts authentication using environment variables.
func tryEnvironmentAuth(registry string) (authn.Authenticator, error) {
	v := viper.New()

	// Normalize registry name by replacing dots and hyphens with underscores for valid env var names
	registryEnvName := strings.ToUpper(strings.NewReplacer(".", "_", "-", "_").Replace(registry))
	usernameKey := fmt.Sprintf("%s_username", registryEnvName)
	passwordKey := fmt.Sprintf("%s_password", registryEnvName)

	// Bind the registry-specific environment variables
	bindEnv(v, usernameKey,
		fmt.Sprintf("%s_USERNAME", registryEnvName),
		fmt.Sprintf("ATMOS_%s_USERNAME", registryEnvName),
	)
	bindEnv(v, passwordKey,
		fmt.Sprintf("%s_PASSWORD", registryEnvName),
		fmt.Sprintf("ATMOS_%s_PASSWORD", registryEnvName),
	)

	username := v.GetString(usernameKey)
	password := v.GetString(passwordKey)

	if username == "" || password == "" {
		return nil, fmt.Errorf("no environment credentials found")
	}

	log.Debug("Using environment variable authentication", logFieldRegistry, registry)
	return &authn.Basic{
		Username: username,
		Password: password,
	}, nil
}

// tryECRAuth attempts AWS ECR authentication.
func tryECRAuth(registry string) (authn.Authenticator, error) {
	if !strings.Contains(registry, "dkr.ecr") || !strings.Contains(registry, "amazonaws.com") {
		return nil, fmt.Errorf("not an ECR registry")
	}

	auth, err := getECRAuth(registry)
	if err == nil {
		log.Debug("Using AWS ECR authentication", logFieldRegistry, registry)
	}
	return auth, err
}

// tryACRAuth attempts Azure Container Registry authentication.
func tryACRAuth(registry string, atmosConfig *schema.AtmosConfiguration) (authn.Authenticator, error) {
	if !strings.Contains(registry, "azurecr.io") {
		return nil, fmt.Errorf("not an Azure registry")
	}

	auth, err := getACRAuth(registry, atmosConfig)
	if err == nil {
		log.Debug("Using Azure ACR authentication", logFieldRegistry, registry)
	}
	return auth, err
}

// tryGCRAuth attempts Google Container Registry authentication.
func tryGCRAuth(registry string) (authn.Authenticator, error) {
	if !strings.Contains(registry, "gcr.io") && !strings.Contains(registry, "pkg.dev") {
		return nil, fmt.Errorf("not a Google registry")
	}

	auth, err := getGCRAuth(registry)
	if err == nil {
		log.Debug("Using Google GCR authentication", logFieldRegistry, registry)
	}
	return auth, err
}

// getRegistryAuth attempts to find authentication credentials for the given registry.
// It checks multiple sources in order of preference:
// 1. GitHub Container Registry (ghcr.io) with GITHUB_TOKEN.
// 2. Docker credential helpers (from ~/.docker/config.json).
// 3. Environment variables for specific registries.
// 4. AWS ECR authentication (if AWS credentials are available).
func getRegistryAuth(registry string, atmosConfig *schema.AtmosConfiguration) (authn.Authenticator, error) {
	// Try authentication methods in order of preference
	authMethods := []func() (authn.Authenticator, error){
		func() (authn.Authenticator, error) { return tryGitHubAuth(registry, atmosConfig) },
		func() (authn.Authenticator, error) { return tryDockerAuth(registry, atmosConfig) },
		func() (authn.Authenticator, error) { return tryEnvironmentAuth(registry) },
		func() (authn.Authenticator, error) { return tryECRAuth(registry) },
		func() (authn.Authenticator, error) { return tryACRAuth(registry, atmosConfig) },
		func() (authn.Authenticator, error) { return tryGCRAuth(registry) },
	}

	for _, method := range authMethods {
		if auth, err := method(); err == nil {
			return auth, nil
		}
	}

	return nil, fmt.Errorf("%w %s", errNoAuthenticationFound, registry)
}

// handleExtractionError handles extraction errors by attempting alternative extraction methods.
func handleExtractionError(extractionErr error, layer v1.Layer, uncompressed io.ReadCloser, destDir string, index int, layerDesc v1.Hash) error {
	log.Error("Layer extraction failed", logFieldIndex, index, logFieldDigest, layerDesc, logFieldError, extractionErr)

	// Try alternative extraction methods for different formats
	log.Debug("Attempting alternative extraction methods", logFieldIndex, index, logFieldDigest, layerDesc)

	// Reset the uncompressed reader
	if uncompressed != nil {
		_ = uncompressed.Close()
	}
	uncompressed, err := layer.Uncompressed()
	if err != nil {
		log.Error("Failed to reset uncompressed reader", logFieldIndex, index, logFieldDigest, layerDesc, logFieldError, err)
		return fmt.Errorf("layer decompression error: %w", err)
	}
	defer func() {
		if uncompressed != nil {
			_ = uncompressed.Close()
		}
	}()

	// Try to extract as raw data first
	if err := extractRawData(uncompressed, destDir, index); err != nil {
		log.Error("Raw data extraction also failed", logFieldIndex, index, logFieldDigest, layerDesc, logFieldError, err)

		// If this is the first layer and it fails, it might be metadata
		if index == 0 {
			log.Warn("First layer extraction failed, this might be metadata. Skipping layer.", logFieldIndex, index, logFieldDigest, layerDesc)
			return nil // Skip this layer instead of failing
		}

		return fmt.Errorf("all extraction methods failed: %w", err)
	}

	log.Debug("Successfully extracted layer using alternative method", logFieldIndex, index, logFieldDigest, layerDesc)
	return nil
}

// processLayer processes a single OCI layer and extracts its contents to the specified destination directory.
func processLayer(layer v1.Layer, index int, destDir string) error {
	layerDesc, err := layer.Digest()
	if err != nil {
		log.Warn("Skipping layer with invalid digest", logFieldIndex, index, logFieldError, err)
		return nil
	}

	// Get layer size for debugging
	size, err := layer.Size()
	if err != nil {
		log.Warn("Could not get layer size", logFieldIndex, index, logFieldDigest, layerDesc, logFieldError, err)
	} else {
		log.Debug("Processing layer", logFieldIndex, index, logFieldDigest, layerDesc, "size", size)
	}

	// Get layer media type for debugging
	mediaType, err := layer.MediaType()
	if err != nil {
		log.Warn("Could not get layer media type", logFieldIndex, index, logFieldDigest, layerDesc, logFieldError, err)
	} else {
		log.Debug("Layer media type", logFieldIndex, index, logFieldDigest, layerDesc, "mediaType", mediaType)
	}

	uncompressed, err := layer.Uncompressed()
	if err != nil {
		log.Error("Layer decompression failed", logFieldIndex, index, logFieldDigest, layerDesc, logFieldError, err)
		return fmt.Errorf("layer decompression error: %w", err)
	}
	defer func() {
		if uncompressed != nil {
			_ = uncompressed.Close()
		}
	}()

	// Try to extract the layer based on media type
	var extractionErr error

	// Check if it's a ZIP file
	mediaTypeStr := string(mediaType)
	if strings.Contains(mediaTypeStr, "zip") {
		log.Debug("Detected ZIP layer, extracting as ZIP", logFieldIndex, index, logFieldDigest, layerDesc, "mediaType", mediaTypeStr)
		extractionErr = extractZipFile(uncompressed, destDir)
	} else {
		// Default to tar extraction
		log.Debug("Extracting as TAR", logFieldIndex, index, logFieldDigest, layerDesc, "mediaType", mediaTypeStr)
		extractionErr = extractTarball(uncompressed, destDir)
	}

	if extractionErr != nil {
		return handleExtractionError(extractionErr, layer, uncompressed, destDir, index, layerDesc)
	}

	log.Debug("Successfully extracted layer", logFieldIndex, index, logFieldDigest, layerDesc)
	return nil
}

// extractRawData attempts to extract raw data from the layer as a fallback.
func extractRawData(reader io.Reader, destDir string, layerIndex int) error {
	// Create a temporary file to store the raw data
	tempFile := filepath.Join(destDir, fmt.Sprintf("layer_%d_raw", layerIndex))

	file, err := os.Create(tempFile)
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}
	defer file.Close()

	// Copy the raw data
	_, err = io.Copy(file, reader)
	if err != nil {
		return fmt.Errorf("failed to copy raw data: %w", err)
	}

	log.Debug("Extracted raw data to temp file", "file", tempFile)
	return nil
}

// validateZipFilePath validates a file path from a ZIP archive for security.
func validateZipFilePath(fileName string) error {
	// Check for empty or null filename
	if fileName == "" {
		return fmt.Errorf("%w: empty filename", errIllegalZipFilePath)
	}

	// Check for absolute paths
	if filepath.IsAbs(fileName) {
		return fmt.Errorf("%w: absolute path not allowed: %s", errIllegalZipFilePath, fileName)
	}

	// Check for path traversal patterns
	if strings.Contains(fileName, "..") {
		return fmt.Errorf("%w: path traversal not allowed: %s", errIllegalZipFilePath, fileName)
	}

	// Check for Windows absolute paths (drive letter followed by colon and backslash)
	if len(fileName) >= 3 && fileName[1] == ':' && (fileName[2] == '\\' || fileName[2] == '/') {
		return fmt.Errorf("%w: Windows absolute path not allowed: %s", errIllegalZipFilePath, fileName)
	}

	// Check for leading slashes or backslashes (Unix/Windows absolute-like paths)
	if strings.HasPrefix(fileName, "/") || strings.HasPrefix(fileName, "\\") {
		return fmt.Errorf("%w: leading separator not allowed: %s", errIllegalZipFilePath, fileName)
	}

	// Check for null bytes (potential injection)
	if strings.Contains(fileName, "\x00") {
		return fmt.Errorf("%w: null byte not allowed: %s", errIllegalZipFilePath, fileName)
	}

	return nil
}

// resolveZipFilePath resolves and validates the destination path for a ZIP file entry.
func resolveZipFilePath(destDir, fileName string) (string, error) {
	// Ensure destination directory is absolute and clean
	cleanDest, err := filepath.Abs(filepath.Clean(destDir))
	if err != nil {
		return "", fmt.Errorf("%w: %v", errFailedToResolveDestination, err)
	}

	// Join the paths and clean the result
	joined := filepath.Join(cleanDest, fileName)
	filePath := filepath.Clean(joined)

	// Ensure the resolved path is within the destination directory
	// This prevents directory traversal attacks
	if !strings.HasPrefix(filePath, cleanDest+string(os.PathSeparator)) && filePath != cleanDest {
		return "", fmt.Errorf("%w: path outside destination directory: %s", errIllegalZipFilePath, fileName)
	}

	return filePath, nil
}

// extractZipFileEntry extracts a single file from a ZIP archive.
func extractZipFileEntry(file *zip.File, destDir string) error {
	// Validate the file path
	if err := validateZipFilePath(file.Name); err != nil {
		return err
	}

	// Resolve the destination path
	filePath, err := resolveZipFilePath(destDir, file.Name)
	if err != nil {
		return err
	}

	// Create parent directories if they don't exist
	if err := os.MkdirAll(filepath.Dir(filePath), defaultDirMode); err != nil {
		return fmt.Errorf("%w for %s: %w", errFailedToCreateDirectory, file.Name, err)
	}

	// Open the file in the ZIP
	rc, err := file.Open()
	if err != nil {
		return fmt.Errorf("%w %s: %w", errFailedToOpenZipFile, file.Name, err)
	}
	defer rc.Close()

	// Create the destination file
	dstFile, err := os.Create(filePath)
	if err != nil {
		return fmt.Errorf("%w %s: %w", errFailedToCreateFile, filePath, err)
	}
	defer dstFile.Close()

	// Copy the file contents
	if _, err := io.Copy(dstFile, rc); err != nil {
		return fmt.Errorf("%w %s: %w", errFailedToCopyFile, file.Name, err)
	}

	log.Debug("Extracted file from ZIP", "file", file.Name, "path", filePath)
	return nil
}

// shouldSkipZipFile determines if a ZIP file entry should be skipped.
func shouldSkipZipFile(file *zip.File) (bool, string) {
	// Skip directories
	if file.FileInfo().IsDir() {
		return true, "directory"
	}

	// Skip symlinks for security
	if file.FileInfo().Mode()&os.ModeSymlink != 0 {
		return true, "symlink"
	}

	return false, ""
}

// extractZipFile extracts a ZIP file from an io.Reader into the destination directory.
func extractZipFile(reader io.Reader, destDir string) error {
	// Read the ZIP data with size limit to prevent decompression bomb attacks
	limitedReader := io.LimitReader(reader, int64(maxZipSize)+1)
	zipData, err := io.ReadAll(limitedReader)
	if err != nil {
		return fmt.Errorf("%w: %v", errFailedToReadZipData, err)
	}

	// Reject if ZIP exceeds configured max size.
	if len(zipData) > maxZipSize {
		return fmt.Errorf("%w: %d bytes", errZipSizeExceeded, maxZipSize)
	}

	// Create a ZIP reader
	zipReader, err := zip.NewReader(bytes.NewReader(zipData), int64(len(zipData)))
	if err != nil {
		return fmt.Errorf("%w: %v", errFailedToCreateZipReader, err)
	}

	// Extract each file in the ZIP
	for _, file := range zipReader.File {
		// Check if we should skip this file
		if skip, reason := shouldSkipZipFile(file); skip {
			if reason == "symlink" {
				log.Warn("Skipping symlink in ZIP", "name", file.Name)
			}
			continue
		}

		// Extract the file
		if err := extractZipFileEntry(file, destDir); err != nil {
			return err
		}
	}

	log.Debug("Successfully extracted ZIP file", "destination", destDir)
	return nil
}

// checkArtifactType to check and log artifact type mismatches.
func checkArtifactType(descriptor *remote.Descriptor, imageName string) {
	manifest, err := parseOCIManifest(bytes.NewReader(descriptor.Manifest))
	if err != nil {
		log.Error("Failed to parse OCI manifest", "image", imageName, logFieldError, err)
		return
	}

	// Check if the artifact type is supported
	supportedTypes := []string{
		targetArtifactType,
		opentofuArtifactType,
		terraformArtifactType,
	}

	isSupported := false
	for _, supportedType := range supportedTypes {
		if manifest.ArtifactType == supportedType {
			isSupported = true
			break
		}
	}

	if !isSupported {
		log.Warn("OCI image artifact type not recognized", "image", imageName, "artifactType", manifest.ArtifactType, "supportedTypes", supportedTypes)
	} else {
		log.Debug("OCI image artifact type is supported", "image", imageName, "artifactType", manifest.ArtifactType)
	}
}

// parseOCIManifest reads and decodes an OCI manifest from a JSON file.
func parseOCIManifest(manifestBytes io.Reader) (*ocispec.Manifest, error) {
	var manifest ocispec.Manifest
	if err := json.NewDecoder(manifestBytes).Decode(&manifest); err != nil {
		return nil, fmt.Errorf("parse OCI manifest: %w", err)
	}

	return &manifest, nil
}
