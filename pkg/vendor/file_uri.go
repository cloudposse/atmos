package vendor

import (
	"net/url"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/cloudposse/atmos/pkg/perf"
)

// ResolveFileURIPath converts a "file://" URI into a local filesystem path, preserving the
// URI's root instead of trimming it down to a relative path. `file:///tmp/source` resolves to
// the absolute path "/tmp/source" (not the relative "tmp/source"); on Windows, a drive-letter
// path like `file:///C:/foo` resolves to `C:\foo`, and a UNC path like `file://host/share/foo`
// resolves to `\\host\share\foo`. Relative forms such as `file://./source` and
// `file://../source` stay relative for the caller to resolve against its configuration directory.
// Returns ok=false when uri does not use the "file" scheme.
func ResolveFileURIPath(uri string) (path string, ok bool) {
	defer perf.Track(nil, "vendor.ResolveFileURIPath")()

	parsedURL, err := url.Parse(uri)
	if err != nil || parsedURL.Scheme != "file" {
		return "", false
	}

	switch parsedURL.Host {
	case ".", "..":
		return filepath.Clean(filepath.Join(parsedURL.Host, filepath.FromSlash(strings.TrimPrefix(parsedURL.Path, "/")))), true
	}

	p := filepath.FromSlash(parsedURL.Path)
	if runtime.GOOS == "windows" {
		if parsedURL.Host != "" && !strings.EqualFold(parsedURL.Host, "localhost") {
			// UNC path: file://host/share/foo -> \\host\share\foo.
			return filepath.Clean(`\\` + parsedURL.Host + p), true
		}
		// Strip the spurious leading separator before a drive letter: /C:/foo -> C:\foo.
		if len(p) >= 3 && p[0] == filepath.Separator && p[2] == ':' {
			return filepath.Clean(p[1:]), true
		}
	}
	return filepath.Clean(p), true
}
