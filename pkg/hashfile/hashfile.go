// Package hashfile computes a deterministic content+filename hash over a set of file paths,
// the shared hashing primitive behind pkg/runner/freshness's checksum.changed fact.
package hashfile

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"io"
	"os"
	"sort"

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
	return hex.EncodeToString(h.Sum(nil)), nil
}

// hashFileContent streams p's content into h as one length-prefixed record, without loading the
// whole file into memory -- large task-runner inputs would otherwise cause a memory spike.
func hashFileContent(h io.Writer, p string) error {
	f, err := os.Open(p)
	if err != nil {
		return err
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return err
	}

	var size [8]byte
	binary.BigEndian.PutUint64(size[:], uint64(info.Size())) //nolint:gosec // file size is never negative.
	if _, err := h.Write(size[:]); err != nil {
		return err
	}
	_, err = io.Copy(h, f)
	return err
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
