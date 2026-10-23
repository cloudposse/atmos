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

// statusStore models GitHub's latest-per-context read and append-only writes.
type statusStore struct {
	latest       map[string]commitStatus
	writes       map[string][]commitStatus
	readFailure  bool
	writeFailure string
}

func newStatusStore() *statusStore {
	return &statusStore{latest: map[string]commitStatus{}, writes: map[string][]commitStatus{}}
}

func (store *statusStore) handle(t *testing.T, w http.ResponseWriter, r *http.Request) bool {
	t.Helper()
	path := strings.TrimPrefix(strings.TrimPrefix(r.URL.Path, "/repos/cloudposse/atmos"), "/")
	if r.Method == http.MethodGet && strings.HasSuffix(path, "/status") {
		if store.readFailure {
			http.Error(w, "status read unavailable", http.StatusServiceUnavailable)
			return true
		}
		sha := strings.TrimSuffix(strings.TrimPrefix(path, "commits/"), "/status")
		statuses := []commitStatus{{Context: "Other CI", State: "success", Description: "Unrelated"}}
		if current, exists := store.latest[sha]; exists {
			statuses = append(statuses, current)
		}
		githubtest.JSON(t, w, map[string]any{"statuses": statuses})
		return true
	}
	if r.Method != http.MethodPost || !strings.HasPrefix(path, "statuses/") {
		return false
	}
	sha := strings.TrimPrefix(path, "statuses/")
	var status commitStatus
	require.NoError(t, json.NewDecoder(r.Body).Decode(&status))
	require.Equal(t, milestoneStatus, status.Context)
	store.writes[sha] = append(store.writes[sha], status)
	if sha == store.writeFailure && status.State != "error" {
		http.Error(w, "final status unavailable", http.StatusServiceUnavailable)
		return true
	}
	store.latest[sha] = status
	githubtest.JSON(t, w, map[string]any{})
	return true
}

func TestMilestoneStatusSkipsUnchangedStateAndTruncatedDescription(t *testing.T) {
	for _, state := range []string{"success", "failure", "error"} {
		t.Run(state, func(t *testing.T) {
			store := newStatusStore()
			c := milestoneHTTP(t, func(w http.ResponseWriter, r *http.Request) {
				require.True(t, store.handle(t, w, r), "unexpected %s", r.URL)
			})
			description := strings.Repeat("界", statusDescriptionLimit+1)
			for range 3 {
				require.NoError(t, c.status(githubtest.SHA, state, description))
			}
			require.NoError(t, c.status(githubtest.SHA, state, description+"another invisible suffix"))
			require.Equal(t, []commitStatus{{Context: milestoneStatus, State: state, Description: strings.Repeat("界", statusDescriptionLimit-3) + "..."}}, store.writes[githubtest.SHA])
			require.NoError(t, c.status(githubtest.SHA, state, "Changed evidence"))
			require.Len(t, store.writes[githubtest.SHA], 2, "a changed description must still update the status")
			require.Equal(t, "Changed evidence", store.latest[githubtest.SHA].Description)
			require.NoError(t, c.status(githubtest.SHA, "pending", "Changed evidence"))
			require.Equal(t, "pending", store.latest[githubtest.SHA].State, "a changed state must still update the status")
			require.Len(t, store.writes[githubtest.SHA], 3)
		})
	}
}

func TestMilestoneCurrentStatusReadFailureCannotPublishSuccess(t *testing.T) {
	store := newStatusStore()
	store.latest[githubtest.SHA] = commitStatus{Context: milestoneStatus, State: "success", Description: "Previously allowed"}
	store.readFailure = true
	c := milestoneHTTP(t, func(w http.ResponseWriter, r *http.Request) { require.True(t, store.handle(t, w, r)) })
	require.ErrorContains(t, c.status(githubtest.SHA, "success", "New evidence"), "commits/"+githubtest.SHA+"/status")
	require.Equal(t, []commitStatus{{Context: milestoneStatus, State: "error", Description: statusEvidenceError}}, store.writes[githubtest.SHA])
}

