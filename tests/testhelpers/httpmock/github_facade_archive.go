package httpmock

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"fmt"
	"sort"
)

// archiveFilePermissions is the mode recorded for every file BuildTarGz/BuildZip write. Tests
// registering assets through this package must never execute the resulting fake binary --
// only assert that atmos placed a file at the expected path -- so the exact mode is
// unimportant beyond being a plausible regular-file permission.
const archiveFilePermissions = 0o755

// sortedArchiveNames returns the keys of files in sorted order so BuildTarGz/BuildZip write
// entries deterministically. Go's map iteration order is randomized, so writing entries in
// map order would make byte-for-byte comparisons of otherwise-identical archives flaky.
func sortedArchiveNames(files map[string]string) []string {
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// BuildTarGz builds a minimal gzip-compressed tar archive containing files (archive path ->
// content), suitable for registering as a fake release asset via RegisterReleaseAsset or
// RegisterArchive. The result is a tiny stand-in for a real tool's release tarball: tests
// should assert atmos extracted/placed a file at the expected path, never execute it. Entries
// are written in sorted name order so identical inputs always produce identical archive bytes.
func BuildTarGz(files map[string]string) ([]byte, error) {
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)

	for _, name := range sortedArchiveNames(files) {
		content := files[name]
		header := &tar.Header{
			Name: name,
			Mode: archiveFilePermissions,
			Size: int64(len(content)),
		}
		if err := tw.WriteHeader(header); err != nil {
			return nil, fmt.Errorf("write tar header %q: %w", name, err)
		}
		if _, err := tw.Write([]byte(content)); err != nil {
			return nil, fmt.Errorf("write tar content %q: %w", name, err)
		}
	}

	if err := tw.Close(); err != nil {
		return nil, fmt.Errorf("close tar writer: %w", err)
	}
	if err := gz.Close(); err != nil {
		return nil, fmt.Errorf("close gzip writer: %w", err)
	}
	return buf.Bytes(), nil
}

// BuildZip builds a minimal zip archive containing files (archive path -> content), suitable
// for registering as a fake release asset via RegisterReleaseAsset. See BuildTarGz for the
// "never execute the fake binary" caveat and the sorted-write-order determinism rationale.
func BuildZip(files map[string]string) ([]byte, error) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)

	for _, name := range sortedArchiveNames(files) {
		content := files[name]
		fw, err := zw.Create(name)
		if err != nil {
			return nil, fmt.Errorf("create zip entry %q: %w", name, err)
		}
		if _, err := fw.Write([]byte(content)); err != nil {
			return nil, fmt.Errorf("write zip entry %q: %w", name, err)
		}
	}

	if err := zw.Close(); err != nil {
		return nil, fmt.Errorf("close zip writer: %w", err)
	}
	return buf.Bytes(), nil
}
