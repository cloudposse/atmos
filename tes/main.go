package main

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"log"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/google/go-containerregistry/pkg/v1/tarball"

	u "github.com/cloudposse/atmos/pkg/utils"
)

const imageName = "public.ecr.aws/r7v2l4o9/vpc:latest"
const tarFileName = "vpc.tar"
const dstDir = "/Users/andriyknysh/Documents/Projects/Go/src/github.com/cloudposse/atmos/tes/2"

func extractTarball(sourceFile, extractPath string) error {
	file, err := os.Open(sourceFile)
	if err != nil {
		return err
	}

	defer func(file *os.File) {
		err := file.Close()
		if err != nil {
			u.LogError(fmt.Errorf("\nerror closing the file %s: %v\n", sourceFile, err))
		}
	}(file)

	var fileReader io.ReadCloser = file

	if strings.HasSuffix(sourceFile, ".gz") {
		if fileReader, err = gzip.NewReader(file); err != nil {
			return err
		}

		defer func(fileReader io.ReadCloser) {
			err := fileReader.Close()
			if err != nil {
				u.LogError(fmt.Errorf("\nerror closing the file %s: %v\n", sourceFile, err))
			}
		}(fileReader)
	}

	tarBallReader := tar.NewReader(fileReader)

	for {
		header, err := tarBallReader.Next()
		if err != nil {
			if err == io.EOF {
				break
			}
			return err
		}

		filename := filepath.Join(extractPath, filepath.FromSlash(header.Name))

		switch header.Typeflag {
		case tar.TypeDir:
			err = os.MkdirAll(filename, os.FileMode(header.Mode))
			if err != nil {
				return err
			}

		case tar.TypeReg:
			err := u.EnsureDir(filename)
			if err != nil {
				return err
			}

			writer, err := os.Create(filename)
			if err != nil {
				return err
			}

			_, err = io.Copy(writer, tarBallReader)
			if err != nil {
				return err
			}

			err = os.Chmod(filename, os.FileMode(header.Mode))
			if err != nil {
				return err
			}

			err = writer.Close()
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func main() {
	// Temp directory for the tarball files
	tempDir, err := os.MkdirTemp("", strconv.FormatInt(time.Now().Unix(), 10))
	if err != nil {
		log.Fatalf(err.Error())
	}

	defer func(path string) {
		err := os.RemoveAll(path)
		if err != nil {
			u.LogError(err)
		}
	}(tempDir)

	// Temp tarball file name
	tempTarFileName := path.Join(tempDir, tarFileName)

	// Get the image reference from the OCI registry
	ref, err := name.ParseReference(imageName)
	if err != nil {
		log.Fatalf("cannot parse reference of the image %s , detail: %v", imageName, err)
	}

	// Get the image descriptor
	descriptor, err := remote.Get(ref)
	if err != nil {
		log.Fatalf("cannot get image %s , detail: %v", imageName, err)
	}

	// Download the image from the OCI registry
	image, err := descriptor.Image()
	if err != nil {
		log.Fatalf("cannot get a descriptor for the OCI image %s. Error: %v", imageName, err)
	}

	// Write the image tarball to the temp directory
	err = tarball.WriteToFile(tempTarFileName, ref, image)
	if err != nil {
		log.Fatalf(err.Error())
	}

	// Get the tarball manifest
	m, err := tarball.LoadManifest(func() (io.ReadCloser, error) {
		f, err := os.Open(tempTarFileName)
		if err != nil {
			u.LogError(err)
			return nil, err
		}
		return f, nil
	})
	if err != nil {
		log.Fatalf(err.Error())
	}

	if len(m) == 0 {
		log.Fatalf("the OCI image '%s' does not have a correct manifest. Refer to https://docs.docker.com/registry/spec/manifest-v2-2",
			imageName)
	}

	manifest := m[0]

	// Check the tarball layers
	if len(manifest.Layers) == 0 {
		log.Fatalf("the OCI image '%s' does not have layers", imageName)
	}

	// Extract the tarball layers into the temp directory
	// The tarball layers are tarballs themselves
	err = extractTarball(tempTarFileName, tempDir)
	if err != nil {
		log.Fatalf(err.Error())
	}

	// Extract the layers into the destination directory
	for _, l := range manifest.Layers {
		layerPath := path.Join(tempDir, l)

		err = extractTarball(layerPath, dstDir)
		if err != nil {
			log.Fatalf(err.Error())
		}
	}
}
