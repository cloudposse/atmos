// Package archive packs and updates zip/tar archives using the Go standard
// library only, so archive creation behaves identically on macOS, Linux, and
// Windows without depending on the `zip`/`tar` binaries. See
// docs/prd/archive-step.md.
package archive

import (
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
)

// Supported archive formats. Writing tar.bz2 and tar.xz is not yet
// implemented even though both are recognized formats — the Go standard
// library has no xz support and only read-only bzip2; see
// ErrArchiveFormatNotImplemented.
const (
	FormatZip    = "zip"
	FormatTar    = "tar"
	FormatTGZ    = "tgz"
	FormatTarBz2 = "tar.bz2"
	FormatTarXz  = "tar.xz"
)

// DetectFormat resolves the archive format to use. An explicit format always
// wins; otherwise the format is inferred from path's extension.
func DetectFormat(explicit, path string) (string, error) {
	defer perf.Track(nil, "archive.DetectFormat")()

	if explicit != "" {
		normalized := normalizeFormat(explicit)
		if !isKnownFormat(normalized) {
			return "", errUtils.Build(errUtils.ErrArchiveUnknownFormat).
				WithExplanationf("%q is not a supported archive format", explicit).
				WithHint("Use one of: zip, tar, tgz, tar.bz2, tar.xz").
				WithContext("format", explicit).
				Err()
		}
		return normalized, nil
	}

	inferred, ok := inferFormatFromPath(path)
	if !ok {
		return "", errUtils.Build(errUtils.ErrArchiveUnknownFormat).
			WithExplanationf("could not infer an archive format from %q", path).
			WithHint("Set format: explicitly (zip, tar, tgz, tar.bz2, tar.xz)").
			WithContext("path", path).
			Err()
	}
	return inferred, nil
}

// updatable reports whether entries in an archive of this format can be
// added/replaced without rewriting the whole archive stream. Zip entries are
// compressed independently; uncompressed tar has no compression at all. The
// whole-stream-compressed formats (tgz, tar.bz2, tar.xz) are not.
func updatable(format string) bool {
	return format == FormatZip || format == FormatTar
}

func normalizeFormat(f string) string {
	return strings.ToLower(strings.TrimSpace(f))
}

func isKnownFormat(f string) bool {
	switch f {
	case FormatZip, FormatTar, FormatTGZ, FormatTarBz2, FormatTarXz:
		return true
	default:
		return false
	}
}

func inferFormatFromPath(p string) (string, bool) {
	lower := strings.ToLower(p)
	switch {
	case strings.HasSuffix(lower, ".tar.gz"), strings.HasSuffix(lower, ".tgz"):
		return FormatTGZ, true
	case strings.HasSuffix(lower, ".tar.bz2"), strings.HasSuffix(lower, ".tbz2"):
		return FormatTarBz2, true
	case strings.HasSuffix(lower, ".tar.xz"), strings.HasSuffix(lower, ".txz"):
		return FormatTarXz, true
	case strings.HasSuffix(lower, ".tar"):
		return FormatTar, true
	case strings.HasSuffix(lower, ".zip"):
		return FormatZip, true
	default:
		return "", false
	}
}
