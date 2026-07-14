package oci

import (
	"archive/zip"
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	log "github.com/cloudposse/atmos/pkg/logger" // Charmbracelet structured logger
)

// maxZipArchiveSize bounds how many bytes of a layer blob are buffered into
// memory before extraction. Applied to the raw (still-compressed-by-zip)
// bytes, so it catches an oversized blob before any per-entry check runs.
const maxZipArchiveSize = 512 * 1024 * 1024 // 512 MiB.

// maxZipEntrySize bounds how many bytes a single zip entry may decompress
// to. Module packages are source-code archives (KBs-MBs), so this is
// generous while still bounding a maliciously crafted entry that expands
// far past its declared size.
const maxZipEntrySize = 512 * 1024 * 1024 // 512 MiB.

// extractZip extracts a ZIP archive read from reader into the destination
// directory. Since zip.Reader requires io.ReaderAt plus a known size, the
// archive is buffered in memory first, bounded by maxZipArchiveSize.
func extractZip(reader io.Reader, extractPath string) error {
	data, err := io.ReadAll(io.LimitReader(reader, maxZipArchiveSize+1))
	if err != nil {
		return fmt.Errorf("failed to read zip archive: %w", err)
	}
	if int64(len(data)) > maxZipArchiveSize {
		return fmt.Errorf("%w: archive exceeds %d bytes", errUtils.ErrArchiveTooLarge, maxZipArchiveSize)
	}

	zipReader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		log.Error("Error reading zip archive", "error", err)
		return fmt.Errorf("failed to parse zip archive: %w", err)
	}

	for _, file := range zipReader.File {
		if strings.Contains(file.Name, "..") {
			log.Warn("Skipping potential directory traversal attempt", "filename", file.Name)
			continue
		}
		if err := processZipFile(file, extractPath); err != nil {
			return err
		}
	}

	return nil
}

// processZipFile processes a zip.File entry and writes the corresponding file
// to the destination directory.
func processZipFile(file *zip.File, extractPath string) error {
	// Normalize and clean the extraction base path to remove any redundant separators or ".." sequences.
	cleanExtractPath := filepath.Clean(extractPath)
	// Clean the file path inside the archive to prevent directory traversal attacks.
	cleanFileName := filepath.Clean(file.Name)
	filePath := filepath.Join(cleanExtractPath, cleanFileName)
	// Ensure the target path is within the intended extraction directory.
	if !strings.HasPrefix(filePath, cleanExtractPath) {
		return fmt.Errorf("%w: %s", ErrInvalidFilePath, filePath)
	}

	if file.FileInfo().IsDir() {
		return createDirectory(filePath)
	}

	return createFileFromZip(filePath, file)
}

// createFileFromZip writes the contents of a zip.File to a file at the
// specified path. It also sets the file mode.
func createFileFromZip(filePath string, file *zip.File) error {
	if file.UncompressedSize64 > maxZipEntrySize {
		return fmt.Errorf("%w: %s (declared %d bytes, max %d)", errUtils.ErrArchiveEntryTooLarge, filePath, file.UncompressedSize64, maxZipEntrySize)
	}

	if err := os.MkdirAll(filepath.Dir(filePath), os.ModePerm); err != nil {
		log.Error("Failed to create parent directory for file", "path", filePath, "error", err)
		return fmt.Errorf("failed to create parent directory for %s: %w", filePath, err)
	}

	src, err := file.Open()
	if err != nil {
		log.Error("Failed to open zip entry", "path", filePath, "error", err)
		return fmt.Errorf("failed to open zip entry %s: %w", filePath, err)
	}
	defer src.Close()

	writer, err := os.Create(filePath)
	if err != nil {
		log.Error("Failed to create file", "path", filePath, "error", err)
		return fmt.Errorf("failed to create file %s: %w", filePath, err)
	}
	defer writer.Close()

	// Copy at most maxZipEntrySize+1 bytes: a nil error means the source still
	// had data left at that point, i.e. the actual decompressed content
	// exceeds the declared size (or the declaration was understated/forged).
	_, err = io.CopyN(writer, src, maxZipEntrySize+1)
	if err != nil && !errors.Is(err, io.EOF) {
		log.Error("Failed to write file contents", "path", filePath, "error", err)
		return fmt.Errorf("failed to write file contents to %s: %w", filePath, err)
	}
	if err == nil {
		return fmt.Errorf("%w: %s (exceeded %d bytes during extraction)", errUtils.ErrArchiveEntryTooLarge, filePath, maxZipEntrySize)
	}

	// Remove setuid/setgid bits for security; standard cross-platform.
	newMode := file.Mode() &^ (os.ModeSetuid | os.ModeSetgid)
	// Set permissions using os.Chmod for all platforms.
	if err := os.Chmod(filePath, newMode); err != nil {
		log.Error("Failed to set file permissions", "path", filePath, "error", err)
	}
	return nil
}
