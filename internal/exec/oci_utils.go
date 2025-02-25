package exec

import (
	"fmt"
	"os"
	"path/filepath"

	log "github.com/charmbracelet/log" // Charmbracelet structured logger

	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/google/go-containerregistry/pkg/v1/tarball"
	"github.com/google/uuid"

	"github.com/cloudposse/atmos/pkg/schema"
)

// Target artifact type for Atmos components
const targetArtifactType = "application/vnd.atmos.component.terraform.v1+tar+gzip"

// processOciImage downloads an OCI image, extracts its layers, and writes to the destination directory.
func processOciImage(atmosConfig schema.AtmosConfiguration, imageName string, destDir string) error {
	// Temp directory for the tarball files
	tempDir, err := os.MkdirTemp("", uuid.New().String())
	if err != nil {
		return err
	}
	defer removeTempDir(atmosConfig, tempDir)

	// Temp tarball file name
	tempTarFileName := filepath.Join(tempDir, uuid.New().String()) + ".tar"

	// Get the image reference from the OCI registry
	ref, err := name.ParseReference(imageName)
	if err != nil {
		log.Error("Failed to parse OCI image reference", "image", imageName, "error", err)
		return fmt.Errorf("cannot parse reference of the image '%s': %v", imageName, err)
	}

	// Get the image descriptor (includes MediaType)
	descriptor, err := remote.Get(ref)
	if err != nil {
		log.Error("Failed to get OCI image", "image", imageName, "error", err)
		return fmt.Errorf("cannot get image '%s': %v", imageName, err)
	}

	// Check if we need to filter by MediaType
	filterByArtifactType := descriptor.MediaType == targetArtifactType

	// Download the image from the OCI registry
	image, err := descriptor.Image()
	if err != nil {
		log.Error("Failed to get image descriptor", "image", imageName, "error", err)
		return fmt.Errorf("cannot get a descriptor for the OCI image '%s': %v", imageName, err)
	}

	// Write the image tarball to the temp directory
	err = tarball.WriteToFile(tempTarFileName, ref, image)
	if err != nil {
		return err
	}

	// Load tarball image
	img, err := tarball.ImageFromPath(tempTarFileName, nil)
	if err != nil {
		log.Error("Failed to load OCI image from tarball", "image", imageName, "error", err)
		return fmt.Errorf("failed to load image from tarball: %v", err)
	}

	// Get image layers
	layers, err := img.Layers()
	if err != nil {
		log.Error("Failed to retrieve layers from OCI image", "image", imageName, "error", err)
		return fmt.Errorf("failed to get image layers: %v", err)
	}

	if len(layers) == 0 {
		log.Warn("OCI image has no layers", "image", imageName)
		return fmt.Errorf("the OCI image '%s' does not have any layers", imageName)
	}

	log.Info("Extracting OCI image", "image", imageName, "layers", len(layers))

	// Extract layers
	for _, layer := range layers {
		// Get layer metadata
		layerDesc, err := layer.Digest()
		if err != nil {
			log.Error("Failed to retrieve layer digest", "image", imageName, "error", err)
			return fmt.Errorf("failed to get layer digest: %v", err)
		}

		// Get uncompressed layer reader
		layerReader, err := layer.Uncompressed()
		if err != nil {
			log.Error("Failed to get uncompressed layer", "image", imageName, "error", err)
			return fmt.Errorf("failed to get uncompressed layer: %v", err)
		}
		defer layerReader.Close()

		// Determine if we should extract the layer
		if filterByArtifactType {
			log.Warn("Skipping layer due to artifact type mismatch", "layer", layerDesc.String(), "artifact_type", descriptor.MediaType)
			continue
		}

		log.Info("Extracting layer", "layer", layerDesc.String())

		// Extract the tarball layer
		err = extractTarball(layerReader, destDir)
		if err != nil {
			return err
		}
	}

	log.Info("Successfully extracted OCI image", "image", imageName)
	return nil
}
