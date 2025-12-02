package exec

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	tuiTerm "github.com/cloudposse/atmos/internal/tui/templates/term"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

const (
	DefaultFileMode os.FileMode = 0o644
	forwardSlash                = "/"
	uncPathPrefix               = "//" // UNC paths after filepath.ToSlash on Windows
)

// RemoveTempDir removes a temporary directory and logs a warning on error.
func RemoveTempDir(path string) {
	defer perf.Track(nil, "exec.RemoveTempDir")()

	err := os.RemoveAll(path)
	if err != nil {
		log.Warn(err.Error())
	}
}

// printOrWriteToFile takes the output format (`yaml` or `json`) and a file name,
// and prints the data to the console or to a file (if file is specified).
// Uses fast-path formatting (without syntax highlighting) when output is not a TTY.
func printOrWriteToFile(
	atmosConfig *schema.AtmosConfiguration,
	format string,
	file string,
	data any,
) error {
	defer perf.Track(atmosConfig, "exec.printOrWriteToFile")()

	// Determine if we should use expensive formatting (syntax highlighting).
	// Only use highlighting when outputting to a TTY (terminal).
	// For files or pipes, use simple formatting without highlighting.
	isTTY := file == "" && tuiTerm.IsTTYSupportForStdout()

	switch format {
	case "yaml":
		if file == "" {
			// Output to stdout.
			if isTTY {
				// TTY: use expensive highlighting for better readability.
				err := u.PrintAsYAML(atmosConfig, data)
				if err != nil {
					return err
				}
			} else {
				// Non-TTY (pipe): use fast-path without highlighting.
				err := u.PrintAsYAMLSimple(atmosConfig, data)
				if err != nil {
					return err
				}
			}
		} else {
			// Output to file: use simple formatting without highlighting.
			err := u.WriteToFileAsYAMLWithConfig(atmosConfig, file, data, DefaultFileMode)
			if err != nil {
				return err
			}
		}

	case "json":
		if file == "" {
			// Output to stdout.
			if isTTY {
				// TTY: use expensive highlighting for better readability.
				err := u.PrintAsJSON(atmosConfig, data)
				if err != nil {
					return err
				}
			} else {
				// Non-TTY (pipe): use fast-path without highlighting.
				err := u.PrintAsJSONSimple(atmosConfig, data)
				if err != nil {
					return err
				}
			}
		} else {
			// Output to file: use simple formatting without highlighting.
			err := u.WriteToFileAsJSON(file, data, DefaultFileMode)
			if err != nil {
				return err
			}
		}

	default:
		return fmt.Errorf("invalid 'format': %s", format)
	}

	return nil
}

// SanitizeFileName replaces invalid characters and query strings with underscores for Windows.
func SanitizeFileName(uri string) string {
	defer perf.Track(nil, "exec.SanitizeFileName")()

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

// toFileURL converts a local filesystem path into a "file://" URL in a way
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
