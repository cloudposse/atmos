//go:build mage

package qualitygate

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/cli/go-gh/v2/pkg/api"
	"github.com/cloudposse/atmos/magefiles/internal/githubapi"
	"github.com/cloudposse/atmos/magefiles/internal/githubtest"
	"github.com/stretchr/testify/require"
)

// TestQualityEventSelection checks event precedence and includes eligible stacked PRs in fallback selection.
func TestQualityEventSelection(t *testing.T) {
	t.Setenv("GITHUB_EVENT_NAME", "workflow_dispatch")
	cases := []struct {
		name, event, path string
		direct            bool
	}{
		{"issue comment", `{"issue":{"number":42,"pull_request":{}}}`, "", true},
		{"pull request", `{"number":42}`, "", true},
		{"status", `{"sha":"` + githubtest.SHA + `"}`, "commits/" + githubtest.SHA + "/pulls?state=open", false},
		{"check", `{"check_run":{"head_sha":"` + githubtest.SHA + `"}}`, "commits/" + githubtest.SHA + "/pulls?state=open", false},
		{"workflow", `{"workflow_run":{"head_sha":"` + githubtest.SHA + `"}}`, "commits/" + githubtest.SHA + "/pulls?state=open", false},
		{"manual", `{}`, "pulls?state=open", false},
		{"plain issue", `{"issue":{"number":999}}`, "pulls?state=open", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			eventName := map[string]string{"issue comment": "issue_comment", "pull request": "pull_request", "status": "status", "check": "check_run", "workflow": "workflow_run", "manual": "workflow_dispatch", "plain issue": "issue_comment"}[tc.name]
			t.Setenv("GITHUB_EVENT_NAME", eventName)
			eventPath := filepath.Join(t.TempDir(), "event.json")
			require.NoError(t, os.WriteFile(eventPath, []byte(tc.event), 0o600))
			t.Setenv("GITHUB_EVENT_PATH", eventPath)
			calls := 0
			q := qualityTestClient(t, func(path string, response any) error {
				calls++
				require.Contains(t, path, tc.path)
				pr := githubtest.PR(githubtest.Run())
				pr.Updated = time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)
				closed := pr
				closed.State = "closed"
				other := pr
				other.Base.Repo.FullName = "another/repository"
				stacked := pr
				stacked.Number, stacked.Base.Ref = 43, "feature/prerequisite"
				return githubtest.Respond([]githubapi.PR{pr, closed, other, stacked}, response)
			})
			q.trueUp = tc.path == "pulls?state=open"
			prs, err := q.eventPRs(time.Date(2026, 10, 13, 12, 0, 0, 0, time.UTC))
			require.NoError(t, err)
			if tc.direct {
				require.Equal(t, []int{42}, prs)
				require.Zero(t, calls)
			} else {
				require.Equal(t, []int{42, 43}, prs)
				require.Equal(t, 1, calls)
			}
		})
	}
}

func TestQualityEventErrorsAndPagination(t *testing.T) {
	t.Setenv("GITHUB_EVENT_NAME", "workflow_dispatch")
	path := filepath.Join(t.TempDir(), "event.json")
	t.Setenv("GITHUB_EVENT_PATH", path)
	q := qualityTestClient(t, func(_ string, _ any) error { return errors.New("API unavailable") })
	q.trueUp = true
	_, err := q.eventPRs(time.Time{})
	require.Error(t, err)
	for _, data := range []string{`{`, `{"sha":"invalid"}`} {
		require.NoError(t, os.WriteFile(path, []byte(data), 0o600))
		_, err = q.eventPRs(time.Time{})
		require.Error(t, err)
	}
	t.Setenv("GITHUB_EVENT_PATH", "")
	_, err = q.eventPRs(time.Time{})
	require.ErrorContains(t, err, "API unavailable")
	page := 0
	q = qualityTestClient(t, func(path string, response any) error {
		page++
		require.Contains(t, path, fmt.Sprintf("page=%d", page))
		if page == 1 {
			return githubtest.Respond(make([]githubapi.PR, 100), response)
		}
		return githubtest.Respond([]githubapi.PR{githubtest.PR(githubtest.Run())}, response)
	})
	q.trueUp = true
	prs, err := q.eventPRs(time.Time{})
	require.NoError(t, err)
	require.Equal(t, []int{42}, prs)
	require.Equal(t, 2, page)
	q = qualityTestClient(t, func(_ string, response any) error { return githubtest.Respond(make([]githubapi.PR, 100), response) })
	q.trueUp = true
	_, err = q.eventPRs(time.Time{})
	require.ErrorContains(t, err, "pagination limit")
}

// A per-PR report never falls back to every open PR when its event names none.
func TestQualityReportEventWithoutPRSelectsNone(t *testing.T) {
	t.Setenv("GITHUB_EVENT_NAME", "issue_comment")
	path := filepath.Join(t.TempDir(), "event.json")
	require.NoError(t, os.WriteFile(path, []byte(`{"issue":{"number":999}}`), 0o600))
	t.Setenv("GITHUB_EVENT_PATH", path)
	q := qualityTestClient(t, func(path string, _ any) error {
		t.Fatalf("unexpected API path %s", path)
		return nil
	})
	_, err := q.eventPRs(time.Time{})
	require.ErrorContains(t, err, "use quality:reconcile")
}

