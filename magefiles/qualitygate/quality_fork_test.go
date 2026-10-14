//go:build mage

package qualitygate

import (
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/cloudposse/atmos/magefiles/internal/githubapi"
	"github.com/cloudposse/atmos/magefiles/internal/githubtest"
	"github.com/stretchr/testify/require"
)

const (
	forkSHA    = "497b3a2d43371e3a98912e38a162f39b6c5d2d96"
	forkOwner  = "jorrite"
	forkBranch = "hp/atmos/t-0004-fix-include-docs-claim-oci-source-suppor"
	forkNumber = 3381
)

func forkPR(mutate func(*githubapi.PR)) githubapi.PR {
	pr := githubtest.PR(githubtest.Run())
	pr.Number, pr.State = forkNumber, "open"
	pr.Head.SHA = forkSHA
	pr.Head.Repo.FullName = forkOwner + "/atmos"
	if mutate != nil {
		mutate(&pr)
	}
	return pr
}

func forkWorkflowRunEvent() string {
	return `{"workflow_run":{"head_sha":"` + forkSHA + `","head_branch":"` + forkBranch + `","head_repository":{"full_name":"` + forkOwner + `/atmos"},"pull_requests":[]}}`
}

// TestQualityForkPREventResolution covers PRs whose head commit exists only in a
// fork: the commit association endpoint returns nothing for them.
func TestQualityForkPREventResolution(t *testing.T) {
	moved := func(pr *githubapi.PR) { pr.Head.SHA = githubtest.Base }
	closed := func(pr *githubapi.PR) { pr.State = "closed" }
	release := func(pr *githubapi.PR) { pr.Base.Ref = "release" }
	cases := []struct {
		name      string
		eventName string
		event     string
		commits   []githubapi.PR // commits/{sha}/pulls
		head      []githubapi.PR // pulls?head=owner:branch
		search    []int          // search/issues items
		pulls     map[int]githubapi.PR
		searchErr bool
		want      []int
		wantErr   string
		wantKinds []string
		wantHead  string
	}{
		{
			name:      "workflow_run resolves a fork PR through its head branch",
			eventName: "workflow_run",
			event:     forkWorkflowRunEvent(),
			head:      []githubapi.PR{forkPR(nil)},
			want:      []int{forkNumber},
			wantKinds: []string{"commits", "head"},
			wantHead:  forkOwner + ":" + forkBranch,
		},
		{
			name:      "check_run has no fork owner and resolves through SHA search",
			eventName: "check_run",
			event:     `{"check_run":{"head_sha":"` + forkSHA + `","check_suite":{"head_branch":"` + forkBranch + `"}}}`,
			search:    []int{forkNumber},
			pulls:     map[int]githubapi.PR{forkNumber: forkPR(nil)},
			want:      []int{forkNumber},
			wantKinds: []string{"commits", "search", "pull"},
		},
		{
			name:      "status resolves through SHA search",
			eventName: "status",
			event:     `{"sha":"` + forkSHA + `","branches":[]}`,
			search:    []int{forkNumber},
			pulls:     map[int]githubapi.PR{forkNumber: forkPR(nil)},
			want:      []int{forkNumber},
			wantKinds: []string{"commits", "search", "pull"},
		},
		{
			name:      "fallback is not used when the commit endpoint lists the PR",
			eventName: "workflow_run",
			event:     forkWorkflowRunEvent(),
			commits:   []githubapi.PR{forkPR(nil)},
			head:      []githubapi.PR{forkPR(nil)},
			search:    []int{forkNumber},
			pulls:     map[int]githubapi.PR{forkNumber: forkPR(nil)},
			want:      []int{forkNumber},
			wantKinds: []string{"commits"},
		},
		{
			name:      "fallback runs when the commit endpoint lists only a closed PR",
			eventName: "workflow_run",
			event:     forkWorkflowRunEvent(),
			commits:   []githubapi.PR{forkPR(closed)},
			head:      []githubapi.PR{forkPR(nil)},
			want:      []int{forkNumber},
			wantKinds: []string{"commits", "head"},
			wantHead:  forkOwner + ":" + forkBranch,
		},
		{
			name:      "a PR whose head moved does not match the stale SHA",
			eventName: "workflow_run",
			event:     forkWorkflowRunEvent(),
			head:      []githubapi.PR{forkPR(moved)},
			search:    []int{forkNumber},
			pulls:     map[int]githubapi.PR{forkNumber: forkPR(moved)},
			wantKinds: []string{"commits", "head", "search", "pull"},
			wantHead:  forkOwner + ":" + forkBranch,
		},
		{
			name:      "search hit whose head moved is rejected",
			eventName: "check_run",
			event:     `{"check_run":{"head_sha":"` + forkSHA + `"}}`,
			search:    []int{forkNumber},
			pulls:     map[int]githubapi.PR{forkNumber: forkPR(moved)},
			wantKinds: []string{"commits", "search", "pull"},
		},
		{
			name:      "closed and non-main PRs are not reconciled",
			eventName: "workflow_run",
			event:     forkWorkflowRunEvent(),
			head:      []githubapi.PR{forkPR(closed), forkPR(release)},
			search:    []int{forkNumber, 3382},
			pulls: map[int]githubapi.PR{
				forkNumber: forkPR(closed),
				3382:       forkPR(func(pr *githubapi.PR) { pr.Number = 3382 }),
			},
			want:      []int{3382},
			wantKinds: []string{"commits", "head", "search", "pull", "pull"},
			wantHead:  forkOwner + ":" + forkBranch,
		},
		{
			name:      "search failure is an error, not an empty result",
			eventName: "check_run",
			event:     `{"check_run":{"head_sha":"` + forkSHA + `"}}`,
			searchErr: true,
			wantErr:   "search unavailable",
			wantKinds: []string{"commits", "search"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("GITHUB_EVENT_NAME", tc.eventName)
			eventPath := filepath.Join(t.TempDir(), "event.json")
			require.NoError(t, os.WriteFile(eventPath, []byte(tc.event), 0o600))
			t.Setenv("GITHUB_EVENT_PATH", eventPath)
			var kinds []string
			var head string
			q := qualityTestClient(t, func(path string, response any) error {
				switch {
				case strings.Contains(path, "/commits/"+forkSHA+"/pulls"):
					kinds = append(kinds, "commits")
					return githubtest.Respond(tc.commits, response)
				case strings.Contains(path, "pulls?") && strings.Contains(path, "head="):
					kinds = append(kinds, "head")
					parsed, err := url.Parse(path)
					require.NoError(t, err)
					head = parsed.Query().Get("head")
					return githubtest.Respond(tc.head, response)
				case strings.HasPrefix(path, "search/issues?"):
					kinds = append(kinds, "search")
					if tc.searchErr {
						return errors.New("search unavailable")
					}
					parsed, err := url.Parse(path)
					require.NoError(t, err)
					require.Equal(t, "repo:"+githubtest.Repo+" type:pr state:open sha:"+forkSHA, parsed.Query().Get("q"))
					items := make([]map[string]int, 0, len(tc.search))
					for _, number := range tc.search {
						items = append(items, map[string]int{"number": number})
					}
					return githubtest.Respond(map[string]any{"items": items}, response)
				case strings.Contains(path, "/pulls/"):
					kinds = append(kinds, "pull")
					for number, pr := range tc.pulls {
						if strings.HasSuffix(path, "/pulls/"+strconv.Itoa(number)) {
							return githubtest.Respond(pr, response)
						}
					}
				}
				t.Fatalf("unexpected API path %s", path)
				return nil
			})
			got, err := q.eventPRs(time.Date(2026, 10, 14, 12, 0, 0, 0, time.UTC))
			if tc.wantErr != "" {
				require.ErrorContains(t, err, tc.wantErr)
			} else {
				require.NoError(t, err)
				require.Equal(t, tc.want, got)
			}
			require.Equal(t, tc.wantKinds, kinds)
			require.Equal(t, tc.wantHead, head)
		})
	}
}
