package toolchain

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"strings"

	log "github.com/charmbracelet/log"
	"github.com/gabriel-vasile/mimetype"

	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/ui"
	"github.com/cloudposse/atmos/toolchain/registry"
)

// simpleExtract is a robust extraction method using magic file type detection.
func (i *Installer) simpleExtract(assetPath, binaryPath string, tool *registry.Tool) error {
	// Detect file type using magic bytes.
	mime, err := mimetype.DetectFile(assetPath)
	if err != nil {
		return fmt.Errorf("%w: failed to detect file type: %w", ErrFileOperation, err)
	}

	log.Debug("Detected file type", "mime", mime.String(), "extension", mime.Extension())

	return i.extractByMimeType(assetPath, binaryPath, tool, mime)
}

// extractByMimeType dispatches extraction based on detected MIME type.
func (i *Installer) extractByMimeType(assetPath, binaryPath string, tool *registry.Tool, mime *mimetype.MIME) error {
	switch {
	case mime.Is("application/zip"):
		return i.extractZip(assetPath, binaryPath, tool)
	case isGzipMime(mime):
		return i.extractGzipOrTarGz(assetPath, binaryPath, tool, mime)
	case mime.Is("application/x-tar"):
		return i.extractTarGz(assetPath, binaryPath, tool)
	case isBinaryMime(mime):
		return i.copyFile(assetPath, binaryPath)
	default:
		return i.extractByExtension(assetPath, binaryPath, tool)
	}
}

// isGzipMime checks if the MIME type is a gzip variant.
func isGzipMime(mime *mimetype.MIME) bool {
	return mime.Is("application/x-gzip") || mime.Is("application/gzip")
}

// isBinaryMime checks if the MIME type indicates a binary executable.
func isBinaryMime(mime *mimetype.MIME) bool {
	return mime.Is("application/octet-stream") || mime.Is("application/x-executable")
}

// extractGzipOrTarGz handles gzip files, determining if they are tar.gz archives.
func (i *Installer) extractGzipOrTarGz(assetPath, binaryPath string, tool *registry.Tool, mime *mimetype.MIME) error {
	// Check if it's a tar.gz (by extension or by magic).
	if isTarGzFile(assetPath, mime) {
		return i.extractTarGz(assetPath, binaryPath, tool)
	}
	// Otherwise, treat as a single gzip-compressed binary.
	return i.extractGzip(assetPath, binaryPath)
}

// isTarGzFile checks if a gzip file is a tar.gz archive.
func isTarGzFile(assetPath string, mime *mimetype.MIME) bool {
	return strings.HasSuffix(assetPath, ".tar.gz") ||
		strings.HasSuffix(assetPath, ".tgz") ||
		mime.Is("application/x-tar")
}

// extractByExtension handles fallback extraction based on file extension.
func (i *Installer) extractByExtension(assetPath, binaryPath string, tool *registry.Tool) error {
	if strings.HasSuffix(assetPath, ".zip") {
		return i.extractZip(assetPath, binaryPath, tool)
	}
	if strings.HasSuffix(assetPath, ".tar.gz") || strings.HasSuffix(assetPath, ".tgz") {
		return i.extractTarGz(assetPath, binaryPath, tool)
	}
	if strings.HasSuffix(assetPath, ".gz") {
		return i.extractGzip(assetPath, binaryPath)
	}
	log.Debug("Unknown file type, copying as binary", filenameKey, filepath.Base(assetPath))
	return i.copyFile(assetPath, binaryPath)
}

// extractZip extracts a ZIP file.
func (i *Installer) extractZip(zipPath, binaryPath string, tool *registry.Tool) error {
	log.Debug("Extracting ZIP archive", filenameKey, filepath.Base(zipPath))

	tempDir, err := os.MkdirTemp("", "installer-extract-")
	if err != nil {
		return fmt.Errorf("%w: failed to create temp dir: %w", ErrFileOperation, err)
	}
	defer os.RemoveAll(tempDir)

	err = Unzip(zipPath, tempDir)
	if err != nil {
		return fmt.Errorf("%w: failed to extract ZIP: %w", ErrFileOperation, err)
	}

	binaryName := resolveBinaryName(tool)
	found, err := findBinaryInDir(tempDir, binaryName)
	if err != nil {
		return err
	}

	return installExtractedBinary(found, binaryPath)
}

// resolveBinaryName determines the binary name from tool metadata.
func resolveBinaryName(tool *registry.Tool) string {
	if tool.BinaryName != "" {
		return tool.BinaryName
	}
	if tool.Name != "" {
		return tool.Name
	}
	return tool.RepoName
}

