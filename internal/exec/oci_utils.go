package exec

import (
	"fmt"
	"os"

	log "github.com/charmbracelet/log" // Charmbracelet structured logger

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/google/uuid"

	"github.com/cloudposse/atmos/pkg/schema"
)

// Target artifact type for Atmos components
const targetArtifactType = "application/vnd.oci.image.layer.v1.tar"

func processOciImage(atmosConfig schema.AtmosConfiguration, imageName string, destDir string) error {
	// Temp directory for operations
	tempDir, err := os.MkdirTemp("", uuid.New().String())
	if err != nil {
		return err
	}
	defer removeTempDir(atmosConfig, tempDir)

	// Parse image reference
	ref, err := name.ParseReference(imageName)
	if err != nil {
		log.Error("Failed to parse OCI image reference", "image", imageName, "error", err)
		return fmt.Errorf("invalid image reference: %v", err)
	}

	// Authentication setup
	githubToken := os.Getenv("GITHUB_TOKEN")
	if githubToken == "" {
		return fmt.Errorf("GITHUB_TOKEN environment variable is required")
	}
	auth := remote.WithAuth(&authn.Basic{
		Username: "oauth2",
		Password: githubToken,
	})

	// Get artifact descriptor
	descriptor, err := remote.Get(ref, auth)
	if err != nil {
		log.Error("Failed to get OCI image", "image", imageName, "error", err)
		return fmt.Errorf("cannot get image '%s': %v", imageName, err)
	}
	// Check if we need to filter by MediaType
	filterByArtifactType := descriptor.MediaType == targetArtifactType
	// Convert to image
	img, err := descriptor.Image()
	if err != nil {
		log.Error("Failed to get image descriptor", "image", imageName, "error", err)
		return fmt.Errorf("cannot get a descriptor for the OCI image '%s': %v", imageName, err)
	}

	// Process layers
	layers, err := img.Layers()
	if err != nil {
		log.Error("Failed to retrieve layers from OCI image", "image", imageName, "error", err)
		return fmt.Errorf("failed to get image layers: %v", err)
	}

	if len(layers) == 0 {
		log.Warn("OCI image has no layers", "image", imageName)
		return fmt.Errorf("the OCI image '%s' does not have any layers", imageName)
	}

	// Process each layer
	for i, layer := range layers {
		layerDesc, err := layer.Digest()
		if err != nil {
			log.Warn("Skipping layer with invalid digest", "index", i, "error", err)
			continue
		}

		// Extract layer contents
		uncompressed, err := layer.Uncompressed()
		if err != nil {
			log.Error("Layer decompression failed", "index", i, "digest", layerDesc, "error", err)
			return fmt.Errorf("layer decompression error: %v", err)
		}
		defer uncompressed.Close()
		// Determine if we should extract the layer
		if filterByArtifactType {
			log.Warn("Skipping layer due to artifact type mismatch", "layer", layerDesc.String(), "artifact_type", descriptor.MediaType)
			continue
		}

		if err := extractTarball(uncompressed, destDir); err != nil {
			log.Error("Layer extraction failed", "index", i, "digest", layerDesc, "error", err)
			return fmt.Errorf("tarball extraction error: %v", err)
		}
	}

	return nil
}
