//go:build mage

package qualitygate

import (
	"errors"
	"strings"
	"testing"

	"github.com/cloudposse/atmos/magefiles/internal/githubapi"
	"github.com/cloudposse/atmos/magefiles/internal/githubtest"
	"github.com/stretchr/testify/require"
)

func TestQualityDefersIncompleteWorkflows(t *testing.T) {
	for _, state := range []string{"queued", "in_progress", "waiting", "pending", "requested", "unknown", "", "no runs", "API failure"} {
		t.Run(state, func(t *testing.T) {
			run := githubtest.Run()
			run.Status = state
			q := qualityTestClient(t, func(path string, response any) error {
				if strings.HasSuffix(path, "/pulls/42") {
					return githubtest.Respond(githubtest.PR(run), response)
				}
				require.Contains(t, path, "actions/runs?", "incomplete CI must not fetch benchmark artifacts or publish")
				if state == "API failure" {
					return errors.New("API unavailable")
				}
				runs := []githubapi.Run{run}
				if state == "no runs" {
					runs = nil
				} else {
					// A successful older attempt must not hide a running rerun.
					runs = append(runs, githubtest.Run())
				}
				return githubtest.Respond(map[string]any{"workflow_runs": runs}, response)
			})
			err := q.reconcile(42)
			if state == "API failure" {
				require.ErrorContains(t, err, "API unavailable")
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestQualityCompletedFailuresAreReportable(t *testing.T) {
	for _, conclusion := range []string{"success", "failure", "cancelled", "skipped", "timed_out", "action_required"} {
		t.Run(conclusion, func(t *testing.T) {
			run := githubtest.Run()
			run.Conclusion = conclusion
			q := qualityTestClient(t, func(_ string, response any) error {
				return githubtest.Respond(map[string]any{"workflow_runs": []githubapi.Run{run}}, response)
			})
			pr := githubtest.PR(run)
			complete, err := q.reportingComplete(&pr)
			require.NoError(t, err)
			require.True(t, complete)
		})
	}
}
