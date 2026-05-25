package sarif

import (
	"errors"
	"testing"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParse_EmptyInput(t *testing.T) {
	f, err := Parse(nil)
	require.NoError(t, err)
	assert.Equal(t, 0, f.Count())
}

func TestParse_InvalidJSON(t *testing.T) {
	_, err := Parse([]byte("{not-json"))
	require.Error(t, err)
	assert.True(t, errors.Is(err, errUtils.ErrParseFile), "expected ErrParseFile, got %v", err)
}

func TestParse_LevelBasedSeverity(t *testing.T) {
	// Minimal SARIF for a tool that conveys severity via the `level`
	// field alone (no properties.severity, no security-severity). This
	// is the simplest valid SARIF shape and keeps the parser independent
	// from any one scanner's richer conventions.
	data := []byte(`{
		"runs": [{
			"tool": {"driver": {"name": "trivy"}},
			"results": [
				{
					"ruleId": "aws-s3-enable-bucket-encryption",
					"level": "error",
					"message": {"text": "Bucket does not have encryption enabled"},
					"locations": [{
						"physicalLocation": {
							"artifactLocation": {"uri": "main.tf"},
							"region": {"startLine": 12}
						}
					}]
				},
				{
					"ruleId": "aws-s3-enable-versioning",
					"level": "warning",
					"message": {"text": "Bucket does not have versioning enabled"},
					"locations": [{
						"physicalLocation": {
							"artifactLocation": {"uri": "main.tf"},
							"region": {"startLine": 12}
						}
					}]
				}
			]
		}]
	}`)
	f, err := Parse(data)
	require.NoError(t, err)
	assert.Equal(t, "trivy", f.Tool)
	require.Equal(t, 2, f.Count())

	counts := f.CountsBySeverity()
	assert.Equal(t, 1, counts["high"])
	assert.Equal(t, 1, counts["medium"])

	// Highest severity (lowest enum value = most severe).
	assert.Equal(t, SeverityHigh, f.HighestSeverity())
}

func TestParse_CheckovLikeSARIF(t *testing.T) {
	// Checkov emits properties.severity strings.
	data := []byte(`{
		"runs": [{
			"tool": {"driver": {"name": "checkov"}},
			"results": [
				{
					"ruleId": "CKV_AWS_19",
					"level": "warning",
					"message": {"text": "Ensure all data stored in the S3 bucket is securely encrypted at rest"},
					"properties": {"severity": "HIGH"},
					"locations": [{
						"physicalLocation": {
							"artifactLocation": {"uri": "s3.tf"},
							"region": {"startLine": 7}
						}
					}]
				},
				{
					"ruleId": "CKV_AWS_18",
					"level": "warning",
					"message": {"text": "Ensure the S3 bucket has access logging enabled"},
					"properties": {"severity": "LOW"}
				}
			]
		}]
	}`)
	f, err := Parse(data)
	require.NoError(t, err)
	assert.Equal(t, "checkov", f.Tool)
	require.Equal(t, 2, f.Count())

	counts := f.CountsBySeverity()
	assert.Equal(t, 1, counts["high"])
	assert.Equal(t, 1, counts["low"])
}

func TestParse_TrivyLikeSARIF(t *testing.T) {
	// Trivy emits security-severity as a numeric string (GitHub convention).
	data := []byte(`{
		"runs": [{
			"tool": {"driver": {"name": "trivy"}},
			"results": [
				{
					"ruleId": "AVD-AWS-0089",
					"level": "error",
					"message": {"text": "Bucket has logging disabled"},
					"properties": {"security-severity": "8.5"}
				},
				{
					"ruleId": "AVD-AWS-0090",
					"level": "warning",
					"message": {"text": "Bucket policy too permissive"},
					"properties": {"security-severity": "9.5"}
				}
			]
		}]
	}`)
	f, err := Parse(data)
	require.NoError(t, err)
	counts := f.CountsBySeverity()
	assert.Equal(t, 1, counts["critical"], "security-severity >= 9.0 maps to critical")
	assert.Equal(t, 1, counts["high"], "security-severity >= 7.0 maps to high")
}

func TestParse_RuleLevelInheritance(t *testing.T) {
	// Results omit `level` and inherit from rule.defaultConfiguration.level.
	data := []byte(`{
		"runs": [{
			"tool": {
				"driver": {
					"name": "kics",
					"rules": [{
						"id": "K001",
						"defaultConfiguration": {"level": "error"}
					}]
				}
			},
			"results": [
				{
					"ruleId": "K001",
					"message": {"text": "Critical config issue"}
				}
			]
		}]
	}`)
	f, err := Parse(data)
	require.NoError(t, err)
	require.Equal(t, 1, f.Count())
	assert.Equal(t, SeverityHigh, f.Findings[0].Severity)
}

func TestParse_MultipleRuns(t *testing.T) {
	// Some tools emit one run per language; verify aggregation works.
	data := []byte(`{
		"runs": [
			{"tool": {"driver": {"name": "kics"}}, "results": [
				{"ruleId": "R1", "level": "error", "message": {"text": "a"}}
			]},
			{"tool": {"driver": {"name": "kics"}}, "results": [
				{"ruleId": "R2", "level": "warning", "message": {"text": "b"}}
			]}
		]
	}`)
	f, err := Parse(data)
	require.NoError(t, err)
	require.Equal(t, 2, f.Count())
	assert.Equal(t, "kics", f.Tool)
}

func TestParse_TolerantOfMissingFields(t *testing.T) {
	// Result with no level, no properties, no locations — should not crash
	// and should classify as info.
	data := []byte(`{
		"runs": [{
			"tool": {"driver": {"name": "minimal"}},
			"results": [
				{"ruleId": "BARE", "message": {"text": "x"}}
			]
		}]
	}`)
	f, err := Parse(data)
	require.NoError(t, err)
	require.Equal(t, 1, f.Count())
	assert.Equal(t, SeverityInfo, f.Findings[0].Severity)
	assert.Equal(t, 0, f.Findings[0].Line)
	assert.Empty(t, f.Findings[0].File)
}

func TestFindings_NilReceiverBehavior(t *testing.T) {
	var f *Findings

	assert.Equal(t, 0, f.Count())
	assert.Nil(t, f.CountsBySeverity())
	assert.Equal(t, SeverityInfo, f.HighestSeverity())
	assert.Nil(t, f.SortedBySeverity())
}

func TestFindings_SortedBySeverity(t *testing.T) {
	f := &Findings{
		Findings: []Finding{
			{RuleID: "C", Severity: SeverityLow},
			{RuleID: "A", Severity: SeverityCritical},
			{RuleID: "B", Severity: SeverityHigh},
			{RuleID: "D", Severity: SeverityMedium},
		},
	}
	sorted := f.SortedBySeverity()
	require.Len(t, sorted, 4)
	assert.Equal(t, "A", sorted[0].RuleID, "critical first")
	assert.Equal(t, "B", sorted[1].RuleID, "high second")
	assert.Equal(t, "D", sorted[2].RuleID, "medium third")
	assert.Equal(t, "C", sorted[3].RuleID, "low last")
}

func TestFindings_SortedBySeverityTieBreakers(t *testing.T) {
	f := &Findings{
		Findings: []Finding{
			{RuleID: "B", Severity: SeverityHigh, File: "b.tf", Line: 2},
			{RuleID: "A", Severity: SeverityHigh, File: "b.tf", Line: 2},
			{RuleID: "A", Severity: SeverityHigh, File: "a.tf", Line: 10},
			{RuleID: "A", Severity: SeverityHigh, File: "a.tf", Line: 1},
		},
	}

	sorted := f.SortedBySeverity()
	require.Len(t, sorted, 4)
	assert.Equal(t, []Finding{
		{RuleID: "A", Severity: SeverityHigh, File: "a.tf", Line: 1},
		{RuleID: "A", Severity: SeverityHigh, File: "a.tf", Line: 10},
		{RuleID: "A", Severity: SeverityHigh, File: "b.tf", Line: 2},
		{RuleID: "B", Severity: SeverityHigh, File: "b.tf", Line: 2},
	}, sorted)
}

func TestBestMessage_FallbackOrder(t *testing.T) {
	tests := []struct {
		name string
		res  rawResult
		rule rawRule
		want string
	}{
		{
			name: "uses rule short description first",
			res:  rawResult{Message: rawMessage{Text: "result message"}},
			rule: rawRule{
				ShortDescription: rawMessage{Text: " rule short "},
				FullDescription:  rawMessage{Text: "rule full"},
			},
			want: "rule short",
		},
		{
			name: "falls back to first non-empty result message line",
			res:  rawResult{Message: rawMessage{Text: "\n result line \n second line"}},
			rule: rawRule{FullDescription: rawMessage{Text: "rule full"}},
			want: "result line",
		},
		{
			name: "falls back to first non-empty full description line",
			res:  rawResult{},
			rule: rawRule{FullDescription: rawMessage{Text: "\n full line \n more"}},
			want: "full line",
		},
		{
			name: "returns empty string with no message fields",
			res:  rawResult{},
			rule: rawRule{},
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, bestMessage(&tt.res, &tt.rule))
		})
	}
}

