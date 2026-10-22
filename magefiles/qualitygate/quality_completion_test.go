//go:build mage

package qualitygate

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/cloudposse/atmos/magefiles/internal/githubapi"
	"github.com/cloudposse/atmos/magefiles/internal/githubtest"
	"github.com/stretchr/testify/require"
)

func TestQualityRecognizesIncompleteWorkflows(t *testing.T) {
	for _, state := range []string{"queued", "in_progress", "waiting", "pending", "requested", "unknown", "", "no runs", "API failure"} {
		t.Run(state, func(t *testing.T) {
			run := githubtest.Run()
			run.Status = state
			q := qualityTestClient(t, func(path string, response any) error {
				require.Contains(t, path, "actions/runs?")
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
			pr := githubtest.PR(run)
			complete, err := q.reportingComplete(&pr)
			require.False(t, complete)
			if state == "API failure" {
				require.ErrorContains(t, err, "API unavailable")
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestQualityIncompleteCIRemovesStaleReadiness(t *testing.T) {
	for _, scenario := range []string{"new head", "same head rerun", "failed job while running", "no runs"} {
		t.Run(scenario, func(t *testing.T) {
			t.Setenv("GITHUB_STEP_SUMMARY", "")
			run := githubtest.Run()
			run.Status, run.Conclusion = "in_progress", ""
			pr := githubtest.PR(run)
			count := 2
			pr.ChangedFiles = &count
			previous := pr
			if scenario == "new head" {
				previous.Head.SHA = githubtest.Base
			}
			body, ready := qualityMarkdown(&previous, []qualityGate{{Name: "CI", Passed: true}}, "")
			require.True(t, ready)
			comment := githubapi.Comment{ID: 17, Body: body}
			comment.User.Login, comment.User.Type = "github-actions[bot]", "Bot"
			reads := 0
			get := qualityPublishGetter(t, "create", &pr, &run, &reads)
			fake := &githubtest.FakeAPI{T: t}
			fake.Get = func(path string, response any) error {
				require.NotContains(t, path, "actions/workflows/", "benchmark recovery must wait for CI")
				switch {
				case strings.Contains(path, "/comments?"):
					return githubtest.Respond([]githubapi.Comment{comment}, response)
				case strings.HasSuffix(path, "/issues/42"):
					return githubtest.Respond(map[string]any{"labels": []map[string]string{{"name": qualityReady}}}, response)
				case scenario == "no runs" && strings.Contains(path, "actions/runs?"):
					return githubtest.Respond(map[string]any{"workflow_runs": []githubapi.Run{}}, response)
				case scenario == "failed job while running" && strings.Contains(path, "/check-runs?"):
					return githubtest.Respond(map[string]any{"check_runs": []qualityCheck{{Name: "Acceptance tests", HeadSHA: pr.Head.SHA, Status: "completed", Conclusion: "failure"}}}, response)
				default:
					return get(path, response)
				}
			}
			var writes []string
			fake.Write = func(method, path string, reader io.Reader) error {
				writes = append(writes, method+" "+path)
				if method == http.MethodPatch {
					var payload map[string]string
					require.NoError(t, json.NewDecoder(reader).Decode(&payload))
					body = payload["body"]
				}
				if method == http.MethodPost {
					var payload struct{ Labels []string }
					require.NoError(t, json.NewDecoder(reader).Decode(&payload))
					require.Equal(t, []string{qualityNotReady}, payload.Labels)
				}
				return nil
			}
			q := qualityClient{ctx: context.Background(), api: &qualityGraphAPI{FakeAPI: fake, t: t, scenario: "valid"}, repo: githubtest.Repo}
			require.NoError(t, q.reconcile(pr.Number))
			require.Len(t, writes, 3, "remove ready, add not-ready, and replace the old report")
			require.Contains(t, writes[0], "DELETE repos/"+githubtest.Repo+"/issues/42/labels/quality:ready")
			require.Contains(t, body, "Quality Gate not ready")
			require.Contains(t, body, "| All CI checks and workflows | Not ready |")
			require.Contains(t, body, qualityHeadMarker(pr.Head.SHA))
			if scenario == "failed job while running" {
				require.Contains(t, body, "Acceptance tests: failure")
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
