package oci

import (
	"archive/tar"
	"bytes"
	"errors"
	"io"
	"strings"

	v1 "github.com/google/go-containerregistry/pkg/v1"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/filesystem"
	"github.com/cloudposse/atmos/pkg/perf"
)

const (
	archiveFileMode       = 0o644
	archiveExecutableMode = 0o755
)

// ReadVerifiedBlob enforces a byte limit and checks both size and digest before
// exposing data. It works identically for registry and offline-layout blobs.
func ReadVerifiedBlob(reader io.Reader, expected *v1.Descriptor, limit int64) ([]byte, error) {
	defer perf.Track(nil, "oci.ReadVerifiedBlob")()

	if expected == nil || limit <= 0 || expected.Size < 0 || expected.Size > limit {
		return nil, errUtils.ErrOCIIntegrity
	}
	raw, err := io.ReadAll(io.LimitReader(reader, limit+1))
	if err != nil {
		return nil, errors.Join(errUtils.ErrOCIIntegrity, err)
	}
	digest, size, err := v1.SHA256(bytes.NewReader(raw))
	if err != nil {
		return nil, errors.Join(errUtils.ErrOCIIntegrity, err)
	}
	if size != expected.Size || digest != expected.Digest {
		return nil, errUtils.ErrOCIIntegrity
	}
	return raw, nil
}

// ExtractTar validates a bounded, already verified tar payload before using the
// shared confined extractor. Links, duplicate names, special files, and modes
// other than ordinary files/directories are rejected before any write.
func ExtractTar(raw []byte, directory string) error {
	defer perf.Track(nil, "oci.ExtractTar")()

	reader := tar.NewReader(bytes.NewReader(raw))
	seen := make(map[string]bool)
	var size int64
	for {
		header, err := reader.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return errors.Join(errUtils.ErrOCIIntegrity, err)
		}
		key := strings.ToLower(strings.TrimSuffix(header.Name, "/"))
		if err := validateTarHeader(directory, header, seen[key]); err != nil {
			return err
		}
		seen[key] = true
		size += header.Size
		if size > int64(len(raw)) {
			return errUtils.ErrOCIIntegrity
		}
	}
	return extractTarball(bytes.NewReader(raw), directory)
}

func validateTarHeader(directory string, header *tar.Header, duplicate bool) error {
	if _, err := filesystem.SafeJoin(directory, header.Name); err != nil {
		return errors.Join(errUtils.ErrOCIIntegrity, err)
	}
	if duplicate || (header.Typeflag != tar.TypeReg && header.Typeflag != tar.TypeDir) {
		return errUtils.ErrOCIIntegrity
	}
	if header.Mode != archiveFileMode && header.Mode != archiveExecutableMode {
		return errUtils.ErrOCIIntegrity
	}
	return nil
}