// findBinaryInDir searches for a binary in a directory recursively.
func findBinaryInDir(dir, binaryName string) (string, error) {
	var found string
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.Mode().IsRegular() && (info.Name() == binaryName || info.Name() == binaryName+".exe") {
			found = path
			return filepath.SkipDir
		}
		return nil
	})
	if err != nil {
		return "", fmt.Errorf("%w: failed to search extracted files: %w", ErrFileOperation, err)
	}
	if found == "" {
		return "", fmt.Errorf("%w: binary %s not found in extracted archive", ErrToolNotFound, binaryName)
	}
	return found, nil
}

// installExtractedBinary moves an extracted binary to its final location.
func installExtractedBinary(src, dst string) error {
	dir := filepath.Dir(dst)
	if err := os.MkdirAll(dir, defaultMkdirPermissions); err != nil {
		return fmt.Errorf("%w: failed to create destination directory: %w", ErrFileOperation, err)
	}

	if err := MoveFile(src, dst); err != nil {
		return fmt.Errorf("%w: failed to move extracted binary: %w", ErrFileOperation, err)
	}

	return nil
}

// Unzip extracts a zip archive to a destination directory.
// Works on Windows, macOS, and Linux.
func Unzip(src, dest string) error {
	defer perf.Track(nil, "toolchain.Unzip")()

	const maxDecompressedSize = maxDecompressedSizeMB * 1024 * 1024 // 3000MB limit per file.

	r, err := zip.OpenReader(src)
	if err != nil {
		return err
	}
	defer r.Close()

	for _, f := range r.File {
		if err := extractZipFile(f, dest, maxDecompressedSize); err != nil {
			return err
		}
	}
	return nil
}

func extractZipFile(f *zip.File, dest string, maxSize int64) error {
	fpath, err := validatePath(f.Name, dest)
	if err != nil {
		return err
	}

	if f.FileInfo().IsDir() {
		return os.MkdirAll(fpath, os.ModePerm)
	}

	if err := os.MkdirAll(filepath.Dir(fpath), os.ModePerm); err != nil {
		return err
	}

	return copyFileContents(f, fpath, maxSize)
}

func validatePath(name, dest string) (string, error) {
	fpath := filepath.Join(dest, name)
	if !strings.HasPrefix(fpath, filepath.Clean(dest)+string(os.PathSeparator)) {
		return "", fmt.Errorf("%w: illegal file path: %s", ErrFileOperation, name)
	}
	return fpath, nil
}

func copyFileContents(f *zip.File, fpath string, maxSize int64) error {
	rc, err := f.Open()
	if err != nil {
		return err
	}
	defer rc.Close()

	outFile, err := os.OpenFile(fpath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.Mode())
	if err != nil {
		return err
	}
	defer outFile.Close()

	return copyWithLimit(rc, outFile, f.Name, maxSize)
}

func copyWithLimit(src io.Reader, dst io.Writer, name string, maxSize int64) error {
	var totalBytes int64
	buf := make([]byte, bufferSizeBytes)

	for {
		n, err := src.Read(buf)
		totalBytes += int64(n)

		if totalBytes > maxSize {
			return fmt.Errorf("%w: decompressed size of %s exceeds limit: %d > %d", ErrFileOperation, name, totalBytes, maxSize)
		}

		if n > 0 {
			if _, err := dst.Write(buf[:n]); err != nil {
				return err
			}
		}

		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
	}
	return nil
}

// ExtractTarGz extracts a .tar.gz file to the given destination directory.
func ExtractTarGz(src, dest string) error {
	defer perf.Track(nil, "toolchain.ExtractTarGz")()

	f, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("%w: failed to open source file: %w", ErrFileOperation, err)
	}
	defer f.Close()

	gzr, err := gzip.NewReader(f)
	if err != nil {
		return fmt.Errorf("%w: failed to create gzip reader: %w", ErrFileOperation, err)
	}
	defer gzr.Close()

	tr := tar.NewReader(gzr)
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("%w: error reading tar: %w", ErrFileOperation, err)
		}

		if err := extractEntry(tr, header, dest); err != nil {
			return err
		}
	}
	return nil
}

func extractEntry(tr *tar.Reader, header *tar.Header, dest string) error {
	//nolint:gosec // G305: Path is validated by isSafePath check on next line.
	targetPath := filepath.Join(dest, header.Name)
	if !isSafePath(targetPath, dest) {
		return fmt.Errorf("%w: illegal file path: %s", ErrFileOperation, header.Name)
	}

	switch header.Typeflag {
	case tar.TypeDir:
		return extractDir(targetPath, header)
	case tar.TypeReg:
		return extractFile(tr, targetPath, header)
	default:
		_ = ui.Warningf("Skipping unknown type: %s", header.Name)
		return nil
	}
}

func isSafePath(path, dest string) bool {
	cleanDest := filepath.Clean(dest) + string(os.PathSeparator)
	return strings.HasPrefix(filepath.Clean(path), cleanDest)
}

func extractDir(path string, header *tar.Header) error {
	// Validate header.Mode.
	if header.Mode < 0 || header.Mode > maxUnixPermissions {
		return fmt.Errorf("%w: invalid mode %d for %s: must be between 0 and %o", ErrFileOperation, header.Mode, path, maxUnixPermissions)
	}

	// Safe conversion to os.FileMode.
	return os.MkdirAll(path, os.FileMode(header.Mode))
}

