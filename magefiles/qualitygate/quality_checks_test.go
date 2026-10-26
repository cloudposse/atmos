//go:build mage

package qualitygate

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/magefiles/internal/githubapi"
	"github.com/cloudposse/atmos/magefiles/internal/githubtest"
)

// TestQualityChecksKeepNewestAcrossSuites rejects response-order assumptions in both collectors.
func TestQualityChecksKeepNewestAcrossSuites(t *testing.T) {
	for _, collector := range []string{"CI", "coverage"} {
		for _, paginated := range []bool{false, true} {
			for _, reversed := range []bool{false, true} {
				for _, newestState := range []string{"success", "failure", "queued", "in_progress"} {
					t.Run(fmt.Sprintf("%s/paginated=%t/reversed=%t/newest=%s", collector, paginated, reversed, newestState), func(t *testing.T) {
						newest := qualityCheck{ID: 200, StartedAt: time.Date(2026, 10, 22, 13, 54, 0, 0, time.UTC), Name: "PR Semver Labels", HeadSHA: githubtest.SHA, Status: "completed", Conclusion: newestState}
						newest.App.Slug = "github-actions"
						if collector == "coverage" {
							newest.Name, newest.App.Slug = "codecov/patch", "codecov"
						}
						if newestState == "queued" || newestState == "in_progress" {
							newest.Status, newest.Conclusion = newestState, ""
						}
						older := newest
						older.ID, older.StartedAt = 100, newest.StartedAt.Add(-time.Hour)
						older.Status, older.Conclusion = "completed", "success"
						if newestState == "success" {
							older.Conclusion = "failure"
						}
						wrongHead := older
						wrongHead.HeadSHA = githubtest.Base
						otherApp := older
						otherApp.App.Slug = "other-app"
						first, last := newest, older
						if reversed {
							first, last = older, newest
						}
						pages := [][]qualityCheck{{wrongHead, otherApp, first}}
						if paginated {
							// Select the newest check even when it arrives on page two.
							for len(pages[0]) < githubapi.PageSize {
								pages[0] = append(pages[0], wrongHead)
							}
							pages = append(pages, []qualityCheck{last})
						} else {
							pages[0] = append(pages[0], last)
						}
						calls := 0
						q := qualityTestClient(t, func(path string, response any) error {
							require.Less(t, calls, len(pages))
							require.Equal(t, fmt.Sprintf("repos/%s/commits/%s/check-runs?filter=latest&per_page=100&page=%d", githubtest.Repo, githubtest.SHA, calls+1), path)
							page := pages[calls]
							calls++
							return githubtest.Respond(map[string]any{"check_runs": page}, response)
						})
						states := map[string]string{}
						if collector == "coverage" {
							require.NoError(t, q.coverageChecks(githubtest.SHA, states))
							require.Equal(t, map[string]string{newest.Name: newestState}, states)
							require.Equal(t, newestState == "success", qualityCoverageGate(states).Passed)
						} else {
							require.NoError(t, q.allCheckStates(githubtest.SHA, states, nil))
							require.Equal(t, map[string]string{
								"check: github-actions/" + newest.Name: newestState,
								"check: other-app/" + newest.Name:      older.Conclusion,
							}, states)
							delete(states, "check: other-app/"+newest.Name)
							require.Equal(t, newestState == "success", qualityCIGate(states).Passed)
						}
						require.Equal(t, len(pages), calls)
					})
				}
			}
		}
	}
}

// TestQualityCheckAgeFallbacks covers queued checks and timestamp/ID ties.
func TestQualityCheckAgeFallbacks(t *testing.T) {
	now := time.Date(2026, 10, 22, 13, 54, 0, 0, time.UTC)
	for _, tc := range []struct {
		name  string
		newer qualityCheck
		older qualityCheck
	}{
		{name: "timestamps override IDs", newer: qualityCheck{ID: 1, StartedAt: now}, older: qualityCheck{ID: 2, StartedAt: now.Add(-time.Hour)}},
		{name: "equal timestamps", newer: qualityCheck{ID: 2, StartedAt: now}, older: qualityCheck{ID: 1, StartedAt: now}},
		{name: "queued rerun has no start", newer: qualityCheck{ID: 2}, older: qualityCheck{ID: 1, StartedAt: now}},
		{name: "previous has no start", newer: qualityCheck{ID: 2, StartedAt: now}, older: qualityCheck{ID: 1}},
		{name: "both missing starts", newer: qualityCheck{ID: 2}, older: qualityCheck{ID: 1}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, checks := range [][]*qualityCheck{{&tc.newer, &tc.older}, {&tc.older, &tc.newer}} {
				var selection qualityCheckSelection
				var newest *qualityCheck
				for _, check := range checks {
					newest = selection.include(check)
				}
				require.Equal(t, &tc.newer, newest)
			}
		})
	}
}

// TestQualityMissingStartOrdersTheWholeGroup avoids inconsistent pairwise ordering.
func TestQualityMissingStartOrdersTheWholeGroup(t *testing.T) {
	now := time.Date(2026, 10, 22, 13, 54, 0, 0, time.UTC)
	checks := []qualityCheck{
		{ID: 1, StartedAt: now},
		{ID: 2},
		{ID: 3, StartedAt: now.Add(-time.Hour)},
	}
	for _, order := range [][]int{{0, 1, 2}, {0, 2, 1}, {1, 0, 2}, {1, 2, 0}, {2, 0, 1}, {2, 1, 0}} {
		var selection qualityCheckSelection
		var newest *qualityCheck
		for _, i := range order {
			newest = selection.include(&checks[i])
		}
		require.Equal(t, int64(3), newest.ID)
	}
}
