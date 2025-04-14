package exec

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	log "github.com/charmbracelet/log" // Charmbracelet structured logger
	"github.com/pkg/errors"

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
	githubTokenEnv     = "GITHUB_TOKEN"
)

// processOciImage processes an OCI image and extracts its layers to the specified destination directory.
func processOciImage(atmosConfig *schema.AtmosConfiguration, imageName string, destDir string) error {
	tempDir, err := os.MkdirTemp("", uuid.New().String())
	if err != nil {
		return fmt.Errorf("failed to create temp directory: %w", err)
	}
	defer removeTempDir(*atmosConfig, tempDir)

	ref, err := name.ParseReference(imageName)
	if err != nil {
		log.Error("Failed to parse OCI image reference", "image", imageName, "error", err)
		return fmt.Errorf("invalid image reference: %w", err)
	}

	descriptor, err := pullImage(ref)
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

	for i, layer := range layers {
		if err := processLayer(layer, i, destDir); err != nil {
			return fmt.Errorf("failed to process layer %d: %w", i, err)
		}
	}

	return nil
}

// pullImage pulls an OCI image from the specified reference and returns its descriptor.
func pullImage(ref name.Reference) (*remote.Descriptor, error) {
	var opts []remote.Option
	opts = append(opts, remote.WithAuth(authn.Anonymous))

	// Get registry from parsed reference
	registry := ref.Context().Registry.Name()
	if strings.EqualFold(registry, "ghcr.io") {
		githubToken := os.Getenv(githubTokenEnv)
		if githubToken != "" {
			opts = append(opts, remote.WithAuth(&authn.Basic{
				Username: "oauth2",
				Password: githubToken,
			}))
			log.Debug("Using GitHub token for authentication", "registry", registry)
		}
	}

	descriptor, err := remote.Get(ref, opts...)
	if err != nil {
		log.Error("Failed to pull OCI image", "image", ref.Name(), "error", err)
		return nil, fmt.Errorf("failed to pull image '%s': %w", ref.Name(), err)
	}

	return descriptor, nil
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
		return fmt.Errorf("layer decompression error: %w", err)
	}
	defer uncompressed.Close()

	if err := extractTarball(uncompressed, destDir); err != nil {
		log.Error("Layer extraction failed", "index", index, "digest", layerDesc, "error", err)
		return fmt.Errorf("tarball extraction error: %w", err)
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
