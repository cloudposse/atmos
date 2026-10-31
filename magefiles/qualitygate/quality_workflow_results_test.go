//go:build mage

package qualitygate

import (
	"strings"
	"testing"
	"time"

	"github.com/cloudposse/atmos/magefiles/internal/githubapi"

	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/magefiles/internal/githubtest"
)

func TestQualityWorkflowResultsIgnoreSupersededMatrixPlaceholders(t *testing.T) {
	for _, newestFirst := range []bool{false, true} {
		t.Run(map[bool]string{false: "older first", true: "newer first"}[newestFirst], func(t *testing.T) {
			older := workflowResultFixture(10, 100, "cancelled")
			newer := workflowResultFixture(20, 200, "success")
			runs := []any{older, newer}
			if newestFirst {
				runs = []any{newer, older}
			}
			checks := []any{
				workflowCheckFixture(100, "Acceptance Tests (${{ matrix.os }})", "cancelled"),
				workflowCheckFixture(200, "Acceptance Tests (linux)", "success"),
			}
			q := qualityTestClient(t, func(path string, response any) error {
				switch {
				case strings.Contains(path, "actions/runs?"):
					return githubtest.Respond(map[string]any{"workflow_runs": runs}, response)
				case strings.Contains(path, "/check-runs?"):
					return githubtest.Respond(map[string]any{"check_runs": checks}, response)
				case strings.Contains(path, "/statuses?"):
					return githubtest.Respond([]any{}, response)
				default:
					t.Fatalf("unexpected request: %s", path)
					return nil
				}
			})
			pr := githubtest.PR(githubtest.Run())
			pr.MergeSHA = ""
			gate, err := q.tests(&pr)
			require.NoError(t, err)
			require.True(t, gate.Passed, gate.Detail)
			require.NotContains(t, gate.Detail, "matrix.os")
		})
	}
}

func workflowResultFixture(id, suite int64, conclusion string) map[string]any {
	return map[string]any{
		"id": id, "check_suite_id": suite, "head_sha": githubtest.SHA,
		"path": ".github/workflows/test.yml", "event": "pull_request",
		"status": "completed", "conclusion": conclusion, "created_at": "2026-10-25T18:18:20Z",
	}
}

func workflowCheckFixture(suite int64, name, conclusion string) map[string]any {
	return map[string]any{
		"id": suite, "head_sha": githubtest.SHA, "name": name,
		"status": "completed", "conclusion": conclusion,
		"check_suite": map[string]any{"id": suite}, "app": map[string]any{"slug": "github-actions"},
	}
}

func TestQualitySupersededSuitesKeepUnprovenAndCurrentFailures(t *testing.T) {
	cases := []struct {
		name   string
		change func([]map[string]any, []map[string]any) ([]map[string]any, []map[string]any)
	}{
		{"current failed", func(r, c []map[string]any) ([]map[string]any, []map[string]any) {
			r[1]["conclusion"] = "failure"
			c[1]["conclusion"] = "failure"
			return r, c
		}},
		{"current queued", func(r, c []map[string]any) ([]map[string]any, []map[string]any) {
			r[1]["status"] = "queued"
			r[1]["conclusion"] = ""
			return r, c[:1]
		}},
		{"old suite unknown", func(r, c []map[string]any) ([]map[string]any, []map[string]any) {
			r[0]["check_suite_id"] = 0
			return r, c
		}},
		{"new suite unknown", func(r, c []map[string]any) ([]map[string]any, []map[string]any) {
			r[1]["check_suite_id"] = 0
			return r, c
		}},
		{"old run unknown", func(r, c []map[string]any) ([]map[string]any, []map[string]any) { r[0]["id"] = 0; return r, c }},
		{"new run unknown", func(r, c []map[string]any) ([]map[string]any, []map[string]any) {
			r[1]["id"] = 0
			r[1]["created_at"] = "2026-10-25T19:18:20Z"
			return r, c
		}},
		{"shared suite", func(r, c []map[string]any) ([]map[string]any, []map[string]any) {
			r[1]["check_suite_id"] = 100
			return r, c
		}},
		{"unknown workflow path", func(r, c []map[string]any) ([]map[string]any, []map[string]any) {
			r[0]["path"] = ""
			r[1]["path"] = ""
			return r, c
		}},
		{"suite current in another workflow", func(r, c []map[string]any) ([]map[string]any, []map[string]any) {
			other := workflowResultFixture(30, 100, "success")
			other["path"] = "deploy.yml"
			return append(r, other), c
		}},
		{"external app", func(r, c []map[string]any) ([]map[string]any, []map[string]any) {
			c[0]["app"] = map[string]any{"slug": "external-checks"}
			return r, c
		}},
		{"unmapped check suite", func(r, c []map[string]any) ([]map[string]any, []map[string]any) {
			c[0]["check_suite"] = map[string]any{"id": 999}
			return r, c
		}},
		{"check suite absent", func(r, c []map[string]any) ([]map[string]any, []map[string]any) {
			delete(c[0], "check_suite")
			return r, c
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			runs, checks := tc.change([]map[string]any{workflowResultFixture(10, 100, "cancelled"), workflowResultFixture(20, 200, "success")}, []map[string]any{workflowCheckFixture(100, "matrix placeholder", "cancelled"), workflowCheckFixture(200, "linux", "success")})
			q := workflowFixtureClient(t, runs, checks, false)
			pr := githubtest.PR(githubtest.Run())
			pr.MergeSHA = ""
			gate, err := q.tests(&pr)
			require.NoError(t, err)
			require.False(t, gate.Passed, gate.Detail)
			if tc.name == "current queued" {
				complete, err := q.reportingComplete(&pr)
				require.NoError(t, err)
				require.False(t, complete)
			}
		})
	}
}

