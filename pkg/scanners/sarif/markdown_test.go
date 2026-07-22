package sarif

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRenderMarkdownEscapesLocationsAndCapsFindings(t *testing.T) {
	findings := &Findings{Tool: "tflint", Findings: []Finding{
		{Severity: SeverityCritical, RuleID: "rule|one", Message: "first|message", File: "dir with space/main.tf", Line: 3, HelpURI: "https://example.test/rule"},
		{Severity: SeverityLow, RuleID: "second", Message: strings.Repeat("x", 140), File: "plain.tf"},
	}}

	got := RenderMarkdown(findings, RenderMarkdownOptions{MaxFindings: 1})
	// The rule ID still links to its remediation guide — that's a real, useful,
	// short external URL, unlike a finding's own local file path.
	assert.Contains(t, got, "[rule\\|one](https://example.test/rule)")
	// With no RepoBaseURL (not running in GitHub Actions), the Location cell is
	// styled inline code, not a markdown link: a link inside a table cell makes
	// glamour pull the href into a numbered footnote below the table, which for a
	// local file path is a wall of "[N]: file /full/absolute/path" noise repeated
	// per finding. See locationCell's doc comment.
	assert.Contains(t, got, "`dir with space/main.tf:3`")
	assert.NotContains(t, got, "](dir")
	assert.Contains(t, got, "_…and 1 more_")
	assert.NotContains(t, got, "second")
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
// produces "[N]: high https://shields.io/badge/..."-style noise. A plain inline-code
// label carries no such baggage.
func TestSeverityBadge_Plain(t *testing.T) {
	assert.Equal(t, "`HIGH`", severityBadge(SeverityHigh, true))
	assert.Equal(t, "`HIGH: 3`", severityCountBadge(SeverityHigh, 3, true))
	assert.NotContains(t, severityBadge(SeverityHigh, true), "shields.io")
	assert.NotContains(t, severityCountBadge(SeverityHigh, 3, true), "shields.io")
}

func TestMarkdownHelpersCoverFallbacks(t *testing.T) {
	assert.Equal(t, "scan", applyRenderDefaults(RenderMarkdownOptions{}, &Findings{}).Tool)
	assert.Equal(t, "inactive", severityBadgeColor(Severity(999)))
	assert.Equal(t, "a…", truncate("abcd", 2))
	assert.Equal(t, "`file.tf`", locationCell(&Finding{File: "file.tf"}, "", ""))
}

// TestLocationCell_NoRepoBaseURL verifies the Location column renders as styled inline
// code with no link at all when repoBaseURL is unset (not running in GitHub Actions):
// a local filesystem path is meaningless as a link outside the machine that produced
// it, and a link inside a table cell makes glamour's ANSI renderer pull the href into a
// numbered footnote list below the table (see charmbracelet/glamour's
// ansi/table_links.go), which for a full local path is a wall of
// "[N]: file /full/absolute/path#L9"-style noise repeated per finding.
func TestLocationCell_NoRepoBaseURL(t *testing.T) {
	cell := locationCell(&Finding{File: "/repo/components/terraform/aws-account/account-map.deprecated.tf", Line: 9}, "", "")
	assert.Equal(t, "`/repo/components/terraform/aws-account/account-map.deprecated.tf:9`", cell)
	assert.NotContains(t, cell, "[")
	assert.NotContains(t, cell, "](")
}

// TestLocationCell_WithRepoBaseURL verifies the Location column becomes a real
// markdown link to the file on GitHub when repoBaseURL is set (running in GitHub
// Actions — see githubBlobBaseURL), since that's a genuinely useful destination there.
func TestLocationCell_WithRepoBaseURL(t *testing.T) {
	cell := locationCell(&Finding{File: "components/terraform/aws-account/account-map.deprecated.tf", Line: 9}, "", "https://github.com/org/repo/blob/abc123")
	assert.Equal(
		t,
		"[components/terraform/aws-account/account-map.deprecated.tf:9](https://github.com/org/repo/blob/abc123/components/terraform/aws-account/account-map.deprecated.tf#L9)",
		cell,
	)
}

// TestLocationCell_WithRepoBaseURL_NoLine verifies the href has no #L fragment when
// the finding has no line number, so it doesn't link to a nonsensical anchor.
func TestLocationCell_WithRepoBaseURL_NoLine(t *testing.T) {
	cell := locationCell(&Finding{File: "components/terraform/aws-account/main.tf"}, "", "https://github.com/org/repo/blob/abc123")
	assert.Equal(
		t,
		"[components/terraform/aws-account/main.tf](https://github.com/org/repo/blob/abc123/components/terraform/aws-account/main.tf)",
		cell,
	)
	assert.NotContains(t, cell, "#L")
}

// TestLocationCell_ComponentRelative verifies the Location column's displayed text
// (not the link, when present) shows a path relative to the component (not the
// finding's full file value) when the finding is under componentDir — this is what
// the "Location" column showing a full absolute path per finding/footnote
// (screenshots) was fixed to stop doing.
func TestLocationCell_ComponentRelative(t *testing.T) {
	tests := []struct {
		name         string
		file         string
		componentDir string
		line         int
		wantDisplay  string
	}{
		{
			name:         "file under component dir is shortened",
			file:         "/repo/components/terraform/aws-account/account-map.deprecated.tf",
			componentDir: "/repo/components/terraform/aws-account",
			line:         11,
			wantDisplay:  "account-map.deprecated.tf:11",
		},
		{
			name:         "nested file under component dir is shortened",
			file:         "/repo/components/terraform/aws-account/modules/nested/main.tf",
			componentDir: "/repo/components/terraform/aws-account",
			line:         4,
			wantDisplay:  "modules/nested/main.tf:4",
		},
		{
			name:         "componentDir unset leaves file unchanged",
			file:         "/repo/components/terraform/aws-account/account-map.deprecated.tf",
			componentDir: "",
			line:         9,
			wantDisplay:  "/repo/components/terraform/aws-account/account-map.deprecated.tf:9",
		},
		{
			name:         "file outside componentDir falls back to full path",
			file:         "/repo/components/terraform/shared-module/main.tf",
			componentDir: "/repo/components/terraform/aws-account",
			line:         2,
			wantDisplay:  "/repo/components/terraform/shared-module/main.tf:2",
		},
		{
			name:         "relative file with absolute componentDir falls back to full path",
			file:         "components/terraform/aws-account/account-map.deprecated.tf",
			componentDir: "/repo/components/terraform/aws-account",
			line:         9,
			wantDisplay:  "components/terraform/aws-account/account-map.deprecated.tf:9",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cell := locationCell(&Finding{File: tt.file, Line: tt.line}, tt.componentDir, "")
			assert.Equal(t, "`"+tt.wantDisplay+"`", cell)
		})
	}
}

// TestLocationCell_ComponentRelative_ChdirRelativePath covers tflint's --chdir mode,
// which reports paths relative to the scanning process's own working directory rather
// than a bare filename (e.g. "../../tmp/x/components/terraform/foo/main.tf"). Since
// componentDir is always absolute, a relative finding path must resolve against the
// process cwd before the component-relative comparison, or it always falls back to the
// full (messy) path unchanged.
func TestLocationCell_ComponentRelative_ChdirRelativePath(t *testing.T) {
	componentDir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(componentDir, "components", "terraform", "foo"), 0o755))
	fullComponentDir := filepath.Join(componentDir, "components", "terraform", "foo")

	cwd := t.TempDir()
	t.Chdir(cwd)

	relFile, err := filepath.Rel(cwd, filepath.Join(fullComponentDir, "main.tf"))
	require.NoError(t, err)

	cell := locationCell(&Finding{File: relFile, Line: 1}, fullComponentDir, "")
	assert.Equal(t, "`main.tf:1`", cell)
}
