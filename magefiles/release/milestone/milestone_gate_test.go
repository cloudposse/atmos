//go:build mage

package milestone

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/cloudposse/atmos/magefiles/internal/githubtest"
	"github.com/stretchr/testify/require"
)

const (
	queueSHA      = "cccccccccccccccccccccccccccccccccccccccc"
	laterQueueSHA = "dddddddddddddddddddddddddddddddddddddddd"
)

func queueItem(head, base string, pr int) queueEntry {
	e := queueEntry{Head: &queueCommit{head}, Base: &queueCommit{base}}
	e.PR.Number = pr
	return e
}

func queueResponse(entries []queueEntry, next bool, cursor string) map[string]any {
	return map[string]any{"data": map[string]any{"repository": map[string]any{"mergeQueue": map[string]any{"entries": map[string]any{"nodes": entries, "pageInfo": map[string]any{"hasNextPage": next, "endCursor": cursor}}}}}}
}

func TestQueueVerdict(t *testing.T) {
	allowed := map[int]error{1: nil, 2: nil, 3: fmt.Errorf("%w: blocked PR", ErrMilestonePolicy)}
	for _, tc := range []struct {
		name, head string
		entries    []queueEntry
		message    string
	}{
		{"allowed group", queueSHA, []queueEntry{queueItem(queueSHA, githubtest.Base, 1), queueItem(queueSHA, githubtest.Base, 2)}, ""},
		{"later blocked does not taint first", queueSHA, []queueEntry{queueItem(queueSHA, githubtest.Base, 1), queueItem(laterQueueSHA, queueSHA, 3)}, ""},
		{"preceding blocked group", laterQueueSHA, []queueEntry{queueItem(queueSHA, githubtest.Base, 3), queueItem(laterQueueSHA, queueSHA, 2)}, "blocked PR"},
		{"same group blocked", queueSHA, []queueEntry{queueItem(queueSHA, githubtest.Base, 1), queueItem(queueSHA, githubtest.Base, 3)}, "blocked PR"},
		{"unknown PR", queueSHA, []queueEntry{queueItem(queueSHA, githubtest.Base, 99)}, "missing from live open PRs"},
		{"invalid base", queueSHA, []queueEntry{queueItem(queueSHA, "short", 1)}, "queue base missing"},
		{"cycle", queueSHA, []queueEntry{queueItem(queueSHA, laterQueueSHA, 1), queueItem(laterQueueSHA, queueSHA, 2)}, "cyclic"},
		{"self base", queueSHA, []queueEntry{queueItem(queueSHA, queueSHA, 1)}, ""},
		{"nil head ignored", queueSHA, []queueEntry{{}, queueItem(queueSHA, githubtest.Base, 1)}, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := queueVerdict(tc.head, tc.entries, allowed, "main")
			if tc.message == "" {
				require.NoError(t, err)
			} else {
				require.ErrorIs(t, err, ErrMilestonePolicy)
				require.ErrorContains(t, err, tc.message)
			}
		})
	}
}

