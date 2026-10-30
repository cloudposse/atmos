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

// PR #3537 had passing live policy and a successful milestone status, but a
// cancelled repository-wide reconciliation still made its quality report red.
func TestQualityReleaseReconciliationUsesCurrentVerdict(t *testing.T) {
	for _, conclusion := range []string{"cancelled", "failure", "queued"} {
		t.Run(conclusion, func(t *testing.T) {
			state := &stateFixture{labels: []string{qualityNotReady}}
			q := state.client(t)
			api := q.api.(*qualityGraphAPI).FakeAPI
			get := api.Get
			status := "success"
			api.Get = func(path string, response any) error {
				switch {
				case strings.Contains(path, "actions/runs?"):
					run := workflowResultFixture(20, 200, conclusion)
					run["path"], run["event"] = ".github/workflows/release-milestones.yml", "pull_request_target"
					if conclusion == "queued" {
						run["status"] = "queued"
					}
					return githubtest.Respond(map[string]any{"workflow_runs": []any{workflowResultFixture(10, 100, "success"), run}}, response)
				case strings.Contains(path, "/check-runs?"):
					var checks struct {
						Checks []map[string]any `json:"check_runs"`
					}
					if err := get(path, &checks); err != nil {
						return err
					}
					checks.Checks = append(checks.Checks, workflowCheckFixture(200, "enforce", "failure"), workflowCheckFixture(200, "previews", "failure"))
					return githubtest.Respond(checks, response)
				case strings.Contains(path, "/statuses?"):
					return githubtest.Respond([]map[string]string{{"context": "Release milestone", "state": status}}, response)
				default:
					return get(path, response)
				}
			}
			pr := state.pr()
			complete, err := q.reportingComplete(&pr)
			require.NoError(t, err)
			require.True(t, complete, "repository reconciliation must not delay the current PR report")
			require.NoError(t, q.refresh(pr.Number, "", nil))
			require.Len(t, state.comments, 1)
			require.Contains(t, state.comments[0], "Quality Gate passed")
			require.NotContains(t, state.comments[0], "release-milestones.yml")
			state.comment, state.labels = state.comments[0], []string{qualityReady}
			state.comments, state.writes = nil, nil
			status = "failure"
			require.NoError(t, q.refresh(pr.Number, "", nil))
			require.Len(t, state.comments, 1)
			require.Contains(t, state.comments[0], "Quality Gate not ready")
			require.Contains(t, state.comments[0], "status: Release milestone: failure")
			require.Contains(t, state.writes, "DELETE repos/"+githubtest.Repo+"/issues/42/labels/"+qualityReady)
		})
	}
}

func TestQualityScheduleRecoversChangedBlockingStatus(t *testing.T) {
	cutoff := time.Date(2026, 10, 26, 0, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		name, current string
		want          bool
	}{
		{"recovered", "success", true},
		{"unchanged", "failure", false},
		{"retry pending", "pending", true},
		{"removed", "", true},
		{"unavailable", "error", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := &scheduleServer{prs: []schedulePR{{number: 42, labeled: true, notReady: true, updated: cutoff.Add(-time.Hour)}}}
			pr := server.listed(&server.prs[0])
			gate := qualityCIGate(map[string]string{"status: Release milestone": "failure", "status: external-ci": "failure"})
			gate.ID, gate.Name = "ci", "All CI checks and workflows"
			body, ready := qualityMarkdown(&pr, []qualityGate{gate}, "")
			require.False(t, ready)
			reads := 0
			q := qualityTestClient(t, func(path string, response any) error {
				switch {
				case strings.Contains(path, "/comments?"):
					comment := githubapi.Comment{ID: 17, Body: body}
					comment.User.Login, comment.User.Type = "github-actions[bot]", "Bot"
					return githubtest.Respond([]githubapi.Comment{comment}, response)
				case strings.Contains(path, "/status?"):
					reads++
					if tc.name == "unavailable" {
						return errors.New("status API unavailable")
					}
					statuses := []map[string]string{{"context": "external-ci", "state": "failure"}}
					if tc.current != "" {
						statuses = append(statuses, map[string]string{"context": "Release milestone", "state": tc.current})
					}
					return githubtest.Respond(map[string]any{"state": "failure", "statuses": statuses}, response)
				default:
					return server.get(path, response)
				}
			})
			reason, err := q.scheduledReason(&pr, cutoff)
			if tc.name == "unavailable" {
				require.ErrorContains(t, err, "status API unavailable")
			} else {
				require.NoError(t, err)
			}
			require.Equal(t, tc.want, reason != "", reason)
			require.Equal(t, 1, reads, "check current contexts once, not the full status history or one request per blocker")
			require.Empty(t, server.commits, "old heads do not need a commit-date lookup")
		})
	}
}

