package exec

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	log "github.com/charmbracelet/log" // Charmbracelet structured logger
	"github.com/pkg/errors"
	"github.com/spf13/viper"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/google/uuid"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"

	"github.com/cloudposse/atmos/pkg/schema"
)

var ErrNoLayers = errors.New("the OCI image does not have any layers")

const (
	targetArtifactType = "application/vnd.atmos.component.terraform.v1+tar+gzip" // Target artifact type for Atmos components
	// Additional supported artifact types
	opentofuArtifactType  = "application/vnd.opentofu.modulepkg"           // OpenTofu module package
	terraformArtifactType = "application/vnd.terraform.module.v1+tar+gzip" // Terraform module package
	githubTokenEnv        = "GITHUB_TOKEN"
)

// bindEnv binds environment variables to Viper with fallback support
func bindEnv(v *viper.Viper, key string, envVars ...string) {
	if err := v.BindEnv(append([]string{key}, envVars...)...); err != nil {
		log.Debug("Failed to bind environment variable", "key", key, "envVars", envVars, "error", err)
	}
}

// processOciImage processes an OCI image and extracts its layers to the specified destination directory.
func processOciImage(atmosConfig *schema.AtmosConfiguration, imageName string, destDir string) error {
	tempDir, err := os.MkdirTemp("", uuid.New().String())
	if err != nil {
		return fmt.Errorf("failed to create temp directory: %w", err)
	}
	defer removeTempDir(tempDir)

	ref, err := name.ParseReference(imageName)
	if err != nil {
		log.Error("Failed to parse OCI image reference", "image", imageName, "error", err)
		return fmt.Errorf("invalid image reference: %w", err)
	}

	descriptor, err := pullImage(ref, atmosConfig)
	if err != nil {
		return fmt.Errorf("failed to pull image: %w", err)
	}

	img, err := descriptor.Image()
	if err != nil {
		log.Error("Failed to get image descriptor", "image", imageName, "error", err)
		return fmt.Errorf("cannot get a descriptor for the OCI image '%s': %w", imageName, err)
	}

	checkArtifactType(descriptor, imageName)

	layers, err := img.Layers()
	if err != nil {
		log.Error("Failed to retrieve layers from OCI image", "image", imageName, "error", err)
		return fmt.Errorf("failed to get image layers: %w", err)
	}

	if len(layers) == 0 {
		log.Warn("OCI image has no layers", "image", imageName)
		return ErrNoLayers
	}

	successfulLayers := 0
	for i, layer := range layers {
		if err := processLayer(layer, i, destDir); err != nil {
			log.Warn("Failed to process layer", "index", i, "error", err)
			continue // Continue with other layers instead of failing completely
		}
		successfulLayers++
	}

	// Check if any files were actually extracted
	files, err := os.ReadDir(destDir)
	if err != nil {
		log.Warn("Could not read destination directory", "dir", destDir, "error", err)
	} else if len(files) == 0 {
		log.Warn("No files were extracted to destination directory", "dir", destDir, "totalLayers", len(layers), "successfulLayers", successfulLayers)
	} else {
		log.Debug("Successfully extracted files", "dir", destDir, "fileCount", len(files), "totalLayers", len(layers), "successfulLayers", successfulLayers)
	}

	return nil
}

// pullImage pulls an OCI image from the specified reference and returns its descriptor.
func pullImage(ref name.Reference, atmosConfig *schema.AtmosConfiguration) (*remote.Descriptor, error) {
	var opts []remote.Option

	// Get registry from parsed reference
	registry := ref.Context().Registry.Name()

	// Try to get authentication from various sources
	auth, err := getRegistryAuth(registry, atmosConfig)
	if err != nil {
		log.Debug("No authentication found, using anonymous", "registry", registry)
		opts = append(opts, remote.WithAuth(authn.Anonymous))
	} else {
		opts = append(opts, remote.WithAuth(auth))
		log.Debug("Using authentication for registry", "registry", registry)
	}

	descriptor, err := remote.Get(ref, opts...)
	if err != nil {
		log.Error("Failed to pull OCI image", "image", ref.Name(), "error", err)
		return nil, fmt.Errorf("failed to pull image '%s': %w", ref.Name(), err)
	}

	return descriptor, nil
}

