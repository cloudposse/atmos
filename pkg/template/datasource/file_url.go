package datasource

import (
	"fmt"
	"net/url"
	"path/filepath"
	"runtime"
	"strings"
)

const (
	forwardSlash  = "/"
	uncPathPrefix = "//"
)

// toFileScheme converts a local filesystem path into a "file://" URL in a way
// that won't confuse Gomplate on Windows or Linux.
//
// On Windows, e.g. localPath = "D:\Temp\foo.json" => "file://D:/Temp/foo.json"
// On Linux,  e.g. localPath = "/tmp/foo.json"    => "file:///tmp/foo.json"
func toFileScheme(localPath string) string {
	pathSlashed := filepath.ToSlash(localPath)

	if runtime.GOOS == "windows" {
		// Handle UNC paths: \\server\share becomes //server/share after ToSlash
		// Per RFC 8089, should become file://server/share (server as authority)
		if strings.HasPrefix(pathSlashed, uncPathPrefix) {
			// Remove both leading slashes for UNC paths
			pathSlashed = strings.TrimPrefix(pathSlashed, uncPathPrefix)
			return "file://" + pathSlashed // e.g. "file://server/share/file.txt"
		}

		// Regular Windows paths: D:\path becomes D:/path after ToSlash
		// Should become file://D:/path
		pathSlashed = strings.TrimPrefix(pathSlashed, forwardSlash)
		return "file://" + pathSlashed // e.g. "file://D:/Temp/foo.json"
	}

	// Non-Windows: a path like "/tmp/foo.json" => "file:///tmp/foo.json"
	// If it doesn't start with '/', make it absolute
	if !strings.HasPrefix(pathSlashed, forwardSlash) {
		pathSlashed = forwardSlash + pathSlashed
	}
	return "file://" + pathSlashed
}

func fixWindowsFileScheme(rawURL string) (*url.URL, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("failed to parse URL %q: %w", rawURL, err)
	}
	if runtime.GOOS == "windows" && u.Scheme == "file" {
		if len(u.Host) > 0 {
			u.Path = u.Host + u.Path
			u.Host = ""
		}
		if strings.HasPrefix(u.Path, forwardSlash) && len(u.Path) > 2 && u.Path[2] == ':' {
			u.Path = strings.TrimPrefix(u.Path, forwardSlash) // => "D:/Temp/foo.json"
		}
	}

	return u, nil
}
