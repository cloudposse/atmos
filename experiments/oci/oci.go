//go:build !linting
// +build !linting

package main

import (
	"archive/tar"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"

	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"oras.land/oras-go/v2/registry/remote"
)

func main() {
	ctx := context.Background()
	registry := "ghcr.io"
	repository := "devcontainers/features/common-utils"
	version := "2.5.2"

	// Initialize a remote repository (anonymous)
	repo, err := remote.NewRepository(registry + "/" + repository)
	if err != nil {
		log.Fatalf("Failed to create repository: %v", err)
	}

	// Resolve the artifact descriptor
	descriptor, err := repo.Resolve(ctx, version)
	if err != nil {
		log.Fatalf("Failed to resolve descriptor: %v", err)
	}

	fmt.Println("✅ Artifact resolved successfully:", descriptor.Digest)

	// Fetch the manifest content
	manifestReader, err := repo.Fetch(ctx, descriptor)
	if err != nil {
		log.Fatalf("Failed to fetch manifest: %v", err)
	}
	defer manifestReader.Close()

	// Decode the manifest JSON
	var manifest ocispec.Manifest
	if err := json.NewDecoder(manifestReader).Decode(&manifest); err != nil {
		log.Fatalf("Failed to decode manifest: %v", err)
	}

	// Define output directory as ./tmp in the current working directory
	cwd, err := os.Getwd()
	if err != nil {
		log.Fatalf("Failed to get current working directory: %v", err)
	}

	// Extract feature name dynamically from repository URL
	featureName := filepath.Base(repository)

	// Define output directory dynamically as ./tmp/features/{featureName}
	outputDir := filepath.Join(cwd, "tmp", "features", featureName)

	// Create the ./tmp directory if it doesn't exist
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		log.Fatalf("Failed to create output directory: %v", err)
	}

	// Iterate through layers and extract them
	for _, layer := range manifest.Layers {
		reader, err := repo.Fetch(ctx, layer)
		if err != nil {
			log.Fatalf("Failed to fetch layer: %v", err)
		}

		filePath := filepath.Join(outputDir, filepath.Base(layer.Digest.String()))
		fmt.Printf("📂 Extracting layer: %s\n", filePath)

		// Save the layer as a file
		file, err := os.Create(filePath)
		if err != nil {
			log.Fatalf("Failed to create file: %v", err)
		}

		_, err = io.Copy(file, reader)
		if err != nil {
			log.Fatalf("Failed to write file: %v", err)
		}

		file.Close()

		// Attempt to extract if it's a tarball
		extractTar(filePath, outputDir)
		os.Remove(filePath) // Delete the tar file after extraction
	}

	fmt.Println("🎉 Feature unpacked successfully into:", outputDir)
}

// extractTar extracts a tar file if it's a valid archive.
func extractTar(tarPath, outputDir string) {
	file, err := os.Open(tarPath)
	if err != nil {
		log.Printf("Skipping extraction: cannot open file %s\n", tarPath)
		return
	}
	defer file.Close()

	tr := tar.NewReader(file)

	for {
		header, err := tr.Next()
		if err == io.EOF {
			break // End of archive
		}

		if err != nil {
			log.Printf("Skipping extraction: error reading tar %s: %v\n", tarPath, err)
			return
		}

		target := filepath.Join(outputDir, header.Name)

		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o755); err != nil {
				log.Fatalf("Failed to create directory: %v", err)
			}
		case tar.TypeReg:
			outFile, err := os.Create(target)
			if err != nil {
				log.Fatalf("Failed to create file: %v", err)
			}

			if _, err := io.Copy(outFile, tr); err != nil {
				log.Fatalf("Failed to extract file: %v", err)
			}

			outFile.Close()
		}
	}

	fmt.Printf("📂 Extracted contents from %s\n", tarPath)
}