func TestSeverityHelpers(t *testing.T) {
	t.Run("parseSecuritySeverityValue", func(t *testing.T) {
		tests := []struct {
			name string
			in   map[string]any
			want float64
			ok   bool
		}{
			{name: "nil map", in: nil, ok: false},
			{name: "missing key", in: map[string]any{}, ok: false},
			{name: "float value", in: map[string]any{"security-severity": 7.1}, want: 7.1, ok: true},
			{name: "string value", in: map[string]any{"security-severity": "4.2"}, want: 4.2, ok: true},
			{name: "invalid string", in: map[string]any{"security-severity": "high"}, ok: false},
			{name: "unsupported type", in: map[string]any{"security-severity": true}, ok: false},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				got, ok := parseSecuritySeverityValue(tt.in)
				assert.Equal(t, tt.ok, ok)
				assert.Equal(t, tt.want, got)
			})
		}
	})

	t.Run("bucketBySecuritySeverity", func(t *testing.T) {
		tests := []struct {
			score float64
			want  Severity
		}{
			{score: 9.0, want: SeverityCritical},
			{score: 7.0, want: SeverityHigh},
			{score: 4.0, want: SeverityMedium},
			{score: 0.1, want: SeverityLow},
			{score: 0, want: SeverityInfo},
		}

		for _, tt := range tests {
			assert.Equal(t, tt.want, bucketBySecuritySeverity(tt.score))
		}
	})

	t.Run("propertySeverity", func(t *testing.T) {
		tests := []struct {
			name string
			in   map[string]any
			want Severity
			ok   bool
		}{
			{name: "nil map", in: nil, want: SeverityInfo, ok: false},
			{name: "non-string severity", in: map[string]any{"severity": 1}, want: SeverityInfo, ok: false},
			{name: "critical", in: map[string]any{"severity": "critical"}, want: SeverityCritical, ok: true},
			{name: "high", in: map[string]any{"severity": "HIGH"}, want: SeverityHigh, ok: true},
			{name: "moderate maps to medium", in: map[string]any{"severity": "moderate"}, want: SeverityMedium, ok: true},
			{name: "informational maps to info", in: map[string]any{"severity": "informational"}, want: SeverityInfo, ok: true},
			{name: "note maps to info", in: map[string]any{"severity": "note"}, want: SeverityInfo, ok: true},
			{name: "unknown severity", in: map[string]any{"severity": "unknown"}, want: SeverityInfo, ok: false},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				got, ok := propertySeverity(tt.in)
				assert.Equal(t, tt.ok, ok)
				assert.Equal(t, tt.want, got)
			})
		}
	})

	t.Run("levelToSeverity", func(t *testing.T) {
		assert.Equal(t, SeverityHigh, levelToSeverity("error"))
		assert.Equal(t, SeverityMedium, levelToSeverity("warning"))
		assert.Equal(t, SeverityLow, levelToSeverity("note"))
		assert.Equal(t, SeverityInfo, levelToSeverity("none"))
		assert.Equal(t, SeverityInfo, levelToSeverity(""))
		assert.Equal(t, SeverityInfo, levelToSeverity("unexpected"))
	})
}

