package validation

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/cloudposse/atmos/pkg/terminal"
)

const richFallbackWidth = 80

// RichOptions controls a human-oriented diagnostic rendering. Root resolves
// relative diagnostic paths, Width is the available terminal width (or zero
// for the standard fallback), and Color is normally derived from Atmos's
// terminal policy by callers.
type RichOptions struct {
	Root  string
	Width int
	Color bool
}

// Rich renders a report as deterministic source excerpts. It intentionally
// degrades to a complete header-only finding when a source cannot be read;
// presentation must never hide a validation failure.
func Rich(report Report, options RichOptions) string {
	width := options.Width
	if width <= 0 {
		width = richFallbackWidth
	}
	var out strings.Builder
	for index, diagnostic := range report.sortedDiagnostics() {
		if index > 0 {
			out.WriteByte('\n')
		}
		writeRichDiagnostic(&out, diagnostic, options, width)
	}
	return out.String()
}

// DefaultRichOptions obtains the standard terminal capabilities. Keeping this
// separate lets tests and command callers supply deterministic settings.
func DefaultRichOptions(root string) RichOptions {
	term := terminal.New()
	return RichOptions{
		Root:  root,
		Width: term.Width(terminal.Stdout),
		Color: term.ColorProfile() != terminal.ColorNone,
	}
}

func writeRichDiagnostic(out *strings.Builder, diagnostic Diagnostic, options RichOptions, width int) {
	location := diagnostic.File
	if diagnostic.Line > 0 {
		location += ":" + strconv.Itoa(diagnostic.Line)
		if diagnostic.Column > 0 {
			location += ":" + strconv.Itoa(diagnostic.Column)
		}
	}
	if location == "" {
		location = "(unknown location)"
	}
	label := diagnostic.Source
	if label == "" {
		label = "validation"
	}
	fmt.Fprintf(out, "%s %s\n", richStyle("["+label+"]", "yellow", options.Color), richStyle(location, "bold", options.Color))
	message := string(diagnostic.Severity)
	if message == "" {
		message = string(SeverityError)
	}
	fmt.Fprintf(out, "%s: %s", richStyle(message, severityColor(diagnostic.Severity), options.Color), diagnostic.Message)
	if diagnostic.RuleID != "" {
		fmt.Fprintf(out, " %s", richStyle("["+diagnostic.RuleID+"]", "muted", options.Color))
	}
	out.WriteByte('\n')

	if diagnostic.File == "" || diagnostic.Line <= 0 {
		return
	}
	contents, err := os.ReadFile(richPath(options.Root, diagnostic.File))
	if err != nil {
		return
	}
	lines := strings.Split(strings.ReplaceAll(string(contents), "\r\n", "\n"), "\n")
	if diagnostic.Line > len(lines) {
		return
	}
	endLine := diagnostic.EndLine
	if endLine < diagnostic.Line {
		endLine = diagnostic.Line
	}
	if endLine > len(lines) {
		endLine = len(lines)
	}
	start := max(1, diagnostic.Line-1)
	end := min(len(lines), endLine+1)
	gutterWidth := len(strconv.Itoa(end))
	for lineNumber := start; lineNumber <= end; lineNumber++ {
		line := lines[lineNumber-1]
		shown, offset := richClip(line, width-gutterWidth-6, diagnosticColumn(diagnostic, line, lineNumber))
		fmt.Fprintf(out, "%*d | %s\n", gutterWidth, lineNumber, shown)
		if lineNumber >= diagnostic.Line && lineNumber <= endLine {
			column := diagnosticColumn(diagnostic, line, lineNumber)
			marker := max(1, column-offset)
			markerWidth := 1
			if lineNumber == endLine && diagnostic.EndColumn > column {
				markerWidth = diagnostic.EndColumn - column
			}
			fmt.Fprintf(out, "%s | %s\n", strings.Repeat(" ", gutterWidth), richStyle(strings.Repeat(" ", marker-1)+strings.Repeat("^", max(1, markerWidth)), "green", options.Color))
		}
	}
}

func richPath(root, file string) string {
	if filepath.IsAbs(file) || root == "" {
		return file
	}
	return filepath.Join(root, file)
}

func diagnosticColumn(diagnostic Diagnostic, line string, lineNumber int) int {
	if lineNumber == diagnostic.Line && diagnostic.Column > 0 {
		return diagnostic.Column
	}
	return firstContentColumn(line)
}

func firstContentColumn(line string) int {
	for index, char := range line {
		if char != ' ' && char != '\t' {
			return utf8.RuneCountInString(line[:index]) + 1
		}
	}
	return 1
}

func richClip(line string, available, column int) (string, int) {
	if available < 20 {
		available = 20
	}
	runes := []rune(line)
	if len(runes) <= available {
		return line, 0
	}
	start := 0
	if column > available-6 {
		start = min(len(runes)-available+1, column-(available/2))
	}
	end := min(len(runes), start+available)
	prefix := ""
	suffix := ""
	if start > 0 {
		prefix = "…"
	}
	if end < len(runes) {
		suffix = "…"
	}
	return prefix + string(runes[start:end]) + suffix, start - len([]rune(prefix))
}

func severityColor(severity Severity) string {
	switch severity {
	case SeverityWarning:
		return "yellow"
	case SeverityNotice:
		return "blue"
	default:
		return "red"
	}
}

func richStyle(value, style string, enabled bool) string {
	if !enabled {
		return value
	}
	code := map[string]string{"bold": "1", "red": "31", "green": "32", "yellow": "33", "blue": "34", "muted": "90"}[style]
	if code == "" {
		return value
	}
	return "\x1b[" + code + "m" + value + "\x1b[0m"
}
