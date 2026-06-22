package hooks

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/ci"
	"github.com/cloudposse/atmos/pkg/ci/providers/github"
	"github.com/cloudposse/atmos/pkg/schema"
)

func boolPtr(b bool) *bool { return &b }

// All CI reporting outputs require the ci.enabled master switch; within that,
// summary/annotations default ON and results defaults OFF.
func TestCIGatingHelpers(t *testing.T) {
	tests := []struct {
		name                    string
		ci                      schema.CIConfig
		summary, annot, results bool
	}{
		{name: "ci disabled → all off", ci: schema.CIConfig{Enabled: false}},
		{
			name:    "ci enabled, defaults (nil)",
			ci:      schema.CIConfig{Enabled: true},
			summary: true, annot: true, results: false,
		},
		{
			name: "explicit off summary+annotations, on results",
			ci: schema.CIConfig{
				Enabled:     true,
				Summary:     schema.CISummaryConfig{Enabled: boolPtr(false)},
				Annotations: schema.CIAnnotationsConfig{Enabled: boolPtr(false)},
				Results:     schema.CIResultsConfig{Enabled: boolPtr(true)},
			},
			summary: false, annot: false, results: true,
		},
		{
			name: "results enabled but ci disabled → still off",
			ci: schema.CIConfig{
				Enabled: false,
				Results: schema.CIResultsConfig{Enabled: boolPtr(true)},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := &ExecContext{AtmosConfig: &schema.AtmosConfiguration{CI: tt.ci}}
			assert.Equal(t, tt.summary, ciSummaryEnabled(ctx), "summary")
			assert.Equal(t, tt.annot, ciAnnotationsEnabled(ctx), "annotations")
			assert.Equal(t, tt.results, ciResultsEnabled(ctx), "results")
		})
	}

	// Nil context / config must be safe (false), never panic.
	assert.False(t, ciSummaryEnabled(nil))
	assert.False(t, ciAnnotationsEnabled(&ExecContext{}))
	assert.False(t, ciResultsEnabled(&ExecContext{AtmosConfig: &schema.AtmosConfiguration{}}))
}

func TestAnnotationLevelForSeverity(t *testing.T) {
	assert.Equal(t, ci.AnnotationError, annotationLevelForSeverity("critical"))
	assert.Equal(t, ci.AnnotationError, annotationLevelForSeverity("high"))
	assert.Equal(t, ci.AnnotationWarning, annotationLevelForSeverity("medium"))
	assert.Equal(t, ci.AnnotationWarning, annotationLevelForSeverity("low"))
	assert.Equal(t, ci.AnnotationWarning, annotationLevelForSeverity("info"))
	assert.Equal(t, ci.AnnotationWarning, annotationLevelForSeverity(""))
}

func TestAnnotationLevelForHook(t *testing.T) {
	warnCtx := &ExecContext{Hook: &Hook{OnFailure: OnFailureWarn}}
	assert.Equal(t, ci.AnnotationWarning, annotationLevelForHook(warnCtx, "critical"))
	assert.Equal(t, ci.AnnotationWarning, annotationLevelForHook(warnCtx, "high"))

	failCtx := &ExecContext{Hook: &Hook{OnFailure: OnFailureFail}}
	assert.Equal(t, ci.AnnotationError, annotationLevelForHook(failCtx, "critical"))
	assert.Equal(t, ci.AnnotationError, annotationLevelForHook(failCtx, "high"))
	assert.Equal(t, ci.AnnotationWarning, annotationLevelForHook(failCtx, "low"))
}

func TestNormalizeSARIFLevels(t *testing.T) {
	const body = `{
		"runs": [{
			"tool": {
				"driver": {
					"rules": [{
						"id": "CKV_AWS_21",
						"defaultConfiguration": {"level": "error"},
						"properties": {"security-severity": 8.9}
					}]
				}
			},
			"results": [{
				"ruleId": "CKV_AWS_21",
				"level": "error",
				"properties": {"security-severity": "8.9"},
				"message": {"text": "Ensure the S3 bucket has versioning enabled"}
			}]
		}]
	}`

	out := normalizeSARIFLevels([]byte(body), "warning")

	var doc map[string]any
	require.NoError(t, json.Unmarshal(out, &doc))
	run := doc["runs"].([]any)[0].(map[string]any)
	result := run["results"].([]any)[0].(map[string]any)
	assert.Equal(t, "warning", result["level"])
	assert.Equal(t, "CKV_AWS_21", result["ruleId"], "rule identity must be preserved")
	assert.Equal(t, nonBlockingSecuritySeverity, result["properties"].(map[string]any)["security-severity"], "result security severity must be non-blocking")

	rule := run["tool"].(map[string]any)["driver"].(map[string]any)["rules"].([]any)[0].(map[string]any)
	defaultConfig := rule["defaultConfiguration"].(map[string]any)
	assert.Equal(t, "warning", defaultConfig["level"])
	assert.Equal(t, nonBlockingSecuritySeverity, rule["properties"].(map[string]any)["security-severity"], "rule security severity must be non-blocking")
}