// getRegistryAuth attempts to find authentication credentials for the given registry.
// It checks multiple sources in order of preference:
// 1. GitHub Container Registry (ghcr.io) with GITHUB_TOKEN
// 2. Docker credential helpers (from ~/.docker/config.json)
// 3. Environment variables for specific registries
// 4. AWS ECR authentication (if AWS credentials are available)
func getRegistryAuth(registry string, atmosConfig *schema.AtmosConfiguration) (authn.Authenticator, error) {
	// Create a Viper instance for environment variable access
	v := viper.New()

	// Check for GitHub Container Registry
	if strings.EqualFold(registry, "ghcr.io") {
		if auth, err := getGitHubAuth(registry, atmosConfig); err == nil {
			log.Debug("Using GitHub authentication", "registry", registry)
			return auth, nil
		}
	}

	// Check for Docker credential helpers (most common for private registries)
	// This will automatically check ~/.docker/config.json and use credential helpers
	if auth, err := getDockerAuth(registry, atmosConfig); err == nil {
		log.Debug("Using Docker config authentication", "registry", registry)
		return auth, nil
	}

	// Check for custom environment variables for specific registries
	// Format: REGISTRY_NAME_USERNAME and REGISTRY_NAME_PASSWORD
	// Example: MY_REGISTRY_COM_USERNAME and MY_REGISTRY_COM_PASSWORD
	// Normalize registry name by replacing dots and hyphens with underscores for valid env var names
	registryEnvName := strings.ToUpper(strings.NewReplacer(".", "_", "-", "_").Replace(registry))
	usernameKey := fmt.Sprintf("%s_username", registryEnvName)
	passwordKey := fmt.Sprintf("%s_password", registryEnvName)

	// Bind the registry-specific environment variables
	bindEnv(v, usernameKey, fmt.Sprintf("%s_USERNAME", registryEnvName))
	bindEnv(v, passwordKey, fmt.Sprintf("%s_PASSWORD", registryEnvName))

	username := v.GetString(usernameKey)
	password := v.GetString(passwordKey)

	if username != "" && password != "" {
		log.Debug("Using environment variable authentication", "registry", registry)
		return &authn.Basic{
			Username: username,
			Password: password,
		}, nil
	}

	// Check for AWS ECR authentication (including FIPS and China endpoints)
	if strings.Contains(registry, "dkr.ecr") && strings.Contains(registry, "amazonaws.com") {
		if auth, err := getECRAuth(registry); err == nil {
			log.Debug("Using AWS ECR authentication", "registry", registry)
			return auth, nil
		} else {
			// Return the specific ECR error for better debugging
			return nil, err
		}
	}

	// Check for Azure Container Registry authentication
	if strings.Contains(registry, "azurecr.io") {
		if auth, err := getACRAuth(registry, atmosConfig); err == nil {
			log.Debug("Using Azure ACR authentication", "registry", registry)
			return auth, nil
		} else {
			// Return the specific Azure error for better debugging
			return nil, err
		}
	}

	// Check for Google Container Registry authentication
	if strings.Contains(registry, "gcr.io") || strings.Contains(registry, "pkg.dev") {
		if auth, err := getGCRAuth(registry); err == nil {
			log.Debug("Using Google GCR authentication", "registry", registry)
			return auth, nil
		}
	}

	return nil, fmt.Errorf("no authentication found for registry %s", registry)
}

// processLayer processes a single OCI layer and extracts its contents to the specified destination directory.
func processLayer(layer v1.Layer, index int, destDir string) error {
	layerDesc, err := layer.Digest()
	if err != nil {
		log.Warn("Skipping layer with invalid digest", "index", index, "error", err)
		return nil
	}

	// Get layer size for debugging
	size, err := layer.Size()
	if err != nil {
		log.Warn("Could not get layer size", "index", index, "digest", layerDesc, "error", err)
	} else {
		log.Debug("Processing layer", "index", index, "digest", layerDesc, "size", size)
	}

	// Get layer media type for debugging
	mediaType, err := layer.MediaType()
	if err != nil {
		log.Warn("Could not get layer media type", "index", index, "digest", layerDesc, "error", err)
	} else {
		log.Debug("Layer media type", "index", index, "digest", layerDesc, "mediaType", mediaType)
	}

	uncompressed, err := layer.Uncompressed()
	if err != nil {
		log.Error("Layer decompression failed", "index", index, "digest", layerDesc, "error", err)
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
		log.Debug("Detected ZIP layer, extracting as ZIP", "index", index, "digest", layerDesc, "mediaType", mediaTypeStr)
		extractionErr = extractZipFile(uncompressed, destDir)
	} else {
		// Default to tar extraction
		log.Debug("Extracting as TAR", "index", index, "digest", layerDesc, "mediaType", mediaTypeStr)
		extractionErr = extractTarball(uncompressed, destDir)
	}

	if extractionErr != nil {
		log.Error("Layer extraction failed", "index", index, "digest", layerDesc, "error", extractionErr)

		// Try alternative extraction methods for different formats
		log.Debug("Attempting alternative extraction methods", "index", index, "digest", layerDesc)

		// Reset the uncompressed reader
		if uncompressed != nil {
			_ = uncompressed.Close()
		}
		uncompressed, err = layer.Uncompressed()
		if err != nil {
			log.Error("Failed to reset uncompressed reader", "index", index, "digest", layerDesc, "error", err)
			return fmt.Errorf("layer decompression error: %w", err)
		}
		// No second defer; the single deferred closer will close the new handle

		// Try to extract as raw data first
		if err := extractRawData(uncompressed, destDir, index); err != nil {
			log.Error("Raw data extraction also failed", "index", index, "digest", layerDesc, "error", err)

			// If this is the first layer and it fails, it might be metadata
			if index == 0 {
				log.Warn("First layer extraction failed, this might be metadata. Skipping layer.", "index", index, "digest", layerDesc)
				return nil // Skip this layer instead of failing
			}

			return fmt.Errorf("all extraction methods failed: %w", err)
		}

		log.Debug("Successfully extracted layer using alternative method", "index", index, "digest", layerDesc)
		return nil
	}

	log.Debug("Successfully extracted layer", "index", index, "digest", layerDesc)
	return nil
}

