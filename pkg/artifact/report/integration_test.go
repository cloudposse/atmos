package report_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/artifact/report"
	"github.com/cloudposse/atmos/pkg/ci"
	"github.com/cloudposse/atmos/pkg/ci/providers/generic"
	"github.com/cloudposse/atmos/pkg/schema"
)

// ciFiles points the generic provider at temporary summary and output files.
func ciFiles(t *testing.T) (summary, outputs string) {
	t.Helper()
	dir := t.TempDir()
	summary, outputs = filepath.Join(dir, "summary.md"), filepath.Join(dir, "output.txt")
	t.Setenv("ATMOS_CI_SUMMARY", summary)
	t.Setenv("ATMOS_CI_OUTPUT", outputs)
	return summary, outputs
}

// realSession builds a session whose reporter is a real ci.Reporter bound to the generic provider,
// so the ci.* gates and the file-backed writers apply.
func realSession(config *schema.AtmosConfiguration) *report.Session {
	return report.NewSession(config, report.WithReporter(func(cfg *schema.AtmosConfiguration) ci.Reporter {
		return ci.NewReporterForProvider(cfg, generic.NewProvider(), nil)
	}))
}

func read(t *testing.T, path string) string {
	t.Helper()
	content, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return ""
	}
	require.NoError(t, err)
	return string(content)
}

func TestReportIsAppendedToExistingSummaryContent(t *testing.T) {
	summary, outputs := ciFiles(t)
	require.NoError(t, os.WriteFile(summary, []byte("## Earlier step\n\nunrelated content\n"), 0o644))
	session := realSession(enabledConfig())
	session.Record(entry("publish", "a", 1, published("a", "components/vpc:1.2.0"), nil))
	require.NoError(t, session.Flush())

	content := read(t, summary)
	assert.Contains(t, content, "## Earlier step\n\nunrelated content\n", "existing job summary content remains")
	assert.Contains(t, content, "### ✅ 1. Publish `components/vpc:1.2.0`")
	assert.Less(t, strings.Index(content, "unrelated content"), strings.Index(content, "## Artifacts"))
	assert.Contains(t, read(t, outputs), "artifact_ref=components/vpc:1.2.0\n")
	assert.Contains(t, read(t, outputs), "artifact_publish_resolved_digest=sha256:")
}

func TestReportGatesWithTheRealReporter(t *testing.T) {
	cases := []struct {
		name        string
		config      *schema.AtmosConfiguration
		wantSummary bool
		wantOutputs bool
	}{
		{"ci disabled", &schema.AtmosConfiguration{}, false, false},
		{"summaries disabled", &schema.AtmosConfiguration{CI: schema.CIConfig{Enabled: true, Summary: schema.CISummaryConfig{Enabled: ptr(false)}}}, false, true},
		{"outputs disabled", &schema.AtmosConfiguration{CI: schema.CIConfig{Enabled: true, Output: schema.CIOutputConfig{Enabled: ptr(false)}}}, true, false},
		{"enabled", enabledConfig(), true, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			summary, outputs := ciFiles(t)
			session := realSession(tc.config)
			session.Record(entry("publish", "a", 1, published("a", "components/vpc:1.2.0"), nil))
			require.NoError(t, session.Flush())
			assert.Equal(t, tc.wantSummary, read(t, summary) != "")
			assert.Equal(t, tc.wantOutputs, read(t, outputs) != "")
		})
	}
}

func TestDefaultReporterHonorsForcedCIMode(t *testing.T) {
	summary, outputs := ciFiles(t)
	// A real CI runner advertises its own provider, which would receive the output instead of the files.
	t.Setenv("GITHUB_ACTIONS", "")
	t.Setenv("ATMOS_CI", "true")
	session := report.NewSession(enabledConfig())
	session.Record(entry("publish", "a", 1, published("a", "components/vpc:1.2.0"), nil))
	require.NoError(t, session.Flush())
	assert.Contains(t, read(t, summary), "## Artifacts")
	assert.Contains(t, read(t, outputs), "artifact_outcome=created")
}

func TestUnconfiguredCIWritesNothing(t *testing.T) {
	summary, outputs := ciFiles(t)
	// A real CI runner advertises its own provider, which would receive the output instead of the files.
	t.Setenv("GITHUB_ACTIONS", "")
	t.Setenv("ATMOS_CI", "true")
	session := report.NewSession(&schema.AtmosConfiguration{})
	session.Record(entry("publish", "a", 1, published("a", "components/vpc:1.2.0"), nil))
	require.NoError(t, session.Flush())
	assert.Empty(t, read(t, summary))
	assert.Empty(t, read(t, outputs))
}