func TestDeriveSARIFCategory(t *testing.T) {
	// Nil/empty inputs return empty (no category).
	assert.Equal(t, "", deriveSARIFCategory(nil, nil))
	assert.Equal(t, "", deriveSARIFCategory(&ExecContext{}, nil))

	t.Run("uses SARIF tool driver name", func(t *testing.T) {
		const body = `{"runs":[{"tool":{"driver":{"name":"Trivy"}}}]}`
		ctx := &ExecContext{Hook: &Hook{Kind: "kics", Command: "kics"}}
		assert.Equal(t, "Trivy", deriveSARIFCategory(ctx, []byte(body)))
	})

	t.Run("falls back to hook kind", func(t *testing.T) {
		ctx := &ExecContext{Hook: &Hook{Kind: "kics", Command: "kics"}}
		assert.Equal(t, "kics", deriveSARIFCategory(ctx, []byte(`{"runs":[]}`)))
	})

	t.Run("falls back to hook command", func(t *testing.T) {
		ctx := &ExecContext{Hook: &Hook{Kind: "command", Command: "custom-scanner"}}
		assert.Equal(t, "custom-scanner", deriveSARIFCategory(ctx, []byte(`not-json`)))
	})

	// Final fallback: no SARIF tool name, empty Kind — returns the bare command.
	t.Run("falls back to command when kind is empty", func(t *testing.T) {
		ctx := &ExecContext{Hook: &Hook{Kind: "", Command: "only-command"}}
		assert.Equal(t, "only-command", deriveSARIFCategory(ctx, []byte(`{"runs":[]}`)))
	})
}

// reportsAsWarning is true only when a hook exists and its on-failure mode is
// not "fail" (warn/ignore downgrade findings to warnings); nil ctx/hook are safe.
func TestReportsAsWarning(t *testing.T) {
	assert.False(t, reportsAsWarning(nil), "nil ctx")
	assert.False(t, reportsAsWarning(&ExecContext{}), "nil hook")
	assert.False(t, reportsAsWarning(&ExecContext{Hook: &Hook{OnFailure: OnFailureFail}}), "fail mode")
	assert.True(t, reportsAsWarning(&ExecContext{Hook: &Hook{OnFailure: OnFailureWarn}}), "warn mode")
	assert.True(t, reportsAsWarning(&ExecContext{Hook: &Hook{OnFailure: OnFailureIgnore}}), "ignore mode")
}

// firstSARIFToolName extracts the first run's tool driver name, tolerating every
// shape of malformed/partial SARIF by returning "" rather than panicking.
func TestFirstSARIFToolName(t *testing.T) {
	tests := []struct {
		name  string
		sarif string
		want  string
	}{
		{name: "empty", sarif: "", want: ""},
		{name: "invalid json", sarif: "not-json", want: ""},
		{name: "runs missing", sarif: `{}`, want: ""},
		{name: "runs not array", sarif: `{"runs":{}}`, want: ""},
		{name: "run not map", sarif: `{"runs":["x"]}`, want: ""},
		{name: "tool missing", sarif: `{"runs":[{}]}`, want: ""},
		{name: "driver missing", sarif: `{"runs":[{"tool":{}}]}`, want: ""},
		{name: "name missing", sarif: `{"runs":[{"tool":{"driver":{}}}]}`, want: ""},
		{name: "name blank", sarif: `{"runs":[{"tool":{"driver":{"name":"  "}}}]}`, want: ""},
		{name: "name present trimmed", sarif: `{"runs":[{"tool":{"driver":{"name":" Trivy "}}}]}`, want: "Trivy"},
		{
			name:  "skips empty run then finds name",
			sarif: `{"runs":[{},{"tool":{"driver":{"name":"kics"}}}]}`,
			want:  "kics",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, firstSARIFToolName([]byte(tt.sarif)))
		})
	}
}

