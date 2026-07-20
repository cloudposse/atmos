package validation

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReportSARIFIncludesActionlintDiagnostic(t *testing.T) {
	report := Report{Diagnostics: []Diagnostic{{
		Source:   "github-actions",
		RuleID:   "syntax-check",
		Severity: SeverityError,
		Message:  "unexpected key",
		File:     ".github/workflows/test.yml",
		Line:     4,
		Column:   5,
	}}}

	body, err := report.SARIF()
	require.NoError(t, err)
	var document map[string]any
	require.NoError(t, json.Unmarshal(body, &document))
	assert.Equal(t, "2.1.0", document["version"])
	assert.Contains(t, string(body), `"ruleId": "syntax-check"`)
	assert.Contains(t, string(body), `"startLine": 4`)
}
