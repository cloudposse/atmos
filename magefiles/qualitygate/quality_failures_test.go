//go:build mage

package qualitygate

import (
	"context"
	"errors"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cloudposse/atmos/magefiles/internal/githubapi"
	"github.com/cloudposse/atmos/magefiles/internal/githubtest"
	"github.com/stretchr/testify/require"
)

func TestQualityInspectionFailuresBlockReadiness(t *testing.T) {
	for _, failedPath := range []string{"actions/runs?", githubtest.SHA + "/check-runs", githubtest.SHA + "/statuses", githubtest.Base + "/check-runs", githubtest.Base + "/statuses", "/reviews?"} {
		t.Run(failedPath, func(t *testing.T) {
			run := githubtest.Run()
			pr := githubtest.PR(run)
			pr.MergeSHA = githubtest.Base
			reads := 0
			get := qualityPublishGetter(t, "create", &pr, &run, &reads)
			q := qualityTestClient(t, func(path string, response any) error {
				if strings.Contains(path, failedPath) {
					return errors.New("API unavailable")
				}
				return get(path, response)
			})
			if failedPath == "/reviews?" {
				gate, err := q.review(&pr)
				require.Error(t, err)
				require.False(t, gate.Passed)
				return
			}
			gate, err := q.tests(&pr)
			require.ErrorContains(t, err, "API unavailable")
			require.False(t, gate.Passed)
			if strings.Contains(failedPath, githubtest.SHA) {
				gate, err = q.coverage(&pr)
				require.Error(t, err)
				require.False(t, gate.Passed)
			}
		})
	}
}

func TestQualityPaginationLimitsFailClosed(t *testing.T) {
	for _, kind := range []string{"workflows", "checks", "statuses", "coverage checks", "coverage statuses", "reviews", "comments"} {
		t.Run(kind, func(t *testing.T) {
			q := qualityTestClient(t, func(_ string, response any) error {
				switch kind {
				case "workflows":
					return githubtest.Respond(map[string]any{"workflow_runs": make([]githubapi.Run, 100)}, response)
				case "checks", "coverage checks":
					return githubtest.Respond(map[string]any{"check_runs": make([]qualityCheck, 100)}, response)
				default:
					return githubtest.Respond(make([]map[string]any, 100), response)
				}
			})
			states := map[string]string{}
			pr := githubtest.PR(githubtest.Run())
			var err error
			switch kind {
			case "workflows":
				err = q.workflowStates(pr.Head.SHA, states, nil)
			case "checks":
				err = q.allCheckStates(pr.Head.SHA, states, nil)
			case "statuses":
				err = q.allStatusStates(pr.Head.SHA, states)
			case "coverage checks":
				err = q.coverageChecks(pr.Head.SHA, states)
			case "coverage statuses":
				err = q.coverageStatuses(pr.Head.SHA, states)
			case "reviews":
				_, err = q.review(&pr)
			case "comments":
				_, err = q.existing(pr.Number)
			}
			require.ErrorContains(t, err, "pagination limit")
		})
	}
}

func TestQualityPublishFailuresAndIdempotency(t *testing.T) {
	for _, scenario := range []string{"lookup", "labels", "summary", "unchanged"} {
		t.Run(scenario, func(t *testing.T) {
			fake := &githubtest.FakeAPI{T: t, Get: func(path string, response any) error {
				if scenario == "labels" || scenario == "lookup" {
					return errors.New("lookup failed")
				}
				if strings.Contains(path, "/comments?") {
					return githubtest.Respond([]map[string]any{{"body": qualityMarker + "same", "user": map[string]string{"login": "github-actions[bot]", "type": "Bot"}}}, response)
				}
				return githubtest.Respond(map[string]any{}, response)
			}}
			fake.Write = func(_ string, path string, _ io.Reader) error { require.NotContains(t, path, "comments"); return nil }
			q := qualityClient{ctx: context.Background(), api: fake, repo: githubtest.Repo}
			t.Setenv("GITHUB_STEP_SUMMARY", "")
			if scenario == "summary" {
				t.Setenv("GITHUB_STEP_SUMMARY", t.TempDir())
			}
			if scenario == "lookup" {
				require.Error(t, q.refresh(42, "", nil))
				return
			}
			published, err := q.publish(42, &githubapi.Comment{Body: qualityMarker + "same"}, qualityMarker+"same", true)
			if scenario == "unchanged" {
				require.NoError(t, err)
				require.True(t, published)
			} else {
				require.Error(t, err)
			}
		})
	}
	q := qualityTestClient(t, func(_ string, _ any) error { return errors.New("API unavailable") })
	require.Error(t, q.write(http.MethodPost, "unused", make(chan int)))
	pr := githubtest.PR(githubtest.Run())
	_, err := q.stillCurrent(&pr, nil)
	require.Error(t, err)
	gate, err := qualityFileCount(&pr)
	require.NoError(t, err)
	require.False(t, gate.Passed)
	require.Contains(t, qualityShortDetail(strings.Repeat("x", 1600)), "see CI checks")
	t.Setenv("GITHUB_STEP_SUMMARY", filepath.Join(t.TempDir(), "absent", "summary"))
	require.Error(t, qualitySummary("report"))
}

func TestQualityRefreshIgnoresClosedAndWrongHead(t *testing.T) {
	for _, closed := range []bool{true, false} {
		pr := githubtest.PR(githubtest.Run())
		run := githubtest.Run()
		run.HeadSHA = githubtest.Base
		if closed {
			pr.State = "closed"
		}
		q := qualityTestClient(t, func(_ string, response any) error { return githubtest.Respond(pr, response) })
		require.NoError(t, q.refresh(42, "", &run))
	}
}
