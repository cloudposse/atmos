package validation

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/terminal"
)

const (
	richFallbackWidth = 80
	// The smallest usable width richClip will clip a source line to, even
	// when the caller's available width is narrower.
	richClipMinWidth = 20
	// Controls how close the diagnostic column may get to the end of the
	// visible clip window before richClip re-centers the clip around it.
	richClipRecenterMargin = 6
)

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
	defer perf.Track(nil, "validation.Rich")()

	width := options.Width
	if width <= 0 {
		width = richFallbackWidth
	}
	var out strings.Builder
	for index, diagnostic := range report.sortedDiagnostics() {
		if index > 0 {
			out.WriteByte('\n')
		}
		writeRichDiagnostic(&out, &diagnostic, options, width)
	}
	return out.String()
}

// DefaultRichOptions obtains the standard terminal capabilities. Keeping this
// separate lets tests and command callers supply deterministic settings.
func DefaultRichOptions(root string) RichOptions {
	defer perf.Track(nil, "validation.DefaultRichOptions")()

	term := terminal.New()
	return RichOptions{
		Root:  root,
		Width: term.Width(terminal.Stdout),
		Color: term.ColorProfile() != terminal.ColorNone,
	}
}

func writeRichDiagnostic(out *strings.Builder, diagnostic *Diagnostic, options RichOptions, width int) {
	writeRichDiagnosticHeader(out, diagnostic, options)
	if diagnostic.File == "" || diagnostic.Line <= 0 {
		return
	}
	writeRichDiagnosticSource(out, diagnostic, options, width)
}

// writeRichDiagnosticHeader writes the "[source] file:line:col" location line
// and the "severity: message [rule]" summary line.
func writeRichDiagnosticHeader(out *strings.Builder, diagnostic *Diagnostic, options RichOptions) {
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
}

// writeRichDiagnosticSource reads the diagnostic's source file and writes the
// surrounding source excerpt with a gutter and a caret marker under the
// finding. It is a silent no-op when the source cannot be read: presentation
// must never fail a validation run that already succeeded at collecting the
// diagnostic itself.
func writeRichDiagnosticSource(out *strings.Builder, diagnostic *Diagnostic, options RichOptions, width int) {
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
	ctx := richLineContext{
		out: out, diagnostic: diagnostic, options: options,
		endLine: endLine, width: width, gutterWidth: len(strconv.Itoa(end)),
	}
	for lineNumber := start; lineNumber <= end; lineNumber++ {
		writeRichDiagnosticLine(ctx, lines[lineNumber-1], lineNumber)
	}
}

// richLineContext bundles the parameters that stay constant across every
// line of one diagnostic's rendered source excerpt.
type richLineContext struct {
	out         *strings.Builder
	diagnostic  *Diagnostic
	options     RichOptions
	endLine     int
	width       int
	gutterWidth int
}

// writeRichDiagnosticLine writes one gutter-numbered source line, followed by
// a caret-marker line underneath when lineNumber falls within the
// diagnostic's [Line, endLine] span.
func writeRichDiagnosticLine(ctx richLineContext, line string, lineNumber int) {
	shown, offset := richClip(line, ctx.width-ctx.gutterWidth-richClipRecenterMargin, diagnosticColumn(ctx.diagnostic, line, lineNumber))
	fmt.Fprintf(ctx.out, "%*d | %s\n", ctx.gutterWidth, lineNumber, shown)
	if lineNumber < ctx.diagnostic.Line || lineNumber > ctx.endLine {
		return
	}
	column := diagnosticColumn(ctx.diagnostic, line, lineNumber)
	marker := max(1, column-offset)
	markerWidth := 1
	if lineNumber == ctx.endLine && ctx.diagnostic.EndColumn > column {
		markerWidth = ctx.diagnostic.EndColumn - column
	}
	fmt.Fprintf(ctx.out, "%s | %s\n", strings.Repeat(" ", ctx.gutterWidth), richStyle(strings.Repeat(" ", marker-1)+strings.Repeat("^", max(1, markerWidth)), "green", ctx.options.Color))
}

func richPath(root, file string) string {
	if filepath.IsAbs(file) || root == "" {
		return file
	}
	return filepath.Join(root, file)
}

func diagnosticColumn(diagnostic *Diagnostic, line string, lineNumber int) int {
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
	if available < richClipMinWidth {
		available = richClipMinWidth
	}
	runes := []rune(line)
	if len(runes) <= available {
		return line, 0
	}
	start := 0
	if column > available-richClipRecenterMargin {
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