// Bot reviews on fork PRs start Quality Gate Signal runs that wait for maintainer
// approval indefinitely. The signal job only wakes the coordinator, so its runs
// and checks are reporter plumbing, never evidence about the PR. Real workflows
// awaiting approval still block, with approval guidance instead of log guidance.
func TestQualitySignalRunsAreNotCIEvidence(t *testing.T) {
	signal := func(id, suite int64, event, conclusion, created string) map[string]any {
		run := workflowResultFixture(id, suite, conclusion)
		run["path"], run["event"], run["created_at"] = qualitySignalWorkflow, event, created
		return run
	}
	tests := workflowResultFixture(10, 100, "success")
	linux := workflowCheckFixture(100, "linux", "success")
	cases := []struct {
		name    string
		runs    []map[string]any
		checks  []map[string]any
		passed  bool
		detail  string
		problem string
	}{
		{
			name:   "newest signal run awaits approval",
			runs:   []map[string]any{tests, signal(20, 200, "pull_request_review_comment", "action_required", "2026-10-25T19:18:20Z")},
			checks: []map[string]any{linux},
			passed: true,
			detail: "2 current PR CI results",
		},
		{
			name: "older signal run and its check are dropped",
			runs: []map[string]any{
				signal(5, 50, "pull_request", "success", "2026-10-25T17:18:20Z"),
				tests,
				signal(20, 200, "pull_request_review", "action_required", "2026-10-25T19:18:20Z"),
			},
			checks: []map[string]any{workflowCheckFixture(50, "signal", "success"), linux},
			passed: true,
			detail: "2 current PR CI results",
		},
		{
			name:   "signal path at an untrusted event stays evidence",
			runs:   []map[string]any{tests, signal(20, 200, "workflow_dispatch", "failure", "2026-10-25T19:18:20Z")},
			checks: []map[string]any{linux},
			detail: "workflow: " + qualitySignalWorkflow + ": failure",
		},
		{
			name: "newer excluded run does not hide an older untrusted run",
			runs: []map[string]any{
				tests,
				signal(20, 200, "workflow_dispatch", "failure", "2026-10-25T19:18:20Z"),
				signal(30, 300, "pull_request_review", "action_required", "2026-10-25T20:18:20Z"),
			},
			checks: []map[string]any{linux, workflowCheckFixture(200, "dispatched", "failure")},
			detail: "check: github-actions/dispatched: failure; workflow: " + qualitySignalWorkflow + ": failure",
		},
		{
			name:    "real workflow awaits approval",
			runs:    []map[string]any{workflowResultFixture(10, 100, "action_required")},
			checks:  []map[string]any{},
			detail:  "workflow: .github/workflows/test.yml: action_required",
			problem: "1 CI results are waiting for a maintainer to approve their workflow runs; 0 are still pending.",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			q := workflowFixtureClient(t, tc.runs, tc.checks, false)
			pr := githubtest.PR(githubtest.Run())
			pr.MergeSHA = ""
			gate, err := q.tests(&pr)
			require.NoError(t, err)
			require.Equal(t, tc.passed, gate.Passed, gate.Detail)
			require.Contains(t, gate.Detail, tc.detail)
			if tc.passed {
				require.NotContains(t, gate.Detail, "signal")
			}
			if tc.problem != "" {
				require.Equal(t, tc.problem, gate.Problem)
				require.Contains(t, gate.NextAction, "Approve the waiting workflow runs")
				require.NotContains(t, gate.NextAction, "Open the logs")
			}
		})
	}
}

