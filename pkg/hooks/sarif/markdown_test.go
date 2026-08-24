package sarif

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestSeverityBadgeColor exercises every named severity bucket (including Low
// and Info, which existing SARIF fixtures elsewhere in the suite never
// happen to trigger) plus an out-of-range value to cover the default branch.
func TestSeverityBadgeColor(t *testing.T) {
	tests := []struct {
		name string
		sev  Severity
		want string
	}{
		{"critical", SeverityCritical, "8b0000"},
		{"high", SeverityHigh, "critical"},
		{"medium", SeverityMedium, "important"},
		{"low", SeverityLow, "yellow"},
		{"info", SeverityInfo, "blue"},
		{"unknown falls back to inactive", Severity(99), "inactive"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, severityBadgeColor(tt.sev))
		})
	}
}

// TestRenderMarkdown_LowAndInfoSeverityBadges confirms the full markdown pipeline
// renders the low/info badge colors correctly end to end (not just the color-lookup
// helper in isolation) when RepoBaseURL is set (running in GitHub Actions), the only
// mode that renders real shields.io images rather than plain inline code.
func TestRenderMarkdown_LowAndInfoSeverityBadges(t *testing.T) {
	f := &Findings{
		Tool: "tfsec",
		Findings: []Finding{
			{RuleID: "R1", Severity: SeverityLow, Message: "low finding"},
			{RuleID: "R2", Severity: SeverityInfo, Message: "info finding"},
		},
	}
	out := RenderMarkdown(f, RenderMarkdownOptions{RepoBaseURL: "https://github.com/org/repo/blob/abc123"})
	assert.Contains(t, out, "LOW-1-yellow")
	assert.Contains(t, out, "INFO-1-blue")
}

// TestSeverityBadge_NoDeadAnchorLink verifies severity badges render as bare images,
// not images wrapped in a link to "#" — that anchor never resolves to anything in any
// renderer (terminal, GitHub, Pro), so wrapping it only added a dead footnote line
// (e.g. "[1]: high #") with no real destination.
func TestSeverityBadge_NoDeadAnchorLink(t *testing.T) {
	assert.NotContains(t, severityBadge(SeverityHigh, false), "](#)")
	assert.NotContains(t, severityCountBadge(SeverityHigh, 3, false), "](#)")
	assert.True(t, strings.HasPrefix(severityBadge(SeverityHigh, false), "!["))
	assert.True(t, strings.HasPrefix(severityCountBadge(SeverityHigh, 3, false), "!["))
}

// TestSeverityBadge_Plain verifies severity badges render as styled inline code, not
// a shields.io image, when plain is true (not running in GitHub Actions): glamour's
// ANSI renderer pulls *any* image inside a table cell (or heading) out into a
// numbered footnote listing its raw URL, so even a bare, un-link-wrapped image still
// produces "[N]: high https://shields.io/badge/..."-style noise.
func TestSeverityBadge_Plain(t *testing.T) {
	assert.Equal(t, "`HIGH`", severityBadge(SeverityHigh, true))
	assert.Equal(t, "`HIGH: 3`", severityCountBadge(SeverityHigh, 3, true))
	assert.NotContains(t, severityBadge(SeverityHigh, true), "shields.io")
	assert.NotContains(t, severityCountBadge(SeverityHigh, 3, true), "shields.io")
}

// TestLocationCell_NoRepoBaseURL verifies the Location column renders as styled inline
// code with no link at all when repoBaseURL is unset (not running in GitHub Actions):
// a local filesystem path is meaningless as a link outside the machine that produced
// it, and a link inside a table cell makes glamour's ANSI renderer pull the href into a
// numbered footnote list below the table.
func TestLocationCell_NoRepoBaseURL(t *testing.T) {
	cell := locationCell(&Finding{File: "components/terraform/vpc/main.tf", Line: 9}, "")
	assert.Equal(t, "`components/terraform/vpc/main.tf:9`", cell)
	assert.NotContains(t, cell, "[")
	assert.NotContains(t, cell, "](")
}

// TestLocationCell_WithRepoBaseURL verifies the Location column becomes a real
// markdown link to the file on GitHub when repoBaseURL is set (running in GitHub
// Actions — see githubBlobBaseURL), since that's a genuinely useful destination there.
func TestLocationCell_WithRepoBaseURL(t *testing.T) {
	cell := locationCell(&Finding{File: "components/terraform/vpc/main.tf", Line: 9}, "https://github.com/org/repo/blob/abc123")
	assert.Equal(
		t,
		"[components/terraform/vpc/main.tf:9](https://github.com/org/repo/blob/abc123/components/terraform/vpc/main.tf#L9)",
		cell,
	)
}