func TestMilestoneGateMergedQueueEntry(t *testing.T) {
	for _, mode := range []string{"merged", "unmerged", "different branch", "different commit", "missing commit"} {
		t.Run(mode, func(t *testing.T) {
			entry := queueItem(queueSHA, githubtest.Base, 1)
			entry.PR.Merged = mode != "unmerged"
			entry.PR.BaseRefName = "main"
			entry.PR.MergeCommit = &queueCommit{OID: queueSHA}
			if mode == "different branch" {
				entry.PR.BaseRefName = "other"
			}
			if mode == "different commit" {
				entry.PR.MergeCommit.OID = laterQueueSHA
			}
			if mode == "missing commit" {
				entry.PR.MergeCommit = nil
			}
			var state string
			c := milestoneHTTP(t, func(w http.ResponseWriter, r *http.Request) {
				if emptyCurrentStatus(t, w, r) {
					return
				}
				switch r.URL.Path {
				case "/repos/cloudposse/atmos":
					githubtest.JSON(t, w, map[string]string{"default_branch": "main"})
				case "/repos/cloudposse/atmos/pulls":
					githubtest.JSON(t, w, []ReleasePR{})
				case "/graphql":
					githubtest.JSON(t, w, queueResponse([]queueEntry{entry}, false, ""))
				case "/repos/cloudposse/atmos/milestones":
					githubtest.JSON(t, w, []Milestone{})
				case "/repos/cloudposse/atmos/releases":
					githubtest.JSON(t, w, []any{stableRelease("v1.233.0")})
				case "/repos/cloudposse/atmos/statuses/" + queueSHA:
					var body commitStatus
					require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
					state = body.State
					githubtest.JSON(t, w, map[string]any{})
				default:
					t.Errorf("unexpected request: %s", r.URL.Path)
					http.NotFound(w, r)
				}
			})
			require.NoError(t, c.reconcileMilestoneStatuses(queueSHA))
			if mode == "merged" {
				require.Equal(t, "success", state, "a merged queue entry must not overwrite its landed commit with a missing-open-PR failure")
			} else {
				require.Equal(t, "failure", state, "incomplete or unrelated merge evidence must not admit a missing PR")
			}
		})
	}
}

func TestQueuePagination(t *testing.T) {
	for _, mode := range []string{"pages", "disabled", "graphql error", "missing repository", "http error", "empty cursor", "repeated cursor", "limit"} {
		t.Run(mode, func(t *testing.T) {
			calls := 0
			c := milestoneHTTP(t, func(w http.ResponseWriter, r *http.Request) {
				if emptyCurrentStatus(t, w, r) {
					return
				}
				require.Equal(t, "/graphql", r.URL.Path)
				require.Equal(t, http.MethodPost, r.Method)
				calls++
				var body struct {
					Query     string         `json:"query"`
					Variables map[string]any `json:"variables"`
				}
				require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
				require.Equal(t, "cloudposse", body.Variables["owner"])
				require.Equal(t, "atmos", body.Variables["name"])
				require.Equal(t, "release/main", body.Variables["branch"])
				if calls == 1 {
					require.Nil(t, body.Variables["cursor"])
				} else {
					require.Equal(t, fmt.Sprintf("cursor-%d", calls-1), body.Variables["cursor"])
				}
				switch mode {
				case "disabled":
					githubtest.JSON(t, w, map[string]any{"data": map[string]any{"repository": map[string]any{"mergeQueue": nil}}})
				case "graphql error":
					githubtest.JSON(t, w, map[string]any{"errors": []any{map[string]string{"message": "forbidden"}}})
				case "missing repository":
					githubtest.JSON(t, w, map[string]any{"data": map[string]any{"repository": nil}})
				case "http error":
					http.Error(w, "unavailable", http.StatusServiceUnavailable)
				default:
					cursor := fmt.Sprintf("cursor-%d", calls)
					if mode == "empty cursor" {
						cursor = ""
					}
					if mode == "repeated cursor" {
						cursor = "cursor-1"
					}
					githubtest.JSON(t, w, queueResponse([]queueEntry{queueItem(queueSHA, githubtest.Base, calls)}, mode != "pages" || calls == 1, cursor))
				}
			})
			got, err := c.queue("release/main")
			switch mode {
			case "pages":
				require.NoError(t, err)
				require.Equal(t, []queueEntry{queueItem(queueSHA, githubtest.Base, 1), queueItem(queueSHA, githubtest.Base, 2)}, got)
			case "disabled":
				require.NoError(t, err)
				require.Empty(t, got)
			default:
				require.Error(t, err)
				require.Nil(t, got)
				if mode == "limit" {
					require.ErrorContains(t, err, "pagination limit")
					require.Equal(t, 100, calls)
				}
			}
		})
	}
}

