package output

import (
	"fmt"

	errUtils "github.com/cloudposse/atmos/errors"
	tuiTerm "github.com/cloudposse/atmos/internal/tui/templates/term"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// WriteDocument takes the output format (`yaml` or `json`) and a file name,
// and prints the data to the console or to a file (if file is specified).
// Uses fast-path formatting (without syntax highlighting) when output is not a TTY.
func WriteDocument(request DocumentRequest) error {
	defer perf.Track(request.AtmosConfig, "output.WriteDocument")()

	// Determine if we should use expensive formatting (syntax highlighting).
	// Only use highlighting when outputting to a TTY (terminal), or when
	// ForceColor is set via ATMOS_FORCE_COLOR.
	// For files or pipes (without force), use simple formatting without highlighting.
	forceColor := request.AtmosConfig != nil && request.AtmosConfig.Settings.Terminal.ForceColor
	isTTY := request.File == "" && (tuiTerm.IsTTYSupportForStdout() || forceColor)

	switch request.Format {
	case "yaml":
		return writeYAMLDocument(request, isTTY)
	case "json":
		return writeJSONDocument(request, isTTY)
	default:
		return invalidDocumentFormat(request.Format)
	}
}

// writeYAMLDocument uses highlighting only for terminal output.
func writeYAMLDocument(request DocumentRequest, isTTY bool) error {
	if request.File != "" {
		return u.WriteToFileAsYAMLWithConfig(request.AtmosConfig, request.File, request.Data, DefaultFileMode)
	}
	if isTTY {
		return u.PrintAsYAML(request.AtmosConfig, request.Data)
	}
	return u.PrintAsYAMLSimple(request.AtmosConfig, request.Data)
}

// writeJSONDocument uses highlighting only for terminal output.
func writeJSONDocument(request DocumentRequest, isTTY bool) error {
	if request.File != "" {
		return u.WriteToFileAsJSON(request.File, request.Data, DefaultFileMode)
	}
	if isTTY {
		return u.PrintAsJSON(request.AtmosConfig, request.Data)
	}
	return u.PrintAsJSONSimple(request.AtmosConfig, request.Data)
}

// invalidDocumentFormat preserves the historical diagnostic while exposing the
// shared invalid-format sentinel to callers using errors.Is.
type invalidDocumentFormat string

func (e invalidDocumentFormat) Error() string {
	defer perf.Track(nil, "output.invalidDocumentFormat.Error")()

	return fmt.Sprintf("invalid 'format': %s", string(e))
}

func (e invalidDocumentFormat) Unwrap() error {
	defer perf.Track(nil, "output.invalidDocumentFormat.Unwrap")()

	return errUtils.ErrInvalidFormat
}

// DocumentRequest describes structured data destined for a file or stdout.
type DocumentRequest struct {
	AtmosConfig *schema.AtmosConfiguration
	Format      string
	File        string
	Data        any
}
