package hooks

import (
	"testing"

	"github.com/stretchr/testify/assert"

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