func TestMilestoneStatusValidationAndUTF8(t *testing.T) {
	calls := 0
	c := milestoneHTTP(t, func(w http.ResponseWriter, r *http.Request) {
		if emptyCurrentStatus(t, w, r) {
			return
		}
		calls++
		require.Equal(t, "/repos/cloudposse/atmos/statuses/"+githubtest.SHA, r.URL.Path)
		var body map[string]string
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		require.Equal(t, "failure", body["state"])
		require.Equal(t, "Release milestone", body["context"])
		require.Equal(t, strings.Repeat("界", 137)+"...", body["description"])
		require.True(t, utf8.ValidString(body["description"]))
		githubtest.JSON(t, w, map[string]any{})
	})
	require.ErrorIs(t, c.status("invalid", "success", "bad"), ErrMilestonePolicy)
	require.Zero(t, calls)
	require.NoError(t, c.status(githubtest.SHA, "failure", strings.Repeat("界", 141)))
	require.Equal(t, 1, calls)
}

func TestMilestoneGateLivePoliciesAndSharedHeads(t *testing.T) {
	m := Milestone{1, "v1.234.0", "open"}
	future := Milestone{2, "v1.235.0", "open"}
	for _, baselineFails := range []bool{false, true} {
		t.Run(fmt.Sprintf("baseline fails %t", baselineFails), func(t *testing.T) {
			prs := []ReleasePR{policyPR("patch", &m), policyPR("minor", &future), policyPR("no-release", nil), policyPR("no-release", nil)}
			for i := range prs {
				prs[i].Number = i + 1
			}
			prs[0].Head.SHA = githubtest.SHA
			prs[1].Head.SHA = githubtest.Base
			prs[2].Head.SHA = githubtest.Base
			prs[3].Head.SHA = laterQueueSHA
			statuses := map[string][]string{}
			c := milestoneHTTP(t, func(w http.ResponseWriter, r *http.Request) {
				if emptyCurrentStatus(t, w, r) {
					return
				}
				switch r.URL.Path {
				case "/repos/cloudposse/atmos":
					githubtest.JSON(t, w, map[string]string{"default_branch": "release/main"})
				case "/repos/cloudposse/atmos/pulls":
					require.Equal(t, "release/main", r.URL.Query().Get("base"))
					githubtest.JSON(t, w, prs)
				case "/graphql":
					githubtest.JSON(t, w, queueResponse([]queueEntry{queueItem(queueSHA, githubtest.Base, 1)}, false, ""))
				case "/repos/cloudposse/atmos/commits/release/main":
					githubtest.JSON(t, w, map[string]string{"sha": githubtest.SHA})
				case "/repos/cloudposse/atmos/compare/v1.233.0..." + githubtest.SHA:
					githubtest.JSON(t, w, map[string]any{"status": "identical", "total_commits": 0, "commits": []any{}})
				case "/repos/cloudposse/atmos/milestones":
					githubtest.JSON(t, w, []Milestone{m, future})
				case "/repos/cloudposse/atmos/releases":
					if baselineFails {
						http.Error(w, "no baseline", http.StatusServiceUnavailable)
						return
					}
					githubtest.JSON(t, w, []any{stableRelease("v1.233.0")})
				default:
					require.True(t, strings.HasPrefix(r.URL.Path, "/repos/cloudposse/atmos/statuses/"))
					var body map[string]string
					require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
					sha := strings.TrimPrefix(r.URL.Path, "/repos/cloudposse/atmos/statuses/")
					statuses[sha] = append(statuses[sha], body["state"])
					githubtest.JSON(t, w, map[string]any{})
				}
			})
			require.NoError(t, c.checkMilestones("pull_request_target", ""))
			expected := "success"
			if baselineFails {
				expected = "failure"
			}
			require.Equal(t, []string{expected}, statuses[githubtest.SHA])
			require.Equal(t, []string{"failure"}, statuses[githubtest.Base], "a no-release PR cannot overwrite a failure at its shared head")
			require.Equal(t, []string{"success"}, statuses[laterQueueSHA], "no-release does not require a published baseline")
			require.Equal(t, []string{expected}, statuses[queueSHA])
		})
	}
}

