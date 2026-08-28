package validate

import (
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/validation"
)

// Text renders diagnostics in a stable file:line:column form suitable for
// terminal and CI UI output.
func Text(report validation.Report) string {
	defer perf.Track(nil, "validate.Text")()

	diagnostics := append([]validation.Diagnostic(nil), report.Diagnostics...)
	sort.SliceStable(diagnostics, func(i, j int) bool {
		left, right := diagnostics[i], diagnostics[j]
		if left.File != right.File {
			return left.File < right.File
		}
		if left.Line != right.Line {
			return left.Line < right.Line
		}
		if left.Column != right.Column {
			return left.Column < right.Column
		}
		if left.RuleID != right.RuleID {
			return left.RuleID < right.RuleID
		}
		return left.Message < right.Message
	})

	var output strings.Builder
	for _, diagnostic := range diagnostics {
		if diagnostic.Column > 0 {
			fmt.Fprintf(&output, "%s:%d:%d: %s [%s]\n", diagnostic.File, diagnostic.Line, diagnostic.Column, diagnostic.Message, diagnostic.RuleID)
			continue
		}
		fmt.Fprintf(&output, "%s:%d: %s [%s]\n", diagnostic.File, diagnostic.Line, diagnostic.Message, diagnostic.RuleID)
	}
	return output.String()
}

// WriteText writes Text to an arbitrary writer for callers that need a raw,
// pipeable representation.
func WriteText(writer io.Writer, report validation.Report) error {
	defer perf.Track(nil, "validate.WriteText")()

	_, err := io.WriteString(writer, Text(report))
	return err
}
