package exec

import (
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

const (
	DefaultFileMode os.FileMode = 0o644
	forwardSlash                = "/"
)

func removeTempDir(path string) {
	err := os.RemoveAll(path)
	if err != nil {
		u.LogWarning(err.Error())
	}
}

func closeFile(fileName string, file io.ReadCloser) {
	err := file.Close()
	if err != nil {
		u.LogError(fmt.Errorf("error closing the file '%s': %v", fileName, err))
	}
}

// printOrWriteToFile takes the output format (`yaml` or `json`) and a file name,
// and prints the data to the console or to a file (if file is specified)
func printOrWriteToFile(
	atmosConfig *schema.AtmosConfiguration,
	format string,
	file string,
	data any,
) error {
	switch format {
	case "yaml":

		if file == "" {
			err := u.PrintAsYAMLWithConfig(atmosConfig, data)
			if err != nil {
				return err
			}
		} else {
			err := u.WriteToFileAsYAMLWithConfig(atmosConfig, file, data, DefaultFileMode)
			if err != nil {
				return err
			}
		}

	case "json":
		if file == "" {
			err := u.PrintAsJSON(atmosConfig, data)
			if err != nil {
				return err
			}
		} else {
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
		// If pathSlashed is "/D:/Temp/foo.json", remove the leading slash => "D:/Temp/foo.json"
		// Then prepend "file://"
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