func TestMilestoneGateFailuresRemainClosed(t *testing.T) {
	for _, mode := range []string{"repo read", "missing default", "PR read", "queue read", "missing event group", "status failure", "milestone read", "final status"} {
		t.Run(mode, func(t *testing.T) {
			statuses := []string{}
			pr := policyPR("no-release", nil)
			pr.Head.SHA = githubtest.SHA
			c := milestoneHTTP(t, func(w http.ResponseWriter, r *http.Request) {
				if emptyCurrentStatus(t, w, r) {
					return
				}
				switch r.URL.Path {
				case "/repos/cloudposse/atmos":
					if mode == "repo read" {
						http.Error(w, "repo failure", http.StatusServiceUnavailable)
						return
					}
					branch := "main"
					if mode == "missing default" {
						branch = ""
					}
					githubtest.JSON(t, w, map[string]string{"default_branch": branch})
				case "/repos/cloudposse/atmos/pulls":
					if mode == "PR read" {
						http.Error(w, "PR failure", http.StatusServiceUnavailable)
						return
					}
					githubtest.JSON(t, w, []ReleasePR{pr})
				case "/graphql":
					if mode == "queue read" {
						http.Error(w, "queue failure", http.StatusServiceUnavailable)
						return
					}
					githubtest.JSON(t, w, queueResponse(nil, false, ""))
				case "/repos/cloudposse/atmos/milestones":
					if mode == "milestone read" {
						http.Error(w, "milestone failure", http.StatusServiceUnavailable)
						return
					}
					githubtest.JSON(t, w, []Milestone{})
				case "/repos/cloudposse/atmos/releases":
					githubtest.JSON(t, w, []any{stableRelease("v1.233.0")})
				default:
					var body map[string]string
					require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
					statuses = append(statuses, body["state"])
					if mode == "status failure" || mode == "final status" && body["state"] == "success" {
						http.Error(w, "status failure", http.StatusServiceUnavailable)
						return
					}
					githubtest.JSON(t, w, map[string]any{})
				}
			})
			eventHead := ""
			if mode == "missing event group" {
				eventHead = queueSHA
			}
			require.Error(t, c.reconcileMilestoneStatuses(eventHead))
			if mode == "queue read" {
				require.Equal(t, []string{"error"}, statuses, "a queue API failure must invalidate prior successful PR-head checks")
			}
			if mode != "final status" && mode != "status failure" {
				require.NotContains(t, statuses, "success")
			}
		})
	}
}

func TestMergeGroupEvent(t *testing.T) {
	for _, mode := range []string{"missing directory", "missing file", "invalid JSON", "invalid SHA", "status failed", "reconciliation failed", "success"} {
		t.Run(mode, func(t *testing.T) {
			file := filepath.Join(t.TempDir(), "event.json")
			if mode == "missing directory" {
				file = filepath.Join(t.TempDir(), "missing", "event.json")
			}
			event := `{"merge_group":{"head_sha":"` + queueSHA + `"}}`
			if mode == "invalid JSON" {
				event = "{"
			}
			if mode == "invalid SHA" {
				event = `{"merge_group":{"head_sha":"short"}}`
			}
			if mode != "missing file" && mode != "missing directory" {
				require.NoError(t, os.WriteFile(file, []byte(event), 0o600))
			}
			statuses := []string{}
			pr := policyPR("no-release", nil)
			pr.Head.SHA = githubtest.SHA
			c := milestoneHTTP(t, func(w http.ResponseWriter, r *http.Request) {
				if emptyCurrentStatus(t, w, r) {
					return
				}
				switch r.URL.Path {
				case "/repos/cloudposse/atmos":
					if mode == "reconciliation failed" {
						http.Error(w, "cannot read repository", http.StatusServiceUnavailable)
						return
					}
					githubtest.JSON(t, w, map[string]string{"default_branch": "main"})
				case "/repos/cloudposse/atmos/pulls":
					githubtest.JSON(t, w, []ReleasePR{pr})
				case "/graphql":
					githubtest.JSON(t, w, queueResponse([]queueEntry{queueItem(queueSHA, githubtest.Base, pr.Number)}, false, ""))
				case "/repos/cloudposse/atmos/milestones":
					githubtest.JSON(t, w, []Milestone{})
				case "/repos/cloudposse/atmos/releases":
					githubtest.JSON(t, w, []any{stableRelease("v1.233.0")})
				default:
					var body map[string]string
					require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
					if strings.HasSuffix(r.URL.Path, queueSHA) {
						statuses = append(statuses, body["state"])
					}
					if mode == "status failed" {
						http.Error(w, "cannot write status", http.StatusServiceUnavailable)
						return
					}
					githubtest.JSON(t, w, map[string]any{})
				}
			})
			var err error
			if mode == "success" {
				t.Setenv("GITHUB_EVENT_NAME", "merge_group")
				t.Setenv("GITHUB_EVENT_PATH", file)
				err = (Milestones{}).Check(githubtest.Repo)
			} else {
				err = c.checkMilestones("merge_group", file)
			}
			if mode == "success" {
				require.NoError(t, err)
				require.Equal(t, []string{"success"}, statuses)
			} else {
				require.Error(t, err)
				if mode == "status failed" {
					require.Equal(t, []string{"success", "error"}, statuses, "a failed final write must attempt to revoke the known merge-group head")
				} else {
					require.NotContains(t, statuses, "success")
				}
			}
			if mode == "reconciliation failed" {
				require.Equal(t, []string{"error"}, statuses)
			}
		})
	}
}

