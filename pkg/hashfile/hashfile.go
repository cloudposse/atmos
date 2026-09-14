// Package hashfile computes a deterministic content+filename hash over a set of file paths,
// the shared hashing primitive behind pkg/runner/freshness's checksum.changed fact.
package hashfile

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"sort"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
)

// HashFiles computes a deterministic hash over paths: sorted first (so input order never
// affects the result), then path+content streamed into one sha256 digest (the path, not just its
// base name, is included so both a rename and a move between directories that share a filename
// are detected even when content is unchanged). Each record is length-prefixed (see writeRecord)
// so no concatenation of two records can ever collide with a different pair of path/content
// values. Returns the full hex-encoded digest; callers wanting a short prefix (e.g. cache keys)
// can slice it.
func HashFiles(paths []string) (string, error) {
	defer perf.Track(nil, "hashfile.HashFiles")()

	return HashFilesAndRecords(paths, nil)
}

// HashFilesAndRecords extends HashFiles with an additional set of opaque string records (e.g.
// "key=value" facts that should invalidate the digest when they change but have no file backing
// them); records are sorted before hashing, so callers never need to pre-sort them and the result
// is stable regardless of the order records are supplied in. Passing a nil or empty records slice
// produces a digest byte-identical to HashFiles(paths).
func HashFilesAndRecords(paths []string, records []string) (string, error) {
	defer perf.Track(nil, "hashfile.HashFilesAndRecords")()

	sorted := make([]string, len(paths))
	copy(sorted, paths)
	sort.Strings(sorted)

	h := sha256.New()
	for _, p := range sorted {
		if err := writeRecord(h, []byte(p)); err != nil {
			return "", err
		}
		if err := hashFileContent(h, p); err != nil {
			return "", err
		}
	}
	if err := writeSortedRecords(h, records); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// HashNamedFiles computes a deterministic hash over a set of files keyed by a caller-supplied
// name rather than their filesystem path, so the resulting digest is independent of where the
// files physically live (e.g. two checkouts of the same component at different absolute paths
// produce identical digests); named maps each logical name to the path whose content should be
// read for it; names are sorted before hashing so map iteration order never affects the result.
// The records parameter behaves exactly as in HashFilesAndRecords.
func HashNamedFiles(named map[string]string, records []string) (string, error) {
	defer perf.Track(nil, "hashfile.HashNamedFiles")()

	names := make([]string, 0, len(named))
	for name := range named {
		names = append(names, name)
	}
	sort.Strings(names)

	h := sha256.New()
	for _, name := range names {
		if err := writeRecord(h, []byte(name)); err != nil {
			return "", err
		}
		if err := hashFileContent(h, named[name]); err != nil {
			return "", err
		}
	}
	if err := writeSortedRecords(h, records); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// writeSortedRecords writes each of records into h as a length-prefixed record (see
// writeRecord), after sorting them so the result never depends on the order records were
// supplied in.
func writeSortedRecords(h io.Writer, records []string) error {
	sorted := make([]string, len(records))
	copy(sorted, records)
	sort.Strings(sorted)

	for _, r := range sorted {
		if err := writeRecord(h, []byte(r)); err != nil {
			return err
		}
	}
	return nil
}

// errWrapFormat wraps a sentinel and the underlying cause around the file path they concern.
const errWrapFormat = "%w: %s: %w"

// hashFileContent streams p's content into h as one length-prefixed record, without loading the
// whole file into memory -- large task-runner inputs would otherwise cause a memory spike.
func hashFileContent(h io.Writer, p string) error {
	f, err := os.Open(p)
	if err != nil {
		return fmt.Errorf(errWrapFormat, errUtils.ErrOpenFile, p, err)
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return fmt.Errorf(errWrapFormat, errUtils.ErrStatFile, p, err)
	}

	var size [8]byte
	binary.BigEndian.PutUint64(size[:], uint64(info.Size())) //nolint:gosec // file size is never negative.
	if _, err := h.Write(size[:]); err != nil {
		return fmt.Errorf(errWrapFormat, errUtils.ErrCopyFile, p, err)
	}
	if _, err := io.Copy(h, f); err != nil {
		return fmt.Errorf(errWrapFormat, errUtils.ErrCopyFile, p, err)
	}
	return nil
}

// writeRecord writes data into h prefixed with its own length (a fixed 8-byte big-endian
// uint64), so two records can never be ambiguous when concatenated -- unlike writing raw
// path/content bytes back-to-back with no boundary, where e.g. path "a" + content "bc" hashes
// identically to path "ab" + content "c".
func writeRecord(h io.Writer, data []byte) error {
	var length [8]byte
	binary.BigEndian.PutUint64(length[:], uint64(len(data)))
	if _, err := h.Write(length[:]); err != nil {
		return err
	}
	_, err := h.Write(data)
	return err
}
