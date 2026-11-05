//go:build mage

package qualitygate

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/magefiles/internal/githubapi"
	"github.com/cloudposse/atmos/magefiles/internal/githubtest"
)

// TestQualityScheduleRecoversOldHeadReruns covers missed workflow events even
// when neither the PR's update date nor its head commit is recent.
func TestQualityScheduleRecoversOldHeadReruns(t *testing.T) {
	now := time.Date(2026, 10, 22, 15, 0, 0, 0, time.UTC)
	old := now.Add(-48 * time.Hour)
	for _, updated := range []time.Time{old, now} {
		for _, state := range []string{"queued", "in_progress", "waiting", "pending", "requested", "pending_deployment", "success", "failure", "cancelled", "empty", "other head", "reporter", "API error"} {
			t.Run(updated.Format(time.RFC3339)+"/"+state, func(t *testing.T) {
				server := &scheduleServer{prs: []schedulePR{{number: 42, labeled: true, updated: updated, committed: old}}}
				pr := server.listed(&server.prs[0])
				workflowReads := 0
				q := qualityTestClient(t, func(path string, response any) error {
					if !strings.Contains(path, "/actions/runs?") {
						return server.get(path, response)
					}
					workflowReads++
					require.Contains(t, path, "head_sha="+pr.Head.SHA)
					if state == "API error" {
						return errors.New("workflows unavailable")
					}
					run := githubtest.Run()
					run.HeadSHA = pr.Head.SHA
					run.Status, run.Conclusion = state, ""
					switch state {
					case "success", "failure", "cancelled":
						run.Status, run.Conclusion = "completed", state
					case "other head":
						run.HeadSHA, run.Status = githubtest.Base, "in_progress"
					case "reporter":
						run.Event, run.Status = "workflow_run", "in_progress"
					}
					runs := []githubapi.Run{run}
					if state == "empty" {
						runs = nil
					}
					return githubtest.Respond(map[string]any{"workflow_runs": runs}, response)
				})
				reason, err := q.scheduledReason(&pr, now.Add(-24*time.Hour))
				require.Equal(t, 1, workflowReads)
				if state == "API error" {
					require.ErrorContains(t, err, "workflows unavailable")
					require.Equal(t, qualityReasonUnknown, reason)
					return
				}
				require.NoError(t, err)
				switch state {
				case "queued", "in_progress", "waiting", "pending", "requested", "pending_deployment":
					require.Equal(t, qualityReasonPending, reason)
					require.Empty(t, server.commits)
				default:
					require.Equal(t, qualityReasonNone, reason)
				}
			})
		}
	}
}
