package exec

import (
	"archive/tar"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	log "github.com/cloudposse/atmos/pkg/logger" // Charmbracelet structured logger
)

// extractTarball extracts the tarball file from an io.Reader into the destination directory .
func extractTarball(reader io.Reader, extractPath string) error {
	// Call untar function to handle tar extraction
	return untar(reader, extractPath)
}

// untar extracts a tar archive into the destination directory .
func untar(reader io.Reader, extractPath string) error {
	tarReader := tar.NewReader(reader)

	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			log.Error("Error reading tar header", "error", err)
			return err
		}
		if strings.Contains(header.Name, "..") {
			log.Warn("Skipping potential directory traversal attempt", "filename", header.Name)
			continue
		}
		if err := processTarHeader(header, tarReader, extractPath); err != nil {
			return err
		}
	}

	return nil
}

// processTarHeader processes a tar header and writes the corresponding file to the destination directory.
func processTarHeader(header *tar.Header, tarReader *tar.Reader, extractPath string) error {
	// Normalize and clean the extraction base path to remove any redundant separators or ".." sequences.
	cleanExtractPath := filepath.Clean(extractPath)
	// Clean the file path inside the archive to prevent directory traversal attacks.
	cleanHeaderName := filepath.Clean(header.Name)
	// Clean the file path inside the archive to prevent directory traversal attacks.
	filePath := filepath.Join(cleanExtractPath, cleanHeaderName)
	// Ensure the target path is within the intended extraction directory.
	if !strings.HasPrefix(filePath, cleanExtractPath) {
		return fmt.Errorf("%w: %s", errUtils.ErrInvalidFilePath, filePath)
	}
	switch header.Typeflag {
	case tar.TypeDir:
		return createDirectory(filePath)
	case tar.TypeReg:
		return createFileFromTar(filePath, tarReader, header)
	default:
		log.Warnf("Unsupported file type: %v in %s", header.Typeflag, header.Name)
	}

	return nil
}

// createDirectory creates a directory at the specified path. If the directory already exists, it does nothing.
func createDirectory(dirPath string) error {
	if err := os.MkdirAll(dirPath, os.ModePerm); err != nil {
		return fmt.Errorf("error creating directory %s: %w", dirPath, err)
	}
	return nil
}

// createFileFromTar writes the contents of a tar file to a file at the specified path. It also sets the file mode.
func createFileFromTar(filePath string, tarReader *tar.Reader, header *tar.Header) error {
	err := os.MkdirAll(filepath.Dir(filePath), os.ModePerm)
	if err != nil {
		log.Error("Failed to create parent directory for file", "path", filePath, "error", err)
		return err
	}
	writer, err := os.Create(filePath)
	if err != nil {
		log.Error("Failed to create file", "path", filePath, "error", err)
		return err
	}
	defer writer.Close()
	_, err = io.Copy(writer, tarReader)
	if err != nil {
		log.Error("Failed to write file contents", "path", filePath, "error", err)
		return err
	}
	// Set correct permissions (remove setuid/setgid bits for security) , os.ModeSetuid, os.ModeSetgid standard Cross-platform
	newMode := header.FileInfo().Mode() &^ (os.ModeSetuid | os.ModeSetgid)
	// Set permissions using os.Chmod for all platforms
	if err := os.Chmod(filePath, newMode); err != nil {
		log.Error("Failed to set file permissions", "path", filePath, "error", err)
	}
	return nil
}
