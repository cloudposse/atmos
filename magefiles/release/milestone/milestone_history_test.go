//go:build mage

package milestone

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/magefiles/internal/githubtest"
)

func TestMilestoneGateRejectsPatchAfterMinorPRMerged(t *testing.T) {
	patch := Milestone{1, "v1.233.1", "open"}
	minor := Milestone{2, "v1.234.0", "open"}
	for _, label := range []string{"minor", "patch"} {
		t.Run(label, func(t *testing.T) {
			pending := policyPR("patch", &patch)
			pending.Head.SHA = githubtest.SHA
			exempt := policyPR("no-release", nil)
			exempt.Number = 43
			exempt.Head.SHA = githubtest.Base
			merged := policyPR(label, &minor)
			merged.Number = 44
			merged.Base.Ref = "main"
			states := map[string]map[string]string{}
			c := milestoneHTTP(t, func(w http.ResponseWriter, r *http.Request) {
				if emptyCurrentStatus(t, w, r) {
					return
				}
				path := strings.TrimPrefix(strings.TrimPrefix(r.URL.Path, "/repos/cloudposse/atmos"), "/")
				switch path {
				case "":
					githubtest.JSON(t, w, map[string]string{"default_branch": "main"})
				case "commits/main":
					githubtest.JSON(t, w, map[string]string{"sha": githubtest.SHA})
				case "milestones":
					githubtest.JSON(t, w, []Milestone{patch, minor})
				case "releases":
					githubtest.JSON(t, w, []any{stableRelease("v1.233.0")})
				case "compare/v1.233.0..." + githubtest.SHA:
					githubtest.JSON(t, w, map[string]any{"status": "ahead", "total_commits": 1, "commits": []map[string]string{{"sha": githubtest.SHA}}})
				case "commits/" + githubtest.SHA + "/pulls":
					githubtest.JSON(t, w, []map[string]int{{"number": 44}})
				case "pulls/44":
					githubtest.JSON(t, w, struct {
						ReleasePR
						Merged bool `json:"merged"`
					}{merged, true})
				default:
					require.True(t, strings.HasPrefix(path, "statuses/"), path)
					require.Equal(t, http.MethodPost, r.Method)
					var body map[string]string
					require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
					states[strings.TrimPrefix(path, "statuses/")] = body
					githubtest.JSON(t, w, map[string]any{})
				}
			})
			require.NoError(t, c.publishPolicyVerdicts([]ReleasePR{pending, exempt}, nil, nil, "main"))
			require.Equal(t, "failure", states[pending.Head.SHA]["state"], "the lowest milestone must still be viable from the unreleased commits")
			require.Contains(t, states[pending.Head.SHA]["description"], "v1.233.1")
			require.Contains(t, states[pending.Head.SHA]["description"], "#44")
			require.Contains(t, states[pending.Head.SHA]["description"], "v1.234.0")
			require.Equal(t, "success", states[exempt.Head.SHA]["state"])
		})
	}
}

func TestReleaseCompareCommits(t *testing.T) {
	for _, mode := range []string{"two pages", "http error", "diverged", "invalid SHA", "incomplete", "page limit"} {
		t.Run(mode, func(t *testing.T) {
			calls := 0
			c := milestoneHTTP(t, func(w http.ResponseWriter, r *http.Request) {
				calls++
				require.Equal(t, "/repos/cloudposse/atmos/"+"compare/v1.233.0..."+githubtest.SHA, r.URL.Path)
				require.Equal(t, strconv.Itoa(calls), r.URL.Query().Get("page"))
				require.Equal(t, "100", r.URL.Query().Get("per_page"))
				if mode == "http error" {
					http.Error(w, "unavailable", http.StatusServiceUnavailable)
					return
				}
				status := "ahead"
				if mode == "diverged" {
					status = "diverged"
				}
				count := 100
				if mode != "page limit" && (mode != "two pages" || calls == 2) {
					count = 1
				}
				commits := make([]map[string]string, count)
				for i := range commits {
					commits[i] = map[string]string{"sha": fmt.Sprintf("%040x", (calls-1)*100+i+1)}
				}
				if mode == "invalid SHA" {
					commits[0]["sha"] = "short"
				}
				total := 101
				if mode == "incomplete" {
					total = 2
				}
				githubtest.JSON(t, w, map[string]any{"status": status, "commits": commits, "total_commits": total})
			})
			got, err := c.CompareCommits("v1.233.0", githubtest.SHA)
			if mode == "two pages" {
				require.NoError(t, err)
				require.Len(t, got, 101)
				require.Equal(t, fmt.Sprintf("%040x", 1), got[0])
				require.Equal(t, fmt.Sprintf("%040x", 101), got[100])
			} else {
				require.Error(t, err)
				require.Nil(t, got)
				if mode == "page limit" {
					require.Equal(t, 100, calls)
					require.ErrorContains(t, err, "pagination limit")
				}
			}
		})
	}
}

