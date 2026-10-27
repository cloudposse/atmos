package filesystem

import (
	"net/url"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/cloudposse/atmos/pkg/perf"
)

// SanitizeFileName replaces invalid characters and query strings with underscores for Windows.
func SanitizeFileName(uri string) string {
	defer perf.Track(nil, "filesystem.SanitizeFileName")()

	// Parse the URI to handle paths and query strings properly
	parsed, err := url.Parse(uri)
	if err != nil {
		// Fallback to basic filepath.Base if URI parsing fails
		return filepath.Base(uri)
	}

	// Extract the path component of the URI
	base := filepath.Base(parsed.Path)

	// This logic applies only to Windows
	if runtime.GOOS != "windows" {
		return base
	}

	// Replace invalid characters for Windows
	base = strings.Map(func(r rune) rune {
		switch r {
		case '\\', '/', ':', '*', '?', '"', '<', '>', '|':
			return '_'
		default:
			return r
		}
	}, base)

	return base
}
