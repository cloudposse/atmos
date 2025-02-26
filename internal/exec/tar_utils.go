package exec

import (
	"archive/tar"
	"io"
	"os"
	"path/filepath"
	"strings"

	log "github.com/charmbracelet/log" // Charmbracelet structured logger
)

// extractTarball extracts the tarball file from an io.Reader into the destination directory
func extractTarball(reader io.Reader, extractPath string) error {

	// Call untar function to handle tar extraction
	return untar(reader, extractPath)
}

// untar extracts a tar archive into the destination directory
func untar(reader io.Reader, destDir string) error {

	tarBallReader := tar.NewReader(reader)

	for {
		header, err := tarBallReader.Next()
		if err != nil {
			if err == io.EOF {
				break
			}
			log.Error("Error reading tar header", "error", err)
			return err
		}

		filename := filepath.Join(destDir, filepath.FromSlash(header.Name))

		if strings.Contains(header.Name, "..") {
			log.Warn("Skipping potential directory traversal attempt", "filename", header.Name)
			continue
		}

		switch header.Typeflag {
		case tar.TypeDir:
			err = os.MkdirAll(filename, os.FileMode(header.Mode))
			if err != nil {
				log.Error("Failed to create directory", "path", filename, "error", err)
				return err
			}

		case tar.TypeReg:
			err := os.MkdirAll(filepath.Dir(filename), os.ModePerm)
			if err != nil {
				log.Error("Failed to create parent directory for file", "path", filename, "error", err)
				return err
			}

			writer, err := os.Create(filename)
			if err != nil {
				log.Error("Failed to create file", "path", filename, "error", err)
				return err
			}

			_, err = io.Copy(writer, tarBallReader)
			if err != nil {
				log.Error("Failed to write file contents", "path", filename, "error", err)
				writer.Close()
				return err
			}

			err = os.Chmod(filename, os.FileMode(header.Mode))
			if err != nil {
				log.Warn("Failed to set file permissions", "path", filename, "error", err)
			}

			err = writer.Close()
			if err != nil {
				log.Error("Failed to close file writer", "path", filename, "error", err)
				return err
			}

		default:
			log.Debug("Skipping unsupported header type in tarball", "filename", header.Name, "type", header.Typeflag)
		}
	}

	return nil
}