func TestQualityReconcileRecoversFailedBenchmark(t *testing.T) {
	for _, scenario := range []string{"missing", "running", "recover", "reported", "closed"} {
		t.Run(scenario, func(t *testing.T) {
			run := githubtest.Run()
			run.Event = "pull_request"
			run.Conclusion = "failure"
			pr := githubtest.PR(run)
			count := 149
			pr.ChangedFiles = &count
			if scenario == "closed" {
				pr.State = "closed"
			}
			reads := 0
			getter := qualityPublishGetter(t, "create", &pr, &run, &reads)
			fake := &githubtest.FakeAPI{T: t}
			fake.Get = func(path string, response any) error {
				switch {
				case strings.Contains(path, "actions/workflows/"):
					runs := []githubapi.Run{run}
					if scenario == "missing" {
						runs = nil
					}
					if scenario == "running" {
						runs[0].Status = "in_progress"
					}
					return githubtest.Respond(map[string]any{"workflow_runs": runs}, response)
				case strings.Contains(path, "/commits/") && strings.Contains(path, "/pulls?"):
					return githubtest.Respond([]githubapi.PR{pr}, response)
				case strings.Contains(path, "/comments?") && scenario == "reported":
					c := githubapi.Comment{ID: 17, Body: qualityMarker + qualityHeadMarker(pr.Head.SHA) + "<!-- atmos-benchmark-run:10:2 -->"}
					c.User.Login = "github-actions[bot]"
					c.User.Type = "Bot"
					return githubtest.Respond([]githubapi.Comment{c}, response)
				default:
					return getter(path, response)
				}
			}
			var body string
			fake.Write = func(_ string, path string, r io.Reader) error {
				if strings.Contains(path, "comments") {
					data, err := io.ReadAll(r)
					require.NoError(t, err)
					body = string(data)
				}
				return nil
			}
			q := qualityClient{ctx: context.Background(), api: &qualityGraphAPI{FakeAPI: fake, t: t, scenario: "valid"}, repo: githubtest.Repo}
			require.NoError(t, q.reconcile(42))
			if scenario == "closed" {
				require.Empty(t, body)
				return
			}
			require.Contains(t, body, "Quality Gate")
			if scenario == "recover" {
				require.Contains(t, body, "benchmark execution failure")
			} else if scenario != "reported" {
				require.Contains(t, body, "pending")
			}
		})
	}
}

func TestQualityReconcileErrors(t *testing.T) {
	for _, failedPath := range []string{"pulls/42", "actions/workflows/", "/comments?"} {
		t.Run(failedPath, func(t *testing.T) {
			q := qualityTestClient(t, func(path string, response any) error {
				if strings.Contains(path, failedPath) {
					return errors.New("API unavailable")
				}
				if strings.HasSuffix(path, "pulls/42") {
					return githubtest.Respond(githubtest.PR(githubtest.Run()), response)
				}
				if strings.Contains(path, "actions/runs?") {
					return githubtest.Respond(map[string]any{"workflow_runs": []githubapi.Run{githubtest.Run()}}, response)
				}
				return githubtest.Respond(map[string]any{}, response)
			})
			require.ErrorContains(t, q.reconcile(42), "API unavailable")
		})
	}
	run := githubtest.Run()
	pr := githubtest.PR(run)
	require.False(t, qualityReported(nil, &pr, &run))
	require.False(t, qualityReported(&githubapi.Comment{Body: qualityHeadMarker(githubtest.Base)}, &pr, &run))
	require.True(t, qualityReported(&githubapi.Comment{Body: qualityHeadMarker(pr.Head.SHA) + "<!-- atmos-benchmark-run:11:1 -->"}, &pr, &run))
}

func TestQualityLabelCreationAndFailures(t *testing.T) {
	for _, fail := range []string{"", "issue", "lookup", "create", "delete", "attach"} {
		t.Run(fail, func(t *testing.T) {
			fake := &githubtest.FakeAPI{T: t, Get: func(path string, response any) error {
				if strings.HasSuffix(path, "/issues/42") {
					if fail == "issue" {
						return errors.New("issue failed")
					}
					return githubtest.Respond(map[string]any{"labels": []map[string]string{{"name": qualityReady}}}, response)
				}
				if fail == "lookup" {
					return errors.New("lookup failed")
				}
				return &api.HTTPError{StatusCode: http.StatusNotFound}
			}}
			var calls []string
			fake.Write = func(method, path string, r io.Reader) error {
				action := "attach"
				if strings.HasSuffix(path, "/labels") && !strings.Contains(path, "issues/") {
					action = "create"
				}
				if method == http.MethodDelete {
					action = "delete"
				}
				calls = append(calls, action)
				if action == fail {
					return errors.New(action + " failed")
				}
				if action == "create" {
					data, err := io.ReadAll(r)
					require.NoError(t, err)
					require.Contains(t, string(data), qualityNotReady)
				}
				return nil
			}
			q := qualityClient{ctx: context.Background(), api: fake, repo: githubtest.Repo}
			err := q.labels(42, false)
			if fail != "" {
				require.ErrorContains(t, err, fail+" failed")
			} else {
				require.NoError(t, err)
				require.Equal(t, []string{"delete", "create", "attach"}, calls)
			}
		})
	}
}

func TestQualityEntrypointValidation(t *testing.T) {
	t.Setenv("GITHUB_EVENT_PATH", "")
	t.Setenv("GITHUB_EVENT_NAME", "workflow_run")
	require.ErrorContains(t, (Quality{}).Report("invalid"), "invalid repository")
	t.Setenv("GITHUB_EVENT_NAME", "schedule")
	require.ErrorContains(t, (Quality{}).Reconcile("invalid"), "invalid repository")
	require.ErrorContains(t, (Bench{}).Report("invalid", "10"), "invalid repository")
	for _, id := range []string{"bad", "0", "-1"} {
		require.ErrorContains(t, (Bench{}).Report(githubtest.Repo, id), "invalid run ID")
	}
}