func TestRenderMarkdown_Empty(t *testing.T) {
	out := RenderMarkdown(&Findings{Tool: "trivy"}, RenderMarkdownOptions{})
	assert.Contains(t, out, "trivy")
	assert.Contains(t, out, "no findings")
}

func TestRenderMarkdown_WithFindings(t *testing.T) {
	f := &Findings{
		Tool: "checkov",
		Findings: []Finding{
			{RuleID: "CKV_AWS_19", Severity: SeverityHigh, Message: "Encrypt at rest", File: "main.tf", Line: 5},
			{RuleID: "CKV_AWS_18", Severity: SeverityLow, Message: "Access logging"},
		},
	}
	out := RenderMarkdown(f, RenderMarkdownOptions{})
	assert.Contains(t, out, "checkov")
	assert.Contains(t, out, "1 HIGH, 1 LOW")
	assert.Contains(t, out, "CKV_AWS_19")
	assert.Contains(t, out, "main.tf:5")
}

func TestRenderMarkdown_RespectsMaxFindings(t *testing.T) {
	findings := make([]Finding, 25)
	for i := range findings {
		findings[i] = Finding{
			RuleID:   "R",
			Severity: SeverityMedium,
			Message:  "x",
		}
	}
	out := RenderMarkdown(&Findings{Tool: "t", Findings: findings}, RenderMarkdownOptions{MaxFindings: 3})
	assert.Contains(t, out, "and 22 more")
}

func TestRenderMarkdown_EscapesPipesInMessages(t *testing.T) {
	f := &Findings{
		Tool: "t",
		Findings: []Finding{
			{RuleID: "R", Severity: SeverityLow, Message: "Use a|b instead", File: "f"},
		},
	}
	out := RenderMarkdown(f, RenderMarkdownOptions{})
	assert.Contains(t, out, `a\|b`, "pipes in messages must be escaped to keep tables valid")
}
