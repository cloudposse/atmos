package httpmock

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
)

// archiveFilePermissions is the mode recorded for every file BuildTarGz/BuildZip write. Tests
// registering assets through this package must never execute the resulting fake binary --
// only assert that atmos placed a file at the expected path -- so the exact mode is
// unimportant beyond being a plausible regular-file permission.
const archiveFilePermissions = 0o755

// BuildTarGz builds a minimal gzip-compressed tar archive containing files (archive path ->
// content), suitable for registering as a fake release asset via RegisterReleaseAsset or
// RegisterArchive. The result is a tiny stand-in for a real tool's release tarball: tests
// should assert atmos extracted/placed a file at the expected path, never execute it.
func BuildTarGz(files map[string]string) []byte {
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)

	for name, content := range files {
		header := &tar.Header{
			Name: name,
			Mode: archiveFilePermissions,
			Size: int64(len(content)),
		}
		_ = tw.WriteHeader(header)
		_, _ = tw.Write([]byte(content))
	}

	_ = tw.Close()
	_ = gz.Close()
	return buf.Bytes()
}

// BuildZip builds a minimal zip archive containing files (archive path -> content), suitable
// for registering as a fake release asset via RegisterReleaseAsset. See BuildTarGz for the
// "never execute the fake binary" caveat.
func BuildZip(files map[string]string) []byte {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)

	for name, content := range files {
		fw, err := zw.Create(name)
		if err != nil {
			continue
		}
		_, _ = fw.Write([]byte(content))
	}

	_ = zw.Close()
	return buf.Bytes()
}
