package sarif

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/cloudposse/atmos/pkg/perf"
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
	// InlineCodeMark wraps text as markdown inline code (e.g. severity badges and
	// the Location cell in plain/non-GitHub-Actions rendering).
	inlineCodeMark = "`"
)

// RenderMarkdownOptions controls how a Findings is rendered to markdown.
type RenderMarkdownOptions struct {
	// MaxFindings caps how many individual findings appear in the table.
	// Defaults to defaultMaxFindings when zero. A negative value (e.g. -1) shows
	// every finding with no cap at all.
	MaxFindings int
	// Tool is shown in the header (e.g., "trivy", "checkov"). Falls back to
	// Findings.Tool when empty.
	Tool string
	// ComponentDir is the absolute path to the component being scanned. When a
	// finding's file is under this directory and RepoBaseURL is unset, the Location
	// column displays the path relative to it (e.g. "main.tf:9") instead of the full
	// path — the component is already established by the surrounding "Linting
	// <component> in <stack>" output, so repeating its full absolute path on every
	// row just adds noise.
	ComponentDir string
	// RepoBaseURL, when set, turns the Location column into a real link to the file
	// on GitHub: "<RepoBaseURL>/<file>#L<line>" (e.g.
	// "https://github.com/org/repo/blob/<sha>"). Populated only when running in
	// GitHub Actions (see githubBlobBaseURL), since a local filesystem path is
	// meaningless as a link anywhere but the machine that produced it — locally the
	// Location column renders as styled inline code instead, with no link at all, so
	// glamour doesn't pull the (useless) href out into a footnote.
	RepoBaseURL string
}

// RenderMarkdown produces a Summary-style markdown body from Findings.
// The same body is what appears in the user's terminal, on the Pro run
// page, and in PR comments — format symmetry per the PRD.
func RenderMarkdown(f *Findings, opts RenderMarkdownOptions) string {
	defer perf.Track(nil, "sarif.RenderMarkdown")()

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

	// plain is true when not running in GitHub Actions (see githubBlobBaseURL):
	// severity badges then render as styled inline code rather than shields.io
	// images, since glamour's ANSI renderer pulls *any* image or link inside a
	// table cell out into a numbered footnote listing its raw URL (see
	// charmbracelet/glamour's ansi/table_links.go) — for a repeated per-row badge
	// that's a wall of "[N]: high https://shields.io/badge/..."-style noise. GitHub
	// (PR comments, Pro run page) renders the same markdown natively and shows the
	// badge image inline with no such footnote, so it keeps the real image there.
	plain := opts.RepoBaseURL == ""
	counts := f.CountsBySeverity()
	fmt.Fprintf(&b, "## %s — %s\n\n", tool, countsTitle(counts, plain))
	renderCountsTable(&b, counts, plain)
	renderFindingsTable(&b, f.SortedBySeverity(), opts.MaxFindings, findingsTableStyle{
		componentDir: opts.ComponentDir,
		repoBaseURL:  opts.RepoBaseURL,
		plain:        plain,
	})
	return b.String()
}