func TestMilestoneCurrentStatusPaginatesContexts(t *testing.T) {
	for _, scenario := range []string{"later page", "missing", "page limit"} {
		t.Run(scenario, func(t *testing.T) {
			calls := 0
			expected := commitStatus{Context: milestoneStatus, State: "failure", Description: "Wrong milestone"}
			c := milestoneHTTP(t, func(w http.ResponseWriter, r *http.Request) {
				calls++
				require.Equal(t, http.MethodGet, r.Method)
				require.Equal(t, "/repos/cloudposse/atmos/commits/"+githubtest.SHA+"/status", r.URL.Path)
				require.Equal(t, strconv.Itoa(calls), r.URL.Query().Get("page"))
				require.Equal(t, "100", r.URL.Query().Get("per_page"))
				statuses := make([]commitStatus, 100)
				for i := range statuses {
					statuses[i] = commitStatus{Context: fmt.Sprintf("Other CI %d", i), State: "success"}
				}
				if scenario == "later page" && calls == 2 {
					statuses = []commitStatus{expected}
				}
				if scenario == "missing" {
					statuses = nil
				}
				githubtest.JSON(t, w, map[string]any{"statuses": statuses})
			})
			got, err := c.currentStatus(githubtest.SHA)
			if scenario == "page limit" {
				require.ErrorContains(t, err, "pagination limit")
				require.Equal(t, 100, calls)
				return
			}
			require.NoError(t, err)
			if scenario == "missing" {
				require.Equal(t, commitStatus{}, got)
			} else {
				require.Equal(t, expected, got)
				require.Equal(t, 2, calls)
			}
		})
	}
}

func TestMilestoneReconciliationWritesOnlyPolicyTransitions(t *testing.T) {
	store := newStatusStore()
	allowed := policyPR("no-release", nil)
	allowed.Head.SHA = githubtest.SHA
	blocked := policyPR("patch", nil)
	blocked.Number = 2
	blocked.Head.SHA = githubtest.Base
	shared := policyPR("no-release", nil)
	shared.Number = 3
	shared.Head.SHA = blocked.Head.SHA
	failure := ""
	c := milestoneHTTP(t, func(w http.ResponseWriter, r *http.Request) {
		if store.handle(t, w, r) {
			return
		}
		switch r.URL.Path {
		case "/repos/cloudposse/atmos":
			githubtest.JSON(t, w, map[string]string{"default_branch": "main"})
		case "/repos/cloudposse/atmos/pulls":
			githubtest.JSON(t, w, []ReleasePR{allowed, blocked, shared})
		case "/graphql":
			githubtest.JSON(t, w, queueResponse([]queueEntry{queueItem(queueSHA, githubtest.Base, allowed.Number), queueItem(laterQueueSHA, queueSHA, blocked.Number)}, false, ""))
		case "/repos/cloudposse/atmos/milestones":
			if failure == "milestones" {
				http.Error(w, "milestones unavailable", http.StatusServiceUnavailable)
				return
			}
			githubtest.JSON(t, w, []Milestone{})
		case "/repos/cloudposse/atmos/releases":
			githubtest.JSON(t, w, []any{stableRelease("v1.233.0")})
		case "/repos/cloudposse/atmos/commits/main":
			githubtest.JSON(t, w, map[string]string{"sha": githubtest.SHA})
		case "/repos/cloudposse/atmos/compare/v1.233.0..." + githubtest.SHA:
			githubtest.JSON(t, w, map[string]any{"status": "identical", "total_commits": 0, "commits": []any{}})
		default:
			t.Errorf("unexpected request %s", r.URL)
		}
	})
	for range 4 {
		require.NoError(t, c.checkMilestones("schedule", ""))
	}
	for _, head := range []string{githubtest.SHA, githubtest.Base, queueSHA, laterQueueSHA} {
		require.Len(t, store.writes[head], 1, "unchanged scheduled runs must not consume statuses")
	}
	require.Equal(t, "success", store.latest[githubtest.SHA].State)
	require.Equal(t, "failure", store.latest[githubtest.Base].State, "shared no-release must not mask a blocked PR")
	require.Equal(t, "success", store.latest[queueSHA].State, "a later blocked queue entry must not taint the earlier group")
	require.Equal(t, "failure", store.latest[laterQueueSHA].State)
	failure = "milestones"
	for range 2 {
		require.ErrorContains(t, c.checkMilestones("schedule", ""), "milestones?")
	}
	for _, head := range []string{githubtest.SHA, githubtest.Base, queueSHA, laterQueueSHA} {
		require.Equal(t, commitStatus{Context: milestoneStatus, State: "error", Description: statusEvidenceError}, store.latest[head])
		require.Len(t, store.writes[head], 2, "repeated evidence failure must not append identical error statuses")
	}
	failure = ""
	require.NoError(t, c.checkMilestones("schedule", ""))
	require.Equal(t, "success", store.latest[githubtest.SHA].State)
	require.Equal(t, "failure", store.latest[laterQueueSHA].State)
	blocked.Labels[0].Name = "no-release"
	for range 2 {
		require.NoError(t, c.checkMilestones("schedule", ""))
	}
	require.Equal(t, "success", store.latest[githubtest.Base].State)
	require.Equal(t, "success", store.latest[laterQueueSHA].State)
	require.Len(t, store.writes[githubtest.SHA], 3)
	require.Len(t, store.writes[githubtest.Base], 4)
	// A failed final write must revoke other already-successful known heads too.
	store.writeFailure = queueSHA
	stale := store.latest[queueSHA]
	stale.Description = "Old description requiring an update"
	store.latest[queueSHA] = stale
	require.ErrorContains(t, c.checkMilestones("schedule", ""), "statuses/"+queueSHA)
	for _, head := range []string{githubtest.SHA, githubtest.Base, queueSHA, laterQueueSHA} {
		require.Equal(t, "error", store.latest[head].State)
	}
}