// extractRawData attempts to extract raw data from the layer as a fallback
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

// extractZipFile extracts a ZIP file from an io.Reader into the destination directory
func extractZipFile(reader io.Reader, destDir string) error {
	// Read the entire ZIP data into memory
	zipData, err := io.ReadAll(reader)
	if err != nil {
		return fmt.Errorf("failed to read ZIP data: %w", err)
	}

	// Create a ZIP reader
	zipReader, err := zip.NewReader(bytes.NewReader(zipData), int64(len(zipData)))
	if err != nil {
		return fmt.Errorf("failed to create ZIP reader: %w", err)
	}

	// Extract each file in the ZIP
	for _, file := range zipReader.File {
		// Skip directories
		if file.FileInfo().IsDir() {
			continue
		}

		// Skip symlinks for security
		if file.FileInfo().Mode()&os.ModeSymlink != 0 {
			log.Warn("Skipping symlink in ZIP", "name", file.Name)
			continue
		}

		// Create the file path (guard against Zip Slip)
		// First, check for absolute paths and path traversal patterns
		if filepath.IsAbs(file.Name) || strings.Contains(file.Name, "..") {
			return fmt.Errorf("illegal file path in ZIP: %s", file.Name)
		}

		// Check for Windows absolute paths (drive letter followed by colon and backslash)
		if len(file.Name) >= 3 && file.Name[1] == ':' && (file.Name[2] == '\\' || file.Name[2] == '/') {
			return fmt.Errorf("illegal file path in ZIP: %s", file.Name)
		}

		// Then use the standard path joining and validation
		joined := filepath.Join(destDir, file.Name)
		cleanDest := filepath.Clean(destDir)
		filePath := filepath.Clean(joined)
		if !strings.HasPrefix(filePath, cleanDest+string(os.PathSeparator)) && filePath != cleanDest {
			return fmt.Errorf("illegal file path in ZIP: %s", file.Name)
		}

		// Create parent directories if they don't exist
		if err := os.MkdirAll(filepath.Dir(filePath), 0o755); err != nil {
			return fmt.Errorf("failed to create directory for %s: %w", file.Name, err)
		}

		// Open the file in the ZIP
		rc, err := file.Open()
		if err != nil {
			return fmt.Errorf("failed to open file %s in ZIP: %w", file.Name, err)
		}

		// Create the destination file
		dstFile, err := os.Create(filePath)
		if err != nil {
			rc.Close()
			return fmt.Errorf("failed to create file %s: %w", filePath, err)
		}

		// Copy the file contents
		if _, err := io.Copy(dstFile, rc); err != nil {
			rc.Close()
			dstFile.Close()
			return fmt.Errorf("failed to copy file %s: %w", file.Name, err)
		}

		// Close both files explicitly
		rc.Close()
		dstFile.Close()

		log.Debug("Extracted file from ZIP", "file", file.Name, "path", filePath)
	}

	log.Debug("Successfully extracted ZIP file", "destination", destDir)
	return nil
}

// checkArtifactType to check and log artifact type mismatches .
func checkArtifactType(descriptor *remote.Descriptor, imageName string) {
	manifest, err := parseOCIManifest(bytes.NewReader(descriptor.Manifest))
	if err != nil {
		log.Error("Failed to parse OCI manifest", "image", imageName, "error", err)
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

// ParseOCIManifest reads and decodes an OCI manifest from a JSON file.
func parseOCIManifest(manifestBytes io.Reader) (*ocispec.Manifest, error) {
	var manifest ocispec.Manifest
	if err := json.NewDecoder(manifestBytes).Decode(&manifest); err != nil {
		return nil, err
	}

	return &manifest, nil
}