func TestQualityReleaseWorkflowKeepsUnprovenFailures(t *testing.T) {
	for _, scenario := range []string{"unrelated suite", "unknown suite", "external app", "shared suite", "untrusted event", "missing event", "missing run ID", "missing suite ID", "no PR CI"} {
		t.Run(scenario, func(t *testing.T) {
			run := workflowResultFixture(20, 200, "cancelled")
			run["path"], run["event"] = releaseMilestonesWorkflow, "pull_request_target"
			runs := []map[string]any{workflowResultFixture(10, 100, "success"), run}
			check := workflowCheckFixture(200, "enforce", "failure")
			switch scenario {
			case "unrelated suite":
				check["check_suite"] = map[string]any{"id": 300}
			case "unknown suite":
				delete(check, "check_suite")
			case "external app":
				check["app"] = map[string]any{"slug": "another-app"}
			case "shared suite":
				runs[0]["check_suite_id"] = 200
			case "untrusted event":
				run["event"] = "pull_request"
			case "missing event":
				delete(run, "event")
			case "missing run ID":
				delete(run, "id")
			case "missing suite ID":
				delete(run, "check_suite_id")
			case "no PR CI":
				runs = runs[1:]
			}
			q := workflowFixtureClient(t, runs, []map[string]any{check}, false)
			pr := githubtest.PR(githubtest.Run())
			pr.MergeSHA = ""
			gate, err := q.tests(&pr)
			require.NoError(t, err)
			require.False(t, gate.Passed, gate.Detail)
		})
	}
}

// The reporter's own run is cancelled whenever a newer event replaces it while
// pending; that must never block the report it would have produced.
func TestQualityReportWorkflowRunIsBackgroundReconciliation(t *testing.T) {
	for _, workflow := range []string{qualityReportWorkflow, qualityReconcileWorkflow} {
		for _, conclusion := range []string{"cancelled", "queued"} {
			t.Run(workflow+"/"+conclusion, func(t *testing.T) {
				run := workflowResultFixture(20, 200, conclusion)
				run["path"], run["event"] = workflow, "pull_request_target"
				if conclusion == "queued" {
					run["status"] = "queued"
				}
				q := workflowFixtureClient(t, []map[string]any{workflowResultFixture(10, 100, "success"), run}, nil, false)
				pr := githubtest.PR(githubtest.Run())
				pr.MergeSHA = ""
				complete, err := q.reportingComplete(&pr)
				require.NoError(t, err)
				require.True(t, complete, "the reporter's own run must not delay its report")
				gate, err := q.tests(&pr)
				require.NoError(t, err)
				require.True(t, gate.Passed, gate.Detail)
				require.NotContains(t, gate.Detail, workflow)
			})
		}
	}
}

// A PR can define a workflow at the reporter's path; that run stays a blocker.
func TestQualityReportWorkflowUntrustedRunStillBlocks(t *testing.T) {
	for _, workflow := range []string{qualityReportWorkflow, qualityReconcileWorkflow} {
		t.Run(workflow, func(t *testing.T) {
			run := workflowResultFixture(20, 200, "failure")
			run["path"], run["event"] = workflow, "pull_request"
			q := workflowFixtureClient(t, []map[string]any{workflowResultFixture(10, 100, "success"), run}, nil, false)
			pr := githubtest.PR(githubtest.Run())
			pr.MergeSHA = ""
			gate, err := q.tests(&pr)
			require.NoError(t, err)
			require.False(t, gate.Passed, gate.Detail)
		})
	}
}