func extractFile(tr *tar.Reader, path string, header *tar.Header) error {
	if err := os.MkdirAll(filepath.Dir(path), defaultMkdirPermissions); err != nil {
		return fmt.Errorf("%w: failed to create parent directory: %w", ErrFileOperation, err)
	}
	// Validate header.Mode is within uint32 range.
	if header.Mode < 0 || header.Mode > math.MaxUint32 {
		return fmt.Errorf("%w: header.Mode out of uint32 range: %d", ErrFileOperation, header.Mode)
	}

	outFile, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.FileMode(header.Mode))
	if err != nil {
		return fmt.Errorf("%w: failed to create file: %w", ErrFileOperation, err)
	}
	defer outFile.Close()

	if _, err := io.Copy(outFile, tr); err != nil {
		return fmt.Errorf("%w: failed to write file: %w", ErrFileOperation, err)
	}
	return nil
}

// extractTarGz extracts a tar.gz file.
func (i *Installer) extractTarGz(tarPath, binaryPath string, tool *registry.Tool) error {
	log.Debug("Extracting tar.gz archive", filenameKey, filepath.Base(tarPath))

	tempDir, err := os.MkdirTemp("", "installer-extract-")
	if err != nil {
		return fmt.Errorf("%w: failed to create temp dir: %w", ErrFileOperation, err)
	}
	defer os.RemoveAll(tempDir)

	if err = ExtractTarGz(tarPath, tempDir); err != nil {
		return fmt.Errorf("%w: failed to extract tar.gz: %w", ErrFileOperation, err)
	}

	binaryName := resolveBinaryName(tool)
	found, err := findBinaryInDir(tempDir, binaryName)
	if err != nil {
		return err
	}

	return installExtractedBinary(found, binaryPath)
}

// MoveFile tries os.Rename, but if that fails due to cross-device link,
// it falls back to a copy+remove.
func MoveFile(src, dst string) error {
	defer perf.Track(nil, "toolchain.MoveFile")()

	// Ensure target dir exists.
	if err := os.MkdirAll(filepath.Dir(dst), defaultMkdirPermissions); err != nil {
		return fmt.Errorf("%w: failed to create target dir: %w", ErrFileOperation, err)
	}

	if err := os.Rename(src, dst); err != nil {
		if err := copyFileFallback(src, dst); err != nil {
			return fmt.Errorf("%w: failed to copy during move fallback: %w", ErrFileOperation, err)
		}
		if err := os.Remove(src); err != nil {
			return fmt.Errorf("%w: failed to remove source after copy: %w", ErrFileOperation, err)
		}
		return nil
	}
	return nil
}

// copyFile copies a file from src to dst.
func copyFile(src, dst string) error {
	return copyFileFallback(src, dst)
}

// copyFileFallback copies a file when rename fails.
func copyFileFallback(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer func() {
		_ = out.Close()
	}()

	if _, err = io.Copy(out, in); err != nil {
		return err
	}
	return out.Sync()
}

// extractGzip decompresses a single gzip-compressed binary.
func (i *Installer) extractGzip(gzPath, binaryPath string) error {
	log.Debug("Decompressing gzip binary", filenameKey, filepath.Base(gzPath))

	in, err := os.Open(gzPath)
	if err != nil {
		return fmt.Errorf("%w: failed to open gzip file: %w", ErrFileOperation, err)
	}
	defer in.Close()

	gzr, err := gzip.NewReader(in)
	if err != nil {
		return fmt.Errorf("%w: failed to create gzip reader: %w", ErrFileOperation, err)
	}
	defer gzr.Close()

	out, err := os.Create(binaryPath)
	if err != nil {
		return fmt.Errorf("%w: failed to create output file: %w", ErrFileOperation, err)
	}
	defer out.Close()

	//nolint:gosec // G110: Single binary extraction from trusted GitHub releases, size limited by GitHub's release size limits.
	if _, err := io.Copy(out, gzr); err != nil {
		return fmt.Errorf("%w: failed to decompress gzip: %w", ErrFileOperation, err)
	}

	return nil
}

// copyFile copies a file.
func (i *Installer) copyFile(src, dst string) error {
	log.Debug("Copying binary", "src", filepath.Base(src), "dst", filepath.Base(dst))

	source, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("%w: failed to open source file: %w", ErrFileOperation, err)
	}
	defer source.Close()

	destination, err := os.Create(dst)
	if err != nil {
		return fmt.Errorf("%w: failed to create destination file: %w", ErrFileOperation, err)
	}
	defer destination.Close()

	_, err = io.Copy(destination, source)
	if err != nil {
		return fmt.Errorf("%w: failed to copy file: %w", ErrFileOperation, err)
	}

	return nil
}