func workflowFixtureClient(t *testing.T, runs, checks []map[string]any, merged bool) *qualityClient {
	t.Helper()
	return qualityTestClient(t, func(path string, response any) error {
		switch {
		case strings.Contains(path, "actions/runs?"):
			return githubtest.Respond(map[string]any{"workflow_runs": runs}, response)
		case strings.Contains(path, "/check-runs?"):
			if merged && strings.Contains(path, githubtest.Base) {
				copies := make([]map[string]any, 0, len(checks))
				for _, check := range checks {
					copy := map[string]any{}
					for key, value := range check {
						copy[key] = value
					}
					copy["head_sha"] = githubtest.Base
					copies = append(copies, copy)
				}
				return githubtest.Respond(map[string]any{"check_runs": copies}, response)
			}
			return githubtest.Respond(map[string]any{"check_runs": checks}, response)
		case strings.Contains(path, "/statuses?"):
			return githubtest.Respond([]any{}, response)
		case strings.HasSuffix(path, "commits/"+githubtest.Base):
			return githubtest.Respond(map[string]any{"parents": []any{map[string]any{"sha": githubtest.SHA}}}, response)
		default:
			t.Fatalf("unexpected request: %s", path)
			return nil
		}
	})
}

func TestQualitySupersededSuitesApplyToSyntheticMergeChecks(t *testing.T) {
	runs := []map[string]any{workflowResultFixture(10, 100, "cancelled"), workflowResultFixture(20, 200, "success")}
	checks := []map[string]any{workflowCheckFixture(100, "matrix placeholder", "cancelled"), workflowCheckFixture(200, "linux", "success")}
	q := workflowFixtureClient(t, runs, checks, true)
	pr := githubtest.PR(githubtest.Run())
	pr.MergeSHA = githubtest.Base
	gate, err := q.tests(&pr)
	require.NoError(t, err)
	require.True(t, gate.Passed, gate.Detail)
	checks = append(checks, workflowCheckFixture(999, "unrelated failure", "failure"))
	q = workflowFixtureClient(t, runs, checks, true)
	gate, err = q.tests(&pr)
	require.NoError(t, err)
	require.False(t, gate.Passed)
	require.Contains(t, gate.Detail, "merge: check: github-actions/unrelated failure")
}

func TestQualityWorkflowOrderingAcrossPagesAndUnrelatedRuns(t *testing.T) {
	older, newer := workflowResultFixture(10, 100, "cancelled"), workflowResultFixture(20, 200, "success")
	wrongHead := workflowResultFixture(1000, 1000, "failure")
	wrongHead["head_sha"] = githubtest.Base
	workflowRun := workflowResultFixture(2000, 2000, "failure")
	workflowRun["event"] = "workflow_run"
	first := []map[string]any{older, workflowRun}
	for len(first) < 100 {
		first = append(first, wrongHead)
	}
	q := qualityTestClient(t, func(path string, response any) error {
		if strings.HasSuffix(path, "page=1") {
			return githubtest.Respond(map[string]any{"workflow_runs": first}, response)
		}
		require.True(t, strings.HasSuffix(path, "page=2"))
		return githubtest.Respond(map[string]any{"workflow_runs": []any{newer}}, response)
	})
	states, superseded := map[string]string{}, map[int64]bool{}
	require.NoError(t, q.workflowStates(githubtest.SHA, states, superseded))
	require.Equal(t, map[string]string{"workflow: .github/workflows/test.yml": "success"}, states)
	require.Equal(t, map[int64]bool{100: true}, superseded)
}

func TestQualityWorkflowAgeSelection(t *testing.T) {
	now := time.Date(2026, 10, 25, 18, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		name        string
		runs        []qualityWorkflowRun
		wantID      int64
		wantAttempt int
	}{
		{"creation beats ID", []qualityWorkflowRun{{Run: githubapi.Run{ID: 1, Created: now}}, {Run: githubapi.Run{ID: 2, Created: now.Add(-time.Hour)}}}, 1, 0},
		{"equal timestamps use ID", []qualityWorkflowRun{{Run: githubapi.Run{ID: 1, Created: now}}, {Run: githubapi.Run{ID: 2, Created: now}}}, 2, 0},
		{"same run uses current attempt", []qualityWorkflowRun{{Run: githubapi.Run{ID: 1, Created: now, Attempt: 1}}, {Run: githubapi.Run{ID: 1, Created: now, Attempt: 2}}}, 1, 2},
		{"missing timestamp switches whole group", []qualityWorkflowRun{{Run: githubapi.Run{ID: 1, Created: now}}, {Run: githubapi.Run{ID: 2}}, {Run: githubapi.Run{ID: 3, Created: now.Add(-time.Hour)}}}, 3, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for start := range tc.runs {
				for _, reverse := range []bool{false, true} {
					var selection qualityWorkflowSelection
					for j := range tc.runs {
						i := (start + j) % len(tc.runs)
						if reverse {
							i = len(tc.runs) - 1 - i
						}
						selection.include(&tc.runs[i])
					}
					selected := selection.current()
					require.Equal(t, tc.wantID, selected.ID)
					require.Equal(t, tc.wantAttempt, selected.Attempt)
				}
			}
		})
	}
}
