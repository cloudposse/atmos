package hooks

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/ci"
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

	rule := run["tool"].(map[string]any)["driver"].(map[string]any)["rules"].([]any)[0].(map[string]any)
	defaultConfig := rule["defaultConfiguration"].(map[string]any)
	assert.Equal(t, "warning", defaultConfig["level"])
	assert.Equal(t, 8.9, rule["properties"].(map[string]any)["security-severity"], "security metadata must be preserved")
}

func TestDeriveSARIFCategory(t *testing.T) {
	// Nil/empty inputs return empty (no category).
	assert.Equal(t, "", deriveSARIFCategory(nil))
	assert.Equal(t, "", deriveSARIFCategory(&ExecContext{}))

	// Shared in-repo source (no per-stack workdir resolves) → component-only,
	// so the same finding dedups across every stack that uses the component.
	ctx := &ExecContext{
		AtmosConfig: &schema.AtmosConfiguration{TerraformDirAbsolutePath: t.TempDir()},
		Info:        &schema.ConfigAndStacksInfo{Stack: "plat-ue2-prod", ComponentFromArg: "bucket"},
	}
	assert.Equal(t, "atmos/bucket", deriveSARIFCategory(ctx))
}
