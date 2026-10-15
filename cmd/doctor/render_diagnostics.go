package doctor

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"

	diagnostic "github.com/cloudposse/atmos/pkg/doctor"
	iolib "github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/pkg/terminal"
	"github.com/cloudposse/atmos/pkg/ui"
	"github.com/cloudposse/atmos/pkg/ui/theme"
	"github.com/cloudposse/atmos/pkg/validation"
)

const (
	defaultReportWidth = 80
	reportSpace        = " "
)

// reportRenderer composes human output before handing it to the masked UI stream.
// Width and styles are captured once so a report has consistent line wrapping.
type reportRenderer struct {
	width  int
	styles *theme.StyleSet
}

func newReportRenderer() reportRenderer {
	width := terminal.New().Width(terminal.Stderr)
	if width <= 0 {
		width = defaultReportWidth
	}
	return reportRenderer{width: width, styles: theme.GetCurrentStyles()}
}

// line masks before wrapping: splitting a secret across lines would prevent the
// final output writer from recognizing it. Prefixes contain only renderer text.
//
//nolint:gocritic // Lip Gloss styles are immutable values, matching its fluent API.
func (p reportRenderer) line(prefix, text string, style lipgloss.Style) {
	text = ansi.Strip(iolib.MaskString(text))
	indent := strings.Repeat(reportSpace, ansi.StringWidth(prefix))
	wrapped := ansi.Wrap(text, max(1, p.width-ansi.StringWidth(prefix)), "")
	for i, line := range strings.Split(wrapped, "\n") {
		lead := prefix
		if i > 0 {
			lead = indent
		}
		ui.Writeln(p.styles.Heading.Render(lead) + style.Render(line))
	}
}

// unwrapped writes one line that is never wrapped, for values such as paths that must stay copyable.
// It still masks the text and strips terminal control sequences from it.
//
//nolint:gocritic // Lip Gloss styles are immutable values, matching its fluent API.
func (p reportRenderer) unwrapped(prefix, text string, style lipgloss.Style) {
	text = ansi.Strip(iolib.MaskString(text))
	ui.Writeln(p.styles.Heading.Render(prefix) + style.Render(text))
}

func renderHeading(fix, dryRun bool) {
	mode := "Diagnose"
	if fix {
		mode = "Repair"
	}
	if dryRun {
		mode = "Dry run"
	}
	p := newReportRenderer()
	p.line("", "Atmos Doctor · "+mode, p.styles.Title)
	ui.Writeln("")
}

// renderFindings is the sole detailed finding output for both human formats.
func renderFindings(r *diagnostic.Report) {
	p := newReportRenderer()
	for i := range r.Findings {
		f := &r.Findings[i]
		if f.Status == "fixed" {
			continue
		}
		p.finding(f, r.SourceRoot)
	}
}

func (p reportRenderer) finding(f *diagnostic.Finding, root string) {
	severity := p.styles.Error
	switch f.Severity {
	case validation.SeverityWarning:
		severity = p.styles.Warning
	case validation.SeverityNotice:
		severity = p.styles.Info
	}
	p.line("", fmt.Sprintf("%s[%s]: %s", f.Severity, f.RuleID, f.Message), severity.Bold(true))
	if f.File != "" {
		location := f.File
		if f.Line > 0 {
			location += ":" + strconv.Itoa(f.Line)
			if f.Column > 0 {
				location += ":" + strconv.Itoa(f.Column)
			}
		}
		p.line("  --> ", location, p.styles.Body.Bold(true))
	}
	p.source(f, root, severity)
	if status := repairLabel(f); status != "" {
		p.line("   = repair: ", status, p.styles.Body)
	}
	if f.Remediation != "" && f.Remediation != "Run atmos doctor --fix" {
		p.line("   = help: ", f.Remediation, p.styles.Command)
	}
	if f.Reason != "" {
		p.line("   = note: ", f.Reason, p.styles.Body)
	}
	ui.Writeln("")
}

func repairLabel(f *diagnostic.Finding) string {
	switch f.Status {
	case "verification-failed":
		return "Verification failed"
	case "verification-incomplete":
		return "Verification incomplete"
	case "manual":
		return "Manual action required"
	case "available":
		if f.Fixable {
			return "Automatic fix available"
		}
	}
	return ""
}

