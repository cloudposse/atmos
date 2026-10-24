package oci

import (
	"archive/tar"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/cloudposse/atmos/pkg/filesystem"
	log "github.com/cloudposse/atmos/pkg/logger" // Charmbracelet structured logger
	"github.com/pkg/errors"
)

var ErrInvalidFilePath = errors.New("invalid file path")

// extractTarball extracts the tarball file from an io.Reader into the destination directory.
func extractTarball(reader io.Reader, extractPath string) error {
	// Call untar function to handle tar extraction.
	return untar(reader, extractPath)
}

// untar extracts a tar archive into the destination directory.
//
// Writes go through an os.Root opened on extractPath rather than plain
// os.MkdirAll/os.Create: SafeJoin only validates the entry name lexically, so
// without os.Root a malicious archive could plant a symlink via one entry
// (e.g. "link" -> "/etc") and have a later entry (e.g. "link/passwd") follow
// it out of extractPath -- os.Root's methods refuse to resolve a path that
// escapes the root, symlinks included.
func untar(reader io.Reader, extractPath string) error {
	tarReader := tar.NewReader(reader)

	if err := os.MkdirAll(extractPath, os.ModePerm); err != nil {
		return fmt.Errorf("failed to create extraction directory %s: %w", extractPath, err)
	}
	root, err := os.OpenRoot(extractPath)
	if err != nil {
		return fmt.Errorf("failed to open extraction root %s: %w", extractPath, err)
	}
	defer root.Close()

	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			log.Error("Error reading tar header", "error", err)
			return err
		}
		if err := processTarHeader(root, header, tarReader, extractPath); err != nil {
			return err
		}
	}

	return nil
}

// processTarHeader processes a tar header and writes the corresponding file to relPath within root.
func processTarHeader(root *os.Root, header *tar.Header, tarReader *tar.Reader, extractPath string) error {
	// SafeJoin validates the entry name (rejects absolute paths, backslashes,
	// and ".." components); the resulting path itself isn't used since writes
	// go through root by relative name instead.
	if _, err := filesystem.SafeJoin(extractPath, header.Name); err != nil {
		return fmt.Errorf("%w: %s", ErrInvalidFilePath, header.Name)
	}
	relPath := filepath.FromSlash(header.Name)
	switch header.Typeflag {
	case tar.TypeDir:
		return createDirectory(root, relPath)
	case tar.TypeReg:
		return createFileFromTar(root, relPath, tarReader, header)
	default:
		log.Warnf("Unsupported file type: %v in %s", header.Typeflag, header.Name)
	}

	return nil
}

// createDirectory creates relPath (and any necessary parents) within root. If the directory already exists, it does nothing.
func createDirectory(root *os.Root, relPath string) error {
	if err := root.MkdirAll(relPath, os.ModePerm); err != nil {
		return fmt.Errorf("error creating directory %s: %w", relPath, err)
	}
	return nil
}

// createFileFromTar writes the contents of a tar file to relPath within root. It also sets the file mode.
func createFileFromTar(root *os.Root, relPath string, tarReader *tar.Reader, header *tar.Header) error {
	err := root.MkdirAll(filepath.Dir(relPath), os.ModePerm)
	if err != nil {
		log.Error("Failed to create parent directory for file", "path", relPath, "error", err)
		return err
	}
	writer, err := root.Create(relPath)
	if err != nil {
		log.Error("Failed to create file", "path", relPath, "error", err)
		return err
	}
	defer writer.Close()
	_, err = io.Copy(writer, tarReader)
	if err != nil {
		log.Error("Failed to write file contents", "path", relPath, "error", err)
		return err
	}
	// Remove setuid/setgid bits for security; standard cross-platform.
	newMode := header.FileInfo().Mode() &^ (os.ModeSetuid | os.ModeSetgid)
	if err := root.Chmod(relPath, newMode); err != nil {
		log.Error("Failed to set file permissions", "path", relPath, "error", err)
	}
	return nil
}
