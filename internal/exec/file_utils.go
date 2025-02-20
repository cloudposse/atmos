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

func removeTempDir(atmosConfig schema.AtmosConfiguration, path string) {
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

// and prints the data to the console or to a file (if file is specified).
func printOrWriteToFile(
	format string,
	file string,
	data any,
) error {
	switch format {
	case "yaml":
		if file == "" {
			err := u.PrintAsYAML(data)
			if err != nil {
				return err
			}
		} else {
			err := u.WriteToFileAsYAML(file, data, 0o644)
			if err != nil {
				return err
			}
		}

	case "json":
		if file == "" {
			err := u.PrintAsJSON(data)
			if err != nil {
				return err
			}
		} else {
			err := u.WriteToFileAsJSON(file, data, 0o644)
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