func TestMilestoneEvidenceErrorsRevokeKnownHeads(t *testing.T) {
	for _, scenario := range []string{"repository", "PR list", "queue", "missing event group"} {
		t.Run(scenario, func(t *testing.T) {
			store := newStatusStore()
			for _, head := range []string{githubtest.SHA, queueSHA, laterQueueSHA} {
				store.latest[head] = commitStatus{Context: milestoneStatus, State: "success", Description: "Old success"}
			}
			pr := policyPR("no-release", nil)
			pr.Head.SHA = githubtest.SHA
			c := milestoneHTTP(t, func(w http.ResponseWriter, r *http.Request) {
				if store.handle(t, w, r) {
					return
				}
				switch r.URL.Path {
				case "/repos/cloudposse/atmos":
					if scenario == "repository" {
						http.Error(w, "repository unavailable", http.StatusServiceUnavailable)
						return
					}
					githubtest.JSON(t, w, map[string]string{"default_branch": "main"})
				case "/repos/cloudposse/atmos/pulls":
					if scenario == "PR list" {
						http.Error(w, "PR list unavailable", http.StatusServiceUnavailable)
						return
					}
					githubtest.JSON(t, w, []ReleasePR{pr})
				case "/graphql":
					if scenario == "queue" {
						http.Error(w, "queue unavailable", http.StatusServiceUnavailable)
						return
					}
					githubtest.JSON(t, w, queueResponse([]queueEntry{queueItem(laterQueueSHA, githubtest.Base, pr.Number)}, false, ""))
				default:
					t.Errorf("unexpected %s", r.URL)
				}
			})
			for range 2 {
				require.Error(t, c.reconcileMilestoneStatuses(queueSHA))
			}
			require.Equal(t, "error", store.latest[queueSHA].State, "event head is known even before repository and PR reads")
			require.Len(t, store.writes[queueSHA], 1)
			if scenario == "queue" || scenario == "missing event group" {
				require.Equal(t, "error", store.latest[githubtest.SHA].State, "queue failures must revoke discovered PR heads")
				require.Len(t, store.writes[githubtest.SHA], 1)
			}
			if scenario == "missing event group" {
				require.Equal(t, "error", store.latest[laterQueueSHA].State, "discovered queue heads remain known when event membership fails")
				require.Len(t, store.writes[laterQueueSHA], 1)
			}
		})
	}
}