func TestCheckReleaseHistory(t *testing.T) {
	patch := Milestone{1, "v1.233.1", "open"}
	for _, scenario := range []string{"compatible", "exempt", "missing milestone", "no open milestone", "missing branch", "invalid head", "repo API", "head API", "compare API", "association API", "pull API", "direct commit", "unmerged", "other branch", "landed stack", "unmerged stack", "unlanded stack", "landed stack missing milestone"} {
		t.Run(scenario, func(t *testing.T) {
			pr := PullRequest{ReleasePR: policyPR("patch", &patch), Merged: true}
			pr.Base.Ref = "main"
			milestones := []Milestone{patch}
			routes := map[string]any{
				"":                                      map[string]string{"default_branch": "main"},
				"commits/main":                          map[string]string{"sha": githubtest.SHA},
				"compare/v1.233.0..." + githubtest.SHA:  map[string]any{"status": "ahead", "total_commits": 2, "commits": []map[string]string{{"sha": githubtest.SHA}, {"sha": githubtest.Base}}},
				"commits/" + githubtest.SHA + "/pulls":  []map[string]int{{"number": 42}},
				"commits/" + githubtest.Base + "/pulls": []map[string]int{{"number": 42}},
			}
			failPath := "unreachable"
			switch scenario {
			case "exempt":
				pr.ReleasePR = policyPR("no-release", nil)
				pr.Base.Ref = "main"
			case "missing milestone":
				pr.Milestone = nil
			case "no open milestone":
				milestones = nil
			case "missing branch":
				routes[""] = map[string]string{}
			case "invalid head":
				routes["commits/main"] = map[string]string{"sha": "short"}
			case "repo API":
				failPath = ""
			case "head API":
				failPath = "commits/main"
			case "compare API":
				failPath = "compare/v1.233.0..." + githubtest.SHA
			case "association API":
				failPath = "commits/" + githubtest.SHA + "/pulls"
			case "pull API":
				failPath = "pulls/42"
			case "direct commit":
				routes["commits/"+githubtest.SHA+"/pulls"] = []any{}
			case "unmerged":
				pr.Merged = false
			case "other branch":
				pr.Base.Ref = "other"
			case "landed stack", "unmerged stack", "unlanded stack", "landed stack missing milestone":
				pr.Base.Ref = "stack/parent"
				pr.MergeCommit = githubtest.Base
				if scenario == "unmerged stack" {
					pr.Merged = false
				}
				if scenario == "unlanded stack" {
					pr.MergeCommit = strings.Repeat("c", 40)
				}
				if scenario == "landed stack missing milestone" {
					pr.Milestone = nil
				}
			}
			routes["pulls/42"] = pr
			pullReads := 0
			c := milestoneHTTP(t, func(w http.ResponseWriter, r *http.Request) {
				require.Equal(t, http.MethodGet, r.Method)
				path := strings.TrimPrefix(strings.TrimPrefix(r.URL.Path, "/repos/cloudposse/atmos"), "/")
				if path == failPath {
					http.Error(w, "unavailable", http.StatusServiceUnavailable)
					return
				}
				if path == "pulls/42" {
					pullReads++
				}
				response, ok := routes[path]
				require.True(t, ok, path)
				githubtest.JSON(t, w, response)
			})
			err := c.CheckReleaseHistory(milestones, "v1.233.0")
			if scenario == "compatible" || scenario == "exempt" || scenario == "landed stack" {
				require.NoError(t, err)
				require.Equal(t, 1, pullReads, "PRs spanning multiple commits must be deduplicated")
				return
			}
			require.Error(t, err)
			if strings.HasSuffix(scenario, "API") {
				require.ErrorContains(t, err, "503")
				return
			}
			require.ErrorIs(t, err, ErrMilestonePolicy)
			if scenario == "landed stack missing milestone" {
				require.ErrorContains(t, err, "requires a release milestone")
			}
		})
	}
}
