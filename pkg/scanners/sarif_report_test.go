package scanners

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

func boolPtr(b bool) *bool { return &b }

func TestReportsAsWarning(t *testing.T) {
	tests := []struct {
		name string
		scan *Context
		want bool
	}{
		{"nil scan is never treated as a warning", nil, false},
		{"empty on_failure defaults to warning", &Context{}, true},
		{"explicit warn", &Context{OnFailure: OnFailureWarn}, true},
		{"explicit ignore treated as warning", &Context{OnFailure: OnFailureIgnore}, true},
		{"explicit fail is not warning", &Context{OnFailure: OnFailureFail}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, reportsAsWarning(tt.scan))
		})
	}
}

func TestAnnotationLevelForSeverity(t *testing.T) {
	tests := []struct {
		severity string
		want     string
	}{
		{"critical", "error"},
		{"high", "error"},
		{"medium", "warning"},
		{"low", "warning"},
		{"", "warning"},
	}
	for _, tt := range tests {
		t.Run(tt.severity, func(t *testing.T) {
			assert.Equal(t, tt.want, string(annotationLevelForSeverity(tt.severity)))
		})
	}
}

func TestAnnotationLevelForScan(t *testing.T) {
	// on_failure=fail uses the severity-derived level.
	failScan := &Context{OnFailure: OnFailureFail}
	assert.Equal(t, "error", string(annotationLevelForScan(failScan, "critical")))
	assert.Equal(t, "warning", string(annotationLevelForScan(failScan, "low")))

	// Anything else (including empty/warn/ignore) always reports as a warning,
	// regardless of the finding's own severity.
	warnScan := &Context{OnFailure: OnFailureWarn}
	assert.Equal(t, "warning", string(annotationLevelForScan(warnScan, "critical")))
}

func TestDeriveSARIFCategory(t *testing.T) {
	sarifWithTool := []byte(`{"runs":[{"tool":{"driver":{"name":"tflint"}}}]}`)

	tests := []struct {
		name  string
		scan  *Context
		sarif []byte
		want  string
	}{
		{"tool name wins over scan name", &Context{Name: "should-not-be-used"}, sarifWithTool, "tflint"},
		{"nil scan with no tool name", nil, nil, ""},
		{"falls back to scan name", &Context{Name: "checkov"}, nil, "checkov"},
		{"falls back to scan command when name empty", &Context{Command: "trivy"}, nil, "trivy"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, deriveSARIFCategory(tt.scan, tt.sarif))
		})
	}
}

func TestFirstSARIFToolName(t *testing.T) {
	tests := []struct {
		name  string
		sarif []byte
		want  string
	}{
		{"empty input", nil, ""},
		{"invalid json", []byte("not json"), ""},
		{"no runs key", []byte(`{}`), ""},
		{"finds trimmed name", []byte(`{"runs":[{"tool":{"driver":{"name":"  tflint  "}}}]}`), "tflint"},
		{"skips run without name, uses next", []byte(`{"runs":[{"tool":{}},{"tool":{"driver":{"name":"checkov"}}}]}`), "checkov"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, firstSARIFToolName(tt.sarif))
		})
	}
}

func TestNormalizeSARIFLevels(t *testing.T) {
	t.Run("empty input returned unchanged", func(t *testing.T) {
		assert.Nil(t, normalizeSARIFLevels(nil, "warning"))
		assert.Equal(t, []byte("data"), normalizeSARIFLevels([]byte("data"), ""))
	})

	t.Run("invalid json returned unchanged", func(t *testing.T) {
		in := []byte("not json")
		assert.Equal(t, in, normalizeSARIFLevels(in, "warning"))
	})

	t.Run("no runs key returned unchanged", func(t *testing.T) {
		in := []byte(`{"foo":"bar"}`)
		assert.Equal(t, in, normalizeSARIFLevels(in, "warning"))
	})

	t.Run("downgrades result and rule levels and security-severity", func(t *testing.T) {
		in := []byte(`{
			"runs": [{
				"results": [{"level": "error", "properties": {"security-severity": "9.8"}}],
				"tool": {"driver": {"rules": [{"properties": {"security-severity": "9.8"}}]}}
			}]
		}`)

		out := normalizeSARIFLevels(in, "warning")
		require.NotEqual(t, in, out)

		var doc map[string]any
		require.NoError(t, json.Unmarshal(out, &doc))

		runs := doc["runs"].([]any)
		run := runs[0].(map[string]any)
		results := run["results"].([]any)
		result := results[0].(map[string]any)
		assert.Equal(t, "warning", result["level"])
		props := result["properties"].(map[string]any)
		assert.Equal(t, nonBlockingSecuritySeverity, props["security-severity"])

		rules := run["tool"].(map[string]any)["driver"].(map[string]any)["rules"].([]any)
		rule := rules[0].(map[string]any)
		defaultConfig := rule["defaultConfiguration"].(map[string]any)
		assert.Equal(t, "warning", defaultConfig["level"])
	})

	t.Run("malformed runs entries skipped without panic", func(t *testing.T) {
		in := []byte(`{"runs": ["not-an-object", {"results": "not-an-array", "tool": "not-an-object"}]}`)
		out := normalizeSARIFLevels(in, "warning")
		require.NotNil(t, out)
	})
}

