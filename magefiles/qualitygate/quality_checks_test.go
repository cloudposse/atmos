//go:build mage

package qualitygate

import (
	"fmt"
	"testing"

	"github.com/cloudposse/atmos/magefiles/internal/githubapi"
	"github.com/cloudposse/atmos/magefiles/internal/githubtest"
	"github.com/stretchr/testify/require"
)

func TestQualityChecksKeepNewestAcrossSuites(t *testing.T) {
	for _, collector := range []string{"CI", "coverage"} {
		for _, paginated := range []bool{false, true} {
			for _, newestState := range []string{"success", "failure", "queued", "in_progress"} {
				t.Run(fmt.Sprintf("%s/paginated=%t/newest=%s", collector, paginated, newestState), func(t *testing.T) {
					newest := qualityCheck{Name: "PR Semver Labels", HeadSHA: githubtest.SHA, Status: "completed", Conclusion: newestState}
					newest.App.Slug = "github-actions"
					if collector == "coverage" {
						newest.Name, newest.App.Slug = "codecov/patch", "codecov"
					}
					if newestState == "queued" || newestState == "in_progress" {
						newest.Status, newest.Conclusion = newestState, ""
					}
					older := newest
					older.Status, older.Conclusion = "completed", "success"
					if newestState == "success" {
						older.Conclusion = "failure"
					}
					wrongHead := older
					wrongHead.HeadSHA = githubtest.Base
					otherApp := older
					otherApp.App.Slug = "other-app"
					pages := [][]qualityCheck{{wrongHead, otherApp, newest}}
					if paginated {
						// A duplicate on page two must not overwrite page one's result.
						for len(pages[0]) < githubapi.PageSize {
							pages[0] = append(pages[0], wrongHead)
						}
						pages = append(pages, []qualityCheck{older})
					} else {
						pages[0] = append(pages[0], older)
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
						require.NoError(t, q.allCheckStates(githubtest.SHA, states))
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
