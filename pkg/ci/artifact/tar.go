package artifact

import (
	"archive/tar"
	"bytes"
	"fmt"
	"io"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
)

// CreateTarArchive creates a tar archive from the given file entries.
// Each entry is read and written into the tar with 0o644 permissions.
// Returns the tar bytes or an error.
func CreateTarArchive(files []FileEntry) ([]byte, error) {
	defer perf.Track(nil, "artifact.CreateTarArchive")()

	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)

	for _, entry := range files {
		data, err := io.ReadAll(entry.Data)
		if err != nil {
			return nil, fmt.Errorf("%w: failed to read file %s: %w", errUtils.ErrArtifactUploadFailed, entry.Name, err)
		}

		header := &tar.Header{
			Name: entry.Name,
			Mode: 0o644,
			Size: int64(len(data)),
		}

		if err := tw.WriteHeader(header); err != nil {
			return nil, fmt.Errorf("%w: failed to write tar header for %s: %w", errUtils.ErrArtifactUploadFailed, entry.Name, err)
		}

		if _, err := tw.Write(data); err != nil {
			return nil, fmt.Errorf("%w: failed to write tar data for %s: %w", errUtils.ErrArtifactUploadFailed, entry.Name, err)
		}
	}

	if err := tw.Close(); err != nil {
		return nil, fmt.Errorf("%w: failed to close tar archive: %w", errUtils.ErrArtifactUploadFailed, err)
	}

	return buf.Bytes(), nil
}

// ExtractTarArchive extracts all file entries from a tar archive.
// Returns a slice of FileResult with io.NopCloser-wrapped readers for each entry.
func ExtractTarArchive(data io.Reader) ([]FileResult, error) {
	defer perf.Track(nil, "artifact.ExtractTarArchive")()

	tr := tar.NewReader(data)
	var results []FileResult

	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("%w: failed to read tar entry: %w", errUtils.ErrArtifactDownloadFailed, err)
		}

		fileData, err := io.ReadAll(tr)
		if err != nil {
			return nil, fmt.Errorf("%w: failed to read tar entry %s: %w", errUtils.ErrArtifactDownloadFailed, header.Name, err)
		}

		results = append(results, FileResult{
			Name: header.Name,
			Data: io.NopCloser(bytes.NewReader(fileData)),
			Size: header.Size,
		})
	}

	return results, nil
}