// applyRenderDefaults fills in zero-valued options from the supplied
// Findings or compile-time constants, so the orchestrator above stays
// focused on layout. A negative MaxFindings ("show every finding") is left as-is
// — only an unset (zero) value falls back to defaultMaxFindings.
func applyRenderDefaults(opts RenderMarkdownOptions, f *Findings) RenderMarkdownOptions {
	if opts.MaxFindings == 0 {
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
func renderCountsTable(b *strings.Builder, counts map[string]int, plain bool) {
	b.WriteString("| Severity | Count |\n")
	b.WriteString("|---|---|\n")
	for _, sev := range []Severity{SeverityCritical, SeverityHigh, SeverityMedium, SeverityLow, SeverityInfo} {
		if c, ok := counts[sev.String()]; ok && c > 0 {
			fmt.Fprintf(b, "| %s | %d |\n", severityBadge(sev, plain), c)
		}
	}
	b.WriteString("\n")
}

// findingsTableStyle bundles renderFindingsTable's per-run rendering context —
// otherwise it needs componentDir, repoBaseURL, and plain as separate parameters,
// pushing the function over the repo's max-argument lint threshold.
type findingsTableStyle struct {
	componentDir string
	repoBaseURL  string
	plain        bool
}

// renderFindingsTable emits the per-finding table (capped at limit rows, or
// uncapped when limit is negative — "show every finding") plus an "…and N more"
// footer when more findings exist than the cap.
func renderFindingsTable(b *strings.Builder, sorted []Finding, limit int, style findingsTableStyle) {
	if limit < 0 || len(sorted) < limit {
		limit = len(sorted)
	}

	b.WriteString("| Severity | Rule | Message | Location |\n")
	b.WriteString("|---|---|---|---|\n")
	for _, fd := range sorted[:limit] {
		// When a helpUri is available, render the rule ID as a markdown
		// link so terminals (and Pro, and PR comments) turn it into a
		// clickable jump to the official remediation guide. Falls back
		// to a plain rule ID when the SARIF doesn't include a helpUri.
		ruleCell := escapeMD(fd.RuleID)
		if fd.HelpURI != "" {
			ruleCell = fmt.Sprintf("[%s](%s)", escapeMD(fd.RuleID), fd.HelpURI)
		}
		fmt.Fprintf(b, "| %s | %s | %s | %s |\n",
			severityBadge(fd.Severity, style.plain), ruleCell, escapeMD(truncate(fd.Message, maxMessageLength)), locationCell(&fd, style.componentDir, style.repoBaseURL))
	}

	if len(sorted) > limit {
		fmt.Fprintf(b, "\n_…and %d more_\n", len(sorted)-limit)
	}
}

// countsTitle builds a compact severity headline from count badges.
// Falls back to the total count when no severity buckets are populated.
func countsTitle(counts map[string]int, plain bool) string {
	if len(counts) == 0 {
		return "no findings"
	}
	parts := make([]string, 0, len(counts))
	for _, sev := range []Severity{SeverityCritical, SeverityHigh, SeverityMedium, SeverityLow, SeverityInfo} {
		name := sev.String()
		if c, ok := counts[name]; ok && c > 0 {
			parts = append(parts, severityCountBadge(sev, c, plain))
		}
	}
	if len(parts) == 0 {
		return "no findings"
	}
	return strings.Join(parts, " ")
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

// severityBadge and severityCountBadge return styled inline code (e.g. "`HIGH`")
// when plain is true, and a shields.io image otherwise. This matters because
// glamour's ANSI renderer pulls *any* image, not just a link-wrapped one, out into
// a numbered footnote listing its raw URL — see charmbracelet/glamour's
// ansi/table_links.go — so a bare, un-link-wrapped image still produces the same
// per-row/per-heading "[N]: high https://shields.io/badge/..." noise a dead "#"
// anchor link did. Plain mode is used locally (not running in GitHub Actions — see
// githubBlobBaseURL); GitHub (PR comments, Pro run page) renders the same
// markdown natively, shows the badge image inline, and never produces a
// footnote, so it keeps the real image there.
func severityBadge(sev Severity, plain bool) string {
	label := strings.ToUpper(shortSeverity(sev.String()))
	if plain {
		return inlineCodeMark + label + inlineCodeMark
	}
	return fmt.Sprintf("![%s](https://shields.io/badge/-%s-%s?style=for-the-badge)",
		strings.ToLower(label), label, severityBadgeColor(sev))
}

func severityCountBadge(sev Severity, count int, plain bool) string {
	label := strings.ToUpper(shortSeverity(sev.String()))
	if plain {
		return fmt.Sprintf("`%s: %d`", label, count)
	}
	return fmt.Sprintf("![%s](https://shields.io/badge/%s-%d-%s?style=for-the-badge)",
		strings.ToLower(label), label, count, severityBadgeColor(sev))
}

func severityBadgeColor(sev Severity) string {
	switch sev {
	case SeverityCritical:
		return "8b0000"
	case SeverityHigh:
		return "critical"
	case SeverityMedium:
		return "important"
	case SeverityLow:
		return "yellow"
	case SeverityInfo:
		return "blue"
	default:
		return "inactive"
	}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}

// locationCell links to the file on GitHub when repoBaseURL is set (running in GitHub
// Actions — see githubBlobBaseURL), since that's a real, useful destination. Otherwise
// it renders styled inline code with no link at all: a *local* filesystem path is
// meaningless as a link outside the machine that produced it, and a link inside a table
// cell makes glamour's ANSI renderer pull the href out into a numbered footnote list
// below the table (see charmbracelet/glamour's ansi/table_links.go) — for a full local
// path that's a wall of "[N]: file /full/absolute/path#L9"-style noise for every single
// finding. Inline code keeps the column visually distinct from the plain Rule/Message
// text without paying that cost.
func locationCell(fd *Finding, componentDir, repoBaseURL string) string {
	if fd.File == "" {
		return ""
	}
	displayFile := shortenToComponent(fd.File, componentDir)
	loc := displayFile
	if fd.Line > 0 {
		loc = fmt.Sprintf("%s:%d", displayFile, fd.Line)
	}

	if repoBaseURL == "" {
		return inlineCodeMark + escapeMD(loc) + inlineCodeMark
	}

	// fd.File is already repo-relative here: normalizeArtifactURIs runs before
	// RenderMarkdown and only resolves githubBlobBaseURL's env vars (GITHUB_SHA etc.)
	// when GITHUB_WORKSPACE is also set, so both are present or both are absent.
	// The #Lline fragment is appended after escaping the path, so its own "#" is
	// never percent-encoded away.
	href := escapeLinkDestination(repoBaseURL + "/" + fd.File)
	if fd.Line > 0 {
		href += fmt.Sprintf("#L%d", fd.Line)
	}
	return fmt.Sprintf("[%s](%s)", escapeMD(loc), href)
}

func escapeLinkDestination(s string) string {
	replacer := strings.NewReplacer(
		" ", "%20",
		"\n", "",
		"\r", "",
		"(", "%28",
		")", "%29",
	)
	return replacer.Replace(s)
}

// shortenToComponent returns file relative to componentDir when file is under it, so
// the Location column doesn't repeat the component's full path on every row/footnote —
// the surrounding "Linting <component> in <stack>" output already establishes it.
// Falls back to file unchanged when componentDir is unset, file isn't under it (e.g. a
// shared module elsewhere), or the two paths aren't comparable (one absolute, one not —
// e.g. file was already normalized to a workspace-relative path in CI).
func shortenToComponent(file, componentDir string) string {
	if componentDir == "" {
		return file
	}
	// tflint's --chdir mode reports paths relative to the scanning process's own
	// working directory rather than the --chdir target (e.g.
	// "../../private/tmp/x/components/terraform/foo/main.tf"), not just a bare
	// filename. filepath.Rel needs both sides absolute or both relative to the
	// same base to compare correctly, and componentDir is always absolute, so
	// resolve a relative file against the process cwd (the same cwd the
	// subprocess inherited, since scanners never set cmd.Dir) before comparing.
	absFile := file
	if !filepath.IsAbs(absFile) {
		if resolved, err := filepath.Abs(absFile); err == nil {
			absFile = resolved
		}
	}
	rel, err := filepath.Rel(componentDir, absFile)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return file
	}
	return filepath.ToSlash(rel)
}

// escapeMD escapes pipe and newline characters that would break a table row.
func escapeMD(s string) string {
	s = strings.ReplaceAll(s, "|", `\|`)
	s = strings.ReplaceAll(s, "\n", " ")
	return s
}
