// Package hashfile computes a deterministic content+filename hash over a set of file paths,
// the shared hashing primitive behind pkg/runner/freshness's checksum.changed fact.
package hashfile

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"sort"

	"github.com/cloudposse/atmos/pkg/perf"
)

// HashFiles computes a deterministic hash over paths: sorted first (so input order never
// affects the result), then filename+content streamed into one sha256 digest (filename is
// included so a rename is detected even when content is unchanged). Returns the full
// hex-encoded digest; callers wanting a short prefix (e.g. cache keys) can slice it.
func HashFiles(paths []string) (string, error) {
	defer perf.Track(nil, "hashfile.HashFiles")()

	sorted := make([]string, len(paths))
	copy(sorted, paths)
	sort.Strings(sorted)

	h := sha256.New()
	for _, p := range sorted {
		data, err := os.ReadFile(p)
		if err != nil {
			return "", err
		}
		if _, err := h.Write([]byte(filepath.Base(p))); err != nil {
			return "", err
		}
		if _, err := h.Write(data); err != nil {
			return "", err
		}
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
