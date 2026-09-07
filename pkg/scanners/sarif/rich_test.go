package sarif

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRichDiagnosticsRespectsFindingLimit(t *testing.T) {
	findings := &Findings{Findings: []Finding{
		{RuleID: "low", Severity: SeverityLow, File: "low.tf", Line: 1},
		{RuleID: "high", Severity: SeverityHigh, File: "high.tf", Line: 2},
	}}

	diagnostics := richDiagnostics(findings, "tflint", 1)
	assert.Len(t, diagnostics, 1)
	assert.Equal(t, "high", diagnostics[0].RuleID)
	assert.Equal(t, "error", string(diagnostics[0].Severity))

	diagnostics = richDiagnostics(findings, "tflint", -1)
	assert.Len(t, diagnostics, 2)
}