func TestMilestoneGateQueueFinalStatus(t *testing.T) {
	for _, failQueue := range []bool{false, true} {
		t.Run(fmt.Sprintf("queue status fails %t", failQueue), func(t *testing.T) {
			statuses := map[string][]string{}
			pr := policyPR("no-release", nil)
			pr.Head.SHA = githubtest.SHA
			entries := []queueEntry{queueItem(githubtest.SHA, githubtest.Base, pr.Number)}
			if failQueue {
				entries = append(entries, queueItem(queueSHA, githubtest.SHA, pr.Number))
			}
			c := milestoneHTTP(t, func(w http.ResponseWriter, r *http.Request) {
				if emptyCurrentStatus(t, w, r) {
					return
				}
				switch r.URL.Path {
				case "/repos/cloudposse/atmos":
					githubtest.JSON(t, w, map[string]string{"default_branch": "main"})
				case "/repos/cloudposse/atmos/pulls":
					githubtest.JSON(t, w, []ReleasePR{pr})
				case "/graphql":
					githubtest.JSON(t, w, queueResponse(entries, false, ""))
				case "/repos/cloudposse/atmos/milestones":
					githubtest.JSON(t, w, []Milestone{})
				case "/repos/cloudposse/atmos/releases":
					githubtest.JSON(t, w, []any{stableRelease("v1.233.0")})
				default:
					var body map[string]string
					require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
					sha := strings.TrimPrefix(r.URL.Path, "/repos/cloudposse/atmos/statuses/")
					statuses[sha] = append(statuses[sha], body["state"])
					if failQueue && sha == queueSHA {
						http.Error(w, "queue status unavailable", http.StatusServiceUnavailable)
						return
					}
					githubtest.JSON(t, w, map[string]any{})
				}
			})
			err := c.reconcileMilestoneStatuses("")
			if failQueue {
				require.ErrorContains(t, err, "503")
				require.Equal(t, []string{"success", "error"}, statuses[githubtest.SHA])
				require.Equal(t, []string{"success", "error"}, statuses[queueSHA])
			} else {
				require.NoError(t, err)
				require.Equal(t, []string{"success"}, statuses[githubtest.SHA], "shared PR/queue head must have one final verdict")
			}
		})
	}
}

func emptyCurrentStatus(t *testing.T, w http.ResponseWriter, r *http.Request) bool {
	t.Helper()
	if r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/status") {
		githubtest.JSON(t, w, map[string]any{"statuses": []any{}})
		return true
	}
	return false
}