// normalizeSARIFLevels must tolerate every malformed/partial SARIF shape and
// return the input unchanged, never panicking. It also creates a missing
// defaultConfiguration so the forced level always lands on each rule.
func TestNormalizeSARIFLevels_EdgeCases(t *testing.T) {
	// Inputs that can't be rewritten are returned verbatim.
	for _, tt := range []struct {
		name, sarif string
	}{
		{"empty", ""},
		{"empty level handled below", `{"runs":[]}`},
		{"invalid json", "not-json"},
		{"runs not array", `{"runs":{}}`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			assert.NotPanics(t, func() { normalizeSARIFLevels([]byte(tt.sarif), "warning") })
		})
	}

	// Empty level short-circuits and returns the input byte-for-byte.
	in := []byte(`{"runs":[{"results":[{"level":"error"}]}]}`)
	assert.Equal(t, in, normalizeSARIFLevels(in, ""))

	// Non-map runs/results/rules entries are skipped without error; a rule with
	// no defaultConfiguration gets one created and leveled.
	const body = `{
		"runs": [
			"not-a-map",
			{
				"results": ["not-a-map", {"level": "error"}],
				"tool": {"driver": {"rules": [
					"not-a-map",
					{"id": "R1"}
				]}}
			}
		]
	}`
	out := normalizeSARIFLevels([]byte(body), "warning")
	var doc map[string]any
	require.NoError(t, json.Unmarshal(out, &doc))
	run := doc["runs"].([]any)[1].(map[string]any)
	result := run["results"].([]any)[1].(map[string]any)
	assert.Equal(t, "warning", result["level"])
	rule := run["tool"].(map[string]any)["driver"].(map[string]any)["rules"].([]any)[1].(map[string]any)
	// defaultConfiguration was absent and must be created with the forced level.
	assert.Equal(t, "warning", rule["defaultConfiguration"].(map[string]any)["level"])
}

// emitCIAnnotations maps a summary's findings to provider annotations when CI
// annotations are enabled, and is a safe no-op otherwise.
func TestEmitCIAnnotations(t *testing.T) {
	restore := ci.SwapRegistryForTest()
	defer restore()
	ci.Register(github.NewProvider())
	t.Setenv("GITHUB_ACTIONS", "true")

	out := &Output{Summary: &Summary{
		Kind: "checkov",
		Findings: []Finding{
			{Path: "main.tf", Line: 6, Severity: "high", RuleID: "CKV_AWS_19", Message: "encrypt the bucket"},
			{Path: "main.tf", Line: 9, Severity: "low", RuleID: "CKV_AWS_21", Message: "enable versioning"},
		},
	}}
	assert.NotPanics(t, func() { emitCIAnnotations(ciEnabledCtx(), out) })

	// No-ops: nil output, empty findings, and annotations disabled.
	assert.NotPanics(t, func() {
		emitCIAnnotations(ciEnabledCtx(), nil)
		emitCIAnnotations(ciEnabledCtx(), &Output{Summary: &Summary{Kind: "checkov"}})
	})
	disabled := &ExecContext{
		Hook:        &Hook{Kind: "checkov"},
		AtmosConfig: &schema.AtmosConfiguration{CI: schema.CIConfig{Enabled: true, Annotations: schema.CIAnnotationsConfig{Enabled: boolPtr(false)}}},
	}
	assert.NotPanics(t, func() { emitCIAnnotations(disabled, out) })
}

// publishCIResults uploads SARIF when ci.results is enabled, normalizing levels
// to warnings for non-failing hooks, and is a safe best-effort no-op otherwise.
func TestPublishCIResults(t *testing.T) {
	restore := ci.SwapRegistryForTest()
	defer restore()
	ci.Register(github.NewProvider())
	t.Setenv("GITHUB_ACTIONS", "true")

	const sarif = `{"version":"2.1.0","runs":[{"tool":{"driver":{"name":"checkov"}},` +
		`"results":[{"ruleId":"CKV_AWS_21","level":"error"}]}]}`

	resultsCtx := func(onFailure string) *ExecContext {
		return &ExecContext{
			Hook:        &Hook{Kind: "checkov", OnFailure: onFailure},
			AtmosConfig: &schema.AtmosConfiguration{CI: schema.CIConfig{Enabled: true, Results: schema.CIResultsConfig{Enabled: boolPtr(true)}}},
			Info:        &schema.ConfigAndStacksInfo{Stack: "test", ComponentFromArg: "bucket"},
		}
	}

	// Best-effort: a missing upload client is swallowed, never panics. Exercises
	// both the warning-normalization branch (warn) and the verbatim branch (fail).
	assert.NotPanics(t, func() {
		publishCIResults(resultsCtx(OnFailureWarn), &Output{Summary: &Summary{Kind: "checkov", SARIF: []byte(sarif)}})
		publishCIResults(resultsCtx(OnFailureFail), &Output{Summary: &Summary{Kind: "checkov", SARIF: []byte(sarif)}})
	})

	// No-ops: nil output, empty SARIF, and results disabled (the default).
	assert.NotPanics(t, func() {
		publishCIResults(resultsCtx(OnFailureFail), nil)
		publishCIResults(resultsCtx(OnFailureFail), &Output{Summary: &Summary{Kind: "checkov"}})
		publishCIResults(ciEnabledCtx(), &Output{Summary: &Summary{Kind: "checkov", SARIF: []byte(sarif)}})
	})
}