func TestCIEnabledGates(t *testing.T) {
	t.Run("nil scan or config disables everything", func(t *testing.T) {
		assert.False(t, ciEnabled(nil))
		assert.False(t, ciEnabled(&Context{}))
		assert.False(t, ciSummaryEnabled(nil))
		assert.False(t, ciAnnotationsEnabled(nil))
		assert.False(t, ciResultsEnabled(nil))
	})

	t.Run("ci disabled short-circuits sub-gates", func(t *testing.T) {
		scan := &Context{AtmosConfig: &schema.AtmosConfiguration{}}
		assert.False(t, ciSummaryEnabled(scan))
		assert.False(t, ciAnnotationsEnabled(scan))
		assert.False(t, ciResultsEnabled(scan))
	})

	t.Run("summary and annotations default to enabled, results default to disabled", func(t *testing.T) {
		scan := &Context{AtmosConfig: &schema.AtmosConfiguration{}}
		scan.AtmosConfig.CI.Enabled = true

		assert.True(t, ciEnabled(scan))
		assert.True(t, ciSummaryEnabled(scan))
		assert.True(t, ciAnnotationsEnabled(scan))
		assert.False(t, ciResultsEnabled(scan))
	})

	t.Run("explicit false disables, explicit true enables results", func(t *testing.T) {
		scan := &Context{AtmosConfig: &schema.AtmosConfiguration{}}
		scan.AtmosConfig.CI.Enabled = true
		scan.AtmosConfig.CI.Summary.Enabled = boolPtr(false)
		scan.AtmosConfig.CI.Annotations.Enabled = boolPtr(false)
		scan.AtmosConfig.CI.Results.Enabled = boolPtr(true)

		assert.False(t, ciSummaryEnabled(scan))
		assert.False(t, ciAnnotationsEnabled(scan))
		assert.True(t, ciResultsEnabled(scan))
	})
}

func TestRenderCISummaryGuards(t *testing.T) {
	// Nil output, nil summary, and empty body all short-circuit before ci.WriteStepSummary.
	renderCISummary(&Context{}, nil)
	renderCISummary(&Context{}, &Output{})
	renderCISummary(&Context{}, &Output{Summary: &Summary{}})

	// CI disabled short-circuits even with a non-empty body.
	renderCISummary(&Context{AtmosConfig: &schema.AtmosConfiguration{}}, &Output{Summary: &Summary{Body: "hello"}})
}

func TestEmitCIAnnotationsGuards(t *testing.T) {
	emitCIAnnotations(&Context{}, nil)
	emitCIAnnotations(&Context{}, &Output{})
	emitCIAnnotations(&Context{}, &Output{Summary: &Summary{}})

	scan := &Context{AtmosConfig: &schema.AtmosConfiguration{}}
	emitCIAnnotations(scan, &Output{Summary: &Summary{Findings: []Finding{{Path: "a.tf", Line: 1}}}})
}

func TestPublishCIResultsGuards(t *testing.T) {
	publishCIResults(&Context{}, nil)
	publishCIResults(&Context{}, &Output{})
	publishCIResults(&Context{}, &Output{Summary: &Summary{}})

	scan := &Context{AtmosConfig: &schema.AtmosConfiguration{}}
	publishCIResults(scan, &Output{Summary: &Summary{SARIF: []byte(`{"runs":[]}`)}})
}

func TestRenderCISummaryUsesEnabledCISink(t *testing.T) {
	// The scanner package has no provider registered in its unit-test binary;
	// this validates that an enabled CI summary safely reaches the public CI seam
	// and remains a no-op when there is no active provider.
	t.Setenv("GITHUB_ACTIONS", "true")
	scan := &Context{AtmosConfig: &schema.AtmosConfiguration{}}
	scan.AtmosConfig.CI.Enabled = true

	renderCISummary(scan, &Output{Summary: &Summary{Body: "## scanner summary"}})
}

func TestEnabledCIReportersAcceptFindingsAndSARIFWithoutProvider(t *testing.T) {
	// A configured scan should build annotations and a SARIF report even outside
	// CI. The CI package then intentionally no-ops because no provider is found.
	t.Setenv("GITHUB_ACTIONS", "")
	results := true
	scan := &Context{AtmosConfig: &schema.AtmosConfiguration{}}
	scan.AtmosConfig.CI.Enabled = true
	scan.AtmosConfig.CI.Results.Enabled = &results
	out := &Output{Summary: &Summary{
		Findings: []Finding{{Path: "components/vpc/main.tf", Line: 7, Severity: "critical", RuleID: "terraform_unused", Message: "unused local"}},
		SARIF:    []byte(`{"runs":[{"tool":{"driver":{"name":"tflint"}},"results":[{"level":"error"}]}]}`),
	}}

	emitCIAnnotations(scan, out)
	publishCIResults(scan, out)
}
