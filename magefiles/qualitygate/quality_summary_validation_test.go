//go:build mage

package qualitygate

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestQualityMarkdownOmitsInvalidSummaryRun(t *testing.T) {
	for _, runID := range []string{"", "invalid", "0", "-1", "9223372036854775808"} {
		t.Run("run="+runID, func(t *testing.T) {
			t.Setenv("GITHUB_STEP_SUMMARY", filepath.Join(t.TempDir(), "summary.md"))
			t.Setenv("GITHUB_RUN_ID", runID)
			state := &stateFixture{}
			pr := state.pr()

			body, ready := qualityMarkdown(&pr, []qualityGate{{Name: "CI", Passed: true}}, "")

			require.True(t, ready, "summary metadata must not affect readiness")
			require.Contains(t, body, "Quality Gate passed")
			require.NotContains(t, body, "View job summary", "invalid run IDs must not produce broken summary links")
		})
	}
}
