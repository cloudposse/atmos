// https://opencontainers.org/
// https://github.com/google/go-containerregistry
// https://docs.aws.amazon.com/AmazonECR/latest/public/public-registries.html

package exec

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/google/go-containerregistry/pkg/v1/tarball"
	"github.com/google/uuid"

	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// processOciImage downloads an Image from an OCI-compatible registry, extracts the layers from the tarball, and writes to the destination directory
func processOciImage(cliConfig schema.CliConfiguration, imageName string, destDir string) error {
	// Temp directory for the tarball files
	tempDir, err := os.MkdirTemp("", uuid.New().String())
	if err != nil {
		return err
	}

	defer removeTempDir(cliConfig, tempDir)

	// Temp tarball file name
	tempTarFileName := filepath.Join(tempDir, uuid.New().String()) + ".tar"

	// Get the image reference from the OCI registry
	ref, err := name.ParseReference(imageName)
	if err != nil {
		return fmt.Errorf("cannot parse reference of the image '%s'. Error: %v", imageName, err)
	}

	// Get the image descriptor
	descriptor, err := remote.Get(ref)
	if err != nil {
		return fmt.Errorf("cannot get image '%s'. Error: %v", imageName, err)
	}

	// Download the image from the OCI registry
	image, err := descriptor.Image()
	if err != nil {
		return fmt.Errorf("cannot get a descriptor for the OCI image '%s'. Error: %v", imageName, err)
	}

	// Write the image tarball to the temp directory
	err = tarball.WriteToFile(tempTarFileName, ref, image)
	if err != nil {
		return err
	}

	// Get the tarball manifest
	m, err := tarball.LoadManifest(func() (io.ReadCloser, error) {
		f, err := os.Open(tempTarFileName)
		if err != nil {
			u.LogError(cliConfig, err)
			return nil, err
		}
		return f, nil
	})
	if err != nil {
		return err
	}

	if len(m) == 0 {
		return fmt.Errorf(
			"the OCI image '%s' does not have a manifest. Refer to https://docs.docker.com/registry/spec/manifest-v2-2",
			imageName)
	}

	manifest := m[0]

	// Check the tarball layers
	if len(manifest.Layers) == 0 {
		return fmt.Errorf("the OCI image '%s' does not have any layers", imageName)
	}

	// Extract the tarball layers into the temp directory
	// The tarball layers are tarballs themselves
	err = extractTarball(cliConfig, tempTarFileName, tempDir)
	if err != nil {
		return err
	}

	// Extract the layers into the destination directory
	for _, l := range manifest.Layers {
		layerPath := filepath.Join(tempDir, l)

		err = extractTarball(cliConfig, layerPath, destDir)
		if err != nil {
			return err
		}
	}

	return nil
}