//nolint:gocritic // Lip Gloss styles are immutable values, matching its fluent API.
func (p reportRenderer) source(f *diagnostic.Finding, root string, severity lipgloss.Style) {
	d := validation.Diagnostic{File: f.File, Line: f.Line, Column: f.Column}
	lines, start, _ := validation.SourceExcerpt(&d, root)
	if len(lines) == 0 {
		return
	}
	gutter := len(strconv.Itoa(start + len(lines) - 1))
	blank := strings.Repeat(reportSpace, gutter) + " | "
	ui.Writeln(p.styles.Muted.Render(strings.TrimRight(blank, reportSpace)))
	for index, raw := range lines {
		column := 0
		if start+index == f.Line {
			column = sourceColumn(raw, f.Column)
		}
		// Mask complete source lines before tokenization or clipping. If redaction
		// changes the line, point at its first content rather than a stale column.
		masked := ansi.Strip(iolib.MaskString(raw))
		if masked != raw && column > 0 {
			column = sourceColumn(masked, 0)
		}
		line := strings.ReplaceAll(masked, "\t", "    ")
		shown, marker := clipSource(highlightYAML(line, p.styles), p.width-gutter-3, column)
		// A blank source line ends at the gutter bar, so the excerpt never carries trailing whitespace.
		number := fmt.Sprintf("%*d | ", gutter, start+index)
		if strings.TrimSpace(ansi.Strip(shown)) == "" {
			number, shown = strings.TrimRight(number, reportSpace), ""
		}
		ui.Writeln(p.styles.Muted.Render(number) + shown)
		if column > 0 {
			ui.Writeln(p.styles.Muted.Render(blank) + strings.Repeat(reportSpace, marker-1) + severity.Render("^"))
		}
	}
	ui.Writeln(p.styles.Muted.Render(strings.TrimRight(blank, reportSpace)))
}

// YAML columns count runes; terminal markers count display cells, including tabs.
func sourceColumn(line string, column int) int {
	runes := []rune(line)
	if column <= 0 {
		column = 1
		for _, r := range runes {
			if r != ' ' && r != '\t' {
				break
			}
			column++
		}
	}
	prefix := string(runes[:min(len(runes), max(0, column-1))])
	return ansi.StringWidth(strings.ReplaceAll(prefix, "\t", "    ")) + 1
}

// clipSource keeps the marker in view and reserves cells for both ellipses.
func clipSource(line string, width, column int) (string, int) {
	width = max(3, width)
	length := ansi.StringWidth(line)
	column = min(max(1, column), max(1, length+1))
	if length < width {
		return line, column
	}
	start := 0
	if column >= width-1 {
		start = max(0, column-width/2)
		// Align the left edge to a grapheme boundary so wide characters do not
		// shift the marker when the clipping window starts inside one.
		start = ansi.StringWidth(ansi.Truncate(line, start, ""))
	}
	prefix := ""
	if start > 0 {
		prefix = "…"
	}
	available := width - ansi.StringWidth(prefix)
	end := min(length, start+available)
	suffix := ""
	if end < length {
		end--
		suffix = "…"
	}
	return prefix + ansi.Cut(line, start, end) + suffix, min(width, column-start+ansi.StringWidth(prefix))
}

// Chroma supplies YAML token boundaries; Atmos supplies semantic theme colors.
func highlightYAML(line string, styles *theme.StyleSet) string {
	if ui.GetColorProfile() == termenv.Ascii {
		return line
	}
	iterator, err := lexers.Get("yaml").Tokenise(nil, line+"\n")
	if err != nil {
		return line
	}
	var out strings.Builder
	for token := iterator(); token != chroma.EOF; token = iterator() {
		style := styles.Body
		switch {
		case token.Type.InCategory(chroma.Comment):
			style = styles.Muted
		case token.Type.InCategory(chroma.Name):
			style = styles.Command
		case token.Type.InCategory(chroma.Literal):
			style = styles.Help.Code
		case token.Type.InCategory(chroma.Keyword):
			style = styles.Info
		}
		out.WriteString(style.Render(strings.TrimSuffix(token.Value, "\n")))
	}
	return strings.TrimSuffix(out.String(), "\n")
}
