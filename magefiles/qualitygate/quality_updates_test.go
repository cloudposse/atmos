//go:build mage

package qualitygate

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/magefiles/internal/githubapi"
	"github.com/cloudposse/atmos/magefiles/internal/githubtest"
)

func TestQualityIncompleteCIWaitsForFinalReport(t *testing.T) {
	for _, conclusion := range []string{"success", "failure"} {
		t.Run(conclusion, func(t *testing.T) {
			state := &stateFixture{labels: []string{qualityNotReady}}
			q := state.client(t)
			api := q.api.(*qualityGraphAPI).FakeAPI
			get := api.Get
			run := githubtest.Run()
			run.Status, run.Conclusion = "in_progress", ""
			checkState := "queued"
			api.Get = func(path string, response any) error {
				switch {
				case strings.Contains(path, "actions/runs?"):
					return githubtest.Respond(map[string]any{"workflow_runs": []githubapi.Run{run}}, response)
				case strings.Contains(path, "actions/workflows/"):
					return githubtest.Respond(map[string]any{"workflow_runs": []githubapi.Run{}}, response)
				case strings.Contains(path, "/check-runs?"):
					check := qualityCheck{Name: "test shard", HeadSHA: run.HeadSHA, Status: checkState, Conclusion: conclusion}
					coverage := qualityCheck{Name: "codecov/patch", HeadSHA: run.HeadSHA, Status: "completed", Conclusion: "success"}
					coverage.App.Slug = "codecov"
					return githubtest.Respond(map[string]any{"check_runs": []qualityCheck{check, coverage}}, response)
				default:
					return get(path, response)
				}
			}

			require.NoError(t, q.reconcile(42))
			require.Len(t, state.comments, 1, "publish an initial report before CI completes")
			state.comment = state.comments[0]
			initial := state.comment
			state.writes, state.comments = nil, nil

			// Later job and workflow completions change the evidence, but CI as a
			// whole remains incomplete. Neither comments nor labels should churn.
			for _, progress := range []string{"in_progress", "completed"} {
				checkState = progress
				require.NoError(t, q.reconcile(42))
				require.Empty(t, state.writes)
			}

			run.Status, run.Conclusion = "completed", conclusion
			require.NoError(t, q.reconcile(42))
			require.Len(t, state.comments, 1, "publish final evidence even if readiness stays false")
			require.NotEqual(t, initial, state.comments[0])
			if conclusion == "failure" {
				require.Contains(t, state.comments[0], "test shard: failure")
				require.Contains(t, state.comments[0], "Quality Gate not ready")
			} else {
				require.Contains(t, state.comments[0], "Quality Gate passed")
			}
		})
	}
}

func TestQualityNewSummaryRunDoesNotRewriteUnchangedReport(t *testing.T) {
	state := &stateFixture{labels: []string{qualityReady}}
	q := state.client(t)
	summary := filepath.Join(t.TempDir(), "summary.md")
	t.Setenv("GITHUB_STEP_SUMMARY", summary)
	t.Setenv("GITHUB_RUN_ID", "100")
	pr := state.pr()
	body, ready, _ := q.render(&pr, nil, "", nil)
	require.True(t, ready)
	state.comment = body
	t.Setenv("GITHUB_RUN_ID", "200")

	require.NoError(t, q.refresh(42, "", nil))
	require.Empty(t, state.writes, "a new reporter run is not a change in PR evidence")
	saved, err := os.ReadFile(summary)
	require.NoError(t, err)
	require.Contains(t, string(saved), "/actions/runs/200#summary", "each invocation still produces a job summary")

	// A real change must publish with the new summary link.
	require.NoError(t, q.refresh(42, "Updated benchmark evidence", nil))
	require.Len(t, state.comments, 1)
	require.Contains(t, state.comments[0], "Updated benchmark evidence")
	require.Contains(t, state.comments[0], "/actions/runs/200#summary")
}

func TestQualityPendingSnapshotDoesNotHideRevisionChanges(t *testing.T) {
	for _, change := range []string{"head", "base", "target"} {
		t.Run(change, func(t *testing.T) {
			state := &stateFixture{labels: []string{qualityNotReady}}
			q := state.client(t)
			q.ciIncomplete = true
			pr := state.pr()
			previous := pr
			switch change {
			case "head":
				previous.Head.SHA = githubtest.Base
			case "base":
				previous.Base.SHA = githubtest.SHA
			case "target":
				previous.Base.Ref = "release"
			}
			body, _ := qualityMarkdown(&previous, []qualityGate{{Name: "CI"}}, "")
			state.comment = body + "\n" + qualityCIPending

			require.NoError(t, q.refresh(42, "", nil))
			require.Len(t, state.comments, 1)
			require.Contains(t, state.comments[0], qualityHeadMarker(pr.Head.SHA))
			require.Contains(t, state.comments[0], qualityBaseMarker(&pr))
			require.Contains(t, state.comments[0], "Quality Gate not ready")
		})
	}
}
