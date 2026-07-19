package sarif

import (
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

// TestRenderMarkdown_LowAndInfoSeverityBadges confirms the full markdown
// pipeline renders the low/info badge colors correctly end to end (not just
// the color-lookup helper in isolation).
func TestRenderMarkdown_LowAndInfoSeverityBadges(t *testing.T) {
	f := &Findings{
		Tool: "tfsec",
		Findings: []Finding{
			{RuleID: "R1", Severity: SeverityLow, Message: "low finding"},
			{RuleID: "R2", Severity: SeverityInfo, Message: "info finding"},
		},
	}
	out := RenderMarkdown(f, RenderMarkdownOptions{})
	assert.Contains(t, out, "LOW-1-yellow")
	assert.Contains(t, out, "INFO-1-blue")
}
