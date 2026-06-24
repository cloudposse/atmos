package sarif

import (
	"fmt"
	"strings"
)

// Markdown rendering tunables. Centralized as constants so the
// magic-number linter has names to point at and so all renderers stay
// in sync.
const (
	// Default cap on individual finding rows when the caller doesn't
	// supply RenderMarkdownOptions.MaxFindings.
	defaultMaxFindings = 10
	// Truncation point for the message column so each row stays
	// scannable in a typical 100-column terminal.
	maxMessageLength = 120
)

// RenderMarkdownOptions controls how a Findings is rendered to markdown.
type RenderMarkdownOptions struct {
	// MaxFindings caps how many individual findings appear in the table.
	// Defaults to defaultMaxFindings when zero.
	MaxFindings int
	// Tool is shown in the header (e.g., "trivy", "checkov"). Falls back to
	// Findings.Tool when empty.
	Tool string
}

// RenderMarkdown produces a Summary-style markdown body from Findings.
// The same body is what appears in the user's terminal, on the Pro run
// page, and in PR comments — format symmetry per the PRD.
func RenderMarkdown(f *Findings, opts RenderMarkdownOptions) string {
	if f == nil {
		return ""
	}
	opts = applyRenderDefaults(opts, f)

	var b strings.Builder
	tool := opts.Tool

	if f.Count() == 0 {
		fmt.Fprintf(&b, "## %s\n\n✅ no findings\n", tool)
		return b.String()
	}

	counts := f.CountsBySeverity()
	fmt.Fprintf(&b, "## %s — %s\n\n", tool, countsTitle(counts))
	renderCountsTable(&b, counts)
	renderFindingsTable(&b, f.SortedBySeverity(), opts.MaxFindings)
	return b.String()
}

// applyRenderDefaults fills in zero-valued options from the supplied
// Findings or compile-time constants, so the orchestrator above stays
// focused on layout.
func applyRenderDefaults(opts RenderMarkdownOptions, f *Findings) RenderMarkdownOptions {
	if opts.MaxFindings <= 0 {
		opts.MaxFindings = defaultMaxFindings
	}
	if opts.Tool == "" {
		opts.Tool = f.Tool
	}
	if opts.Tool == "" {
		opts.Tool = "scan"
	}
	return opts
}

// renderCountsTable emits the severity-summary table — one row per
// severity bucket that actually has findings.
func renderCountsTable(b *strings.Builder, counts map[string]int) {
	b.WriteString("| Severity | Count |\n")
	b.WriteString("|---|---|\n")
	for _, sev := range []Severity{SeverityCritical, SeverityHigh, SeverityMedium, SeverityLow, SeverityInfo} {
		if c, ok := counts[sev.String()]; ok && c > 0 {
			fmt.Fprintf(b, "| %s | %d |\n", sev, c)
		}
	}
	b.WriteString("\n")
}

// renderFindingsTable emits the per-finding table (capped at limit
// rows) plus an "…and N more" footer when more findings exist.
func renderFindingsTable(b *strings.Builder, sorted []Finding, limit int) {
	if len(sorted) < limit {
		limit = len(sorted)
	}

	b.WriteString("| Severity | Rule | Message | Location |\n")
	b.WriteString("|---|---|---|---|\n")
	for _, fd := range sorted[:limit] {
		loc := fd.File
		if fd.Line > 0 {
			loc = fmt.Sprintf("%s:%d", fd.File, fd.Line)
		}
		// When a helpUri is available, render the rule ID as a markdown
		// link so terminals (and Pro, and PR comments) turn it into a
		// clickable jump to the official remediation guide. Falls back
		// to a plain rule ID when the SARIF doesn't include a helpUri.
		ruleCell := escapeMD(fd.RuleID)
		if fd.HelpURI != "" {
			ruleCell = fmt.Sprintf("[%s](%s)", escapeMD(fd.RuleID), fd.HelpURI)
		}
		fmt.Fprintf(b, "| %s | %s | %s | %s |\n",
			fd.Severity, ruleCell, escapeMD(truncate(fd.Message, maxMessageLength)), escapeMD(loc))
	}

	if len(sorted) > limit {
		fmt.Fprintf(b, "\n_…and %d more_\n", len(sorted)-limit)
	}
}

// countsTitle builds a compact severity headline like "2 HIGH, 5 MED".
// Falls back to the total count when no severity buckets are populated.
func countsTitle(counts map[string]int) string {
	if len(counts) == 0 {
		return "no findings"
	}
	parts := make([]string, 0, len(counts))
	for _, sev := range []Severity{SeverityCritical, SeverityHigh, SeverityMedium, SeverityLow, SeverityInfo} {
		name := sev.String()
		if c, ok := counts[name]; ok && c > 0 {
			parts = append(parts, fmt.Sprintf("%d %s", c, strings.ToUpper(shortSeverity(name))))
		}
	}
	if len(parts) == 0 {
		return "no findings"
	}
	return strings.Join(parts, ", ")
}

// shortSeverity returns a compact severity label for headlines.
func shortSeverity(s string) string {
	switch s {
	case "critical":
		return "crit"
	case "medium":
		return "med"
	default:
		return s
	}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}

// escapeMD escapes pipe and newline characters that would break a table row.
func escapeMD(s string) string {
	s = strings.ReplaceAll(s, "|", `\|`)
	s = strings.ReplaceAll(s, "\n", " ")
	return s
}
