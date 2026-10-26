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
)

func removeTempDir(path string) {
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
	// Only use highlighting when outputting to a TTY (terminal), or when
	// ForceColor is set via ATMOS_FORCE_COLOR.
	// For files or pipes (without force), use simple formatting without highlighting.
	forceColor := atmosConfig != nil && atmosConfig.Settings.Terminal.ForceColor
	isTTY := file == "" && (tuiTerm.IsTTYSupportForStdout() || forceColor)

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
