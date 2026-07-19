package sarif

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRenderMarkdownEscapesLocationsAndCapsFindings(t *testing.T) {
	findings := &Findings{Tool: "tflint", Findings: []Finding{
		{Severity: SeverityCritical, RuleID: "rule|one", Message: "first|message", File: "dir with space/main.tf", Line: 3, HelpURI: "https://example.test/rule"},
		{Severity: SeverityLow, RuleID: "second", Message: strings.Repeat("x", 140), File: "plain.tf"},
	}}

	got := RenderMarkdown(findings, RenderMarkdownOptions{MaxFindings: 1})
	assert.Contains(t, got, "[rule\\|one](https://example.test/rule)")
	assert.Contains(t, got, "[dir with space/main.tf:3](dir%20with%20space/main.tf#L3)")
	assert.Contains(t, got, "_…and 1 more_")
	assert.NotContains(t, got, "second")
}

func TestMarkdownHelpersCoverFallbacks(t *testing.T) {
	assert.Equal(t, "scan", applyRenderDefaults(RenderMarkdownOptions{}, &Findings{}).Tool)
	assert.Equal(t, "inactive", severityBadgeColor(Severity(999)))
	assert.Equal(t, "a…", truncate("abcd", 2))
	assert.Equal(t, "file.tf", locationCell(&Finding{File: "file.tf"}))
	assert.Equal(t, "a%20b%23%28c%29", escapeLinkDestination("a b#(c)"))
}
