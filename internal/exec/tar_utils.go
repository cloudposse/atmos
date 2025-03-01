package exec

import (
	"archive/tar"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	log "github.com/charmbracelet/log" // Charmbracelet structured logger
	"github.com/pkg/errors"
)

var ErrInvalidFilePath = errors.New("invalid file path")

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
	targetPath := filepath.Join(cleanExtractPath, cleanHeaderName)
	// Ensure the target path is within the intended extraction directory.
	// The trailing `os.PathSeparator` prevents bypasses where an attacker might use a prefix match trick.
	if !strings.HasPrefix(targetPath, cleanExtractPath+string(os.PathSeparator)) {
		return fmt.Errorf("%w: %s", ErrInvalidFilePath, targetPath)
	}
	switch header.Typeflag {
	case tar.TypeDir:
		return createDirectory(targetPath)
	case tar.TypeReg:
		return writeFile(targetPath, tarReader, header.FileInfo().Mode())
	default:
		log.Warnf("Unsupported file type: %v in %s", header.Typeflag, header.Name)
	}

	return nil
}

// createDirectory creates a directory at the specified path. If the directory already exists, it does nothing.
func createDirectory(targetPath string) error {
	if err := os.MkdirAll(targetPath, os.ModePerm); err != nil {
		return fmt.Errorf("error creating directory %s: %w", targetPath, err)
	}
	return nil
}

// writeFile writes the contents of a tar file to a file at the specified path. It also sets the file mode.
func writeFile(targetPath string, tarReader *tar.Reader, fileMode os.FileMode) error {
	file, err := os.OpenFile(targetPath, os.O_CREATE|os.O_RDWR, fileMode)
	if err != nil {
		return fmt.Errorf("error creating file %s: %w", targetPath, err)
	}
	defer file.Close()

	if _, err := io.Copy(file, tarReader); err != nil {
		return fmt.Errorf("error writing to file %s: %w", targetPath, err)
	}

	return nil
}
