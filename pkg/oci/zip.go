package oci

import (
	"archive/zip"
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/filesystem"
	log "github.com/cloudposse/atmos/pkg/logger" // Charmbracelet structured logger
)

// maxZipArchiveSize bounds how many bytes of a layer blob are buffered into
// memory before extraction. Applied to the raw (still-compressed-by-zip)
// bytes, so it catches an oversized blob before any per-entry check runs.
const maxZipArchiveSize = 512 * 1024 * 1024 // 512 MiB.

// maxZipEntrySize bounds how many bytes a single zip entry may decompress
// to. Module packages are source-code archives (KBs-MBs), so this is
// generous while still bounding a maliciously crafted entry that expands
// far past its declared size. A var, not a const, so tests can shrink it
// temporarily instead of writing hundreds of megabytes of fixture data.
var maxZipEntrySize uint64 = 512 * 1024 * 1024 // 512 MiB.

// extractZip extracts a ZIP archive read from reader into the destination
// directory. Since zip.Reader requires io.ReaderAt plus a known size, the
// archive is buffered in memory first, bounded by maxZipArchiveSize.
//
// Writes go through an os.Root opened on extractPath rather than plain
// os.MkdirAll/os.Create: SafeJoin only validates the entry name lexically, so
// without os.Root a malicious archive could plant a symlink via one entry
// (e.g. "link" -> "/etc") and have a later entry (e.g. "link/passwd") follow
// it out of extractPath -- os.Root's methods refuse to resolve a path that
// escapes the root, symlinks included.
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

	if err := os.MkdirAll(extractPath, os.ModePerm); err != nil {
		return fmt.Errorf("failed to create extraction directory %s: %w", extractPath, err)
	}
	root, err := os.OpenRoot(extractPath)
	if err != nil {
		return fmt.Errorf("failed to open extraction root %s: %w", extractPath, err)
	}
	defer root.Close()

	for _, file := range zipReader.File {
		if err := processZipFile(root, file, extractPath); err != nil {
			return err
		}
	}

	return nil
}

// processZipFile processes a zip.File entry and writes the corresponding file
// to the destination directory via root.
func processZipFile(root *os.Root, file *zip.File, extractPath string) error {
	// SafeJoin validates the entry name (rejects absolute paths, backslashes,
	// and ".." components); the resulting path itself isn't used since writes
	// go through root by relative name instead.
	if _, err := filesystem.SafeJoin(extractPath, file.Name); err != nil {
		return fmt.Errorf("%w: %s", ErrInvalidFilePath, file.Name)
	}
	relPath := filepath.FromSlash(file.Name)

	if file.FileInfo().IsDir() {
		return createDirectory(root, relPath)
	}

	return createFileFromZip(root, relPath, file)
}

// createFileFromZip writes the contents of a zip.File to relPath within root.
// It also sets the file mode.
func createFileFromZip(root *os.Root, relPath string, file *zip.File) error {
	if file.UncompressedSize64 > maxZipEntrySize {
		return fmt.Errorf("%w: %s (declared %d bytes, max %d)", errUtils.ErrArchiveEntryTooLarge, relPath, file.UncompressedSize64, maxZipEntrySize)
	}

	if err := root.MkdirAll(filepath.Dir(relPath), os.ModePerm); err != nil {
		log.Error("Failed to create parent directory for file", "path", relPath, "error", err)
		return fmt.Errorf("failed to create parent directory for %s: %w", relPath, err)
	}

	src, err := file.Open()
	if err != nil {
		log.Error("Failed to open zip entry", "path", relPath, "error", err)
		return fmt.Errorf("failed to open zip entry %s: %w", relPath, err)
	}
	defer src.Close()

	writer, err := root.Create(relPath)
	if err != nil {
		log.Error("Failed to create file", "path", relPath, "error", err)
		return fmt.Errorf("failed to create file %s: %w", relPath, err)
	}
	defer writer.Close()

	// Copy at most maxZipEntrySize+1 bytes: a nil error means the source still
	// had data left at that point, i.e. the actual decompressed content
	// exceeds the declared size (or the declaration was understated/forged).
	_, err = io.CopyN(writer, src, int64(maxZipEntrySize)+1) //nolint:gosec // G115: maxZipEntrySize is an internal, code-controlled limit (default 512MiB, only ever shrunk by tests), never derived from untrusted archive input, so it never approaches int64's range.
	if err != nil && !errors.Is(err, io.EOF) {
		log.Error("Failed to write file contents", "path", relPath, "error", err)
		return fmt.Errorf("failed to write file contents to %s: %w", relPath, err)
	}
	if err == nil {
		return fmt.Errorf("%w: %s (exceeded %d bytes during extraction)", errUtils.ErrArchiveEntryTooLarge, relPath, maxZipEntrySize)
	}

	// Remove setuid/setgid bits for security; standard cross-platform.
	newMode := file.Mode() &^ (os.ModeSetuid | os.ModeSetgid)
	if err := root.Chmod(relPath, newMode); err != nil {
		log.Error("Failed to set file permissions", "path", relPath, "error", err)
	}
	return nil
}
