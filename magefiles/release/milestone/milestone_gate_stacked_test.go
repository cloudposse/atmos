//go:build mage

package milestone

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/cloudposse/atmos/magefiles/internal/githubtest"
	"github.com/stretchr/testify/require"
)

// TestMilestoneGateStackedQueueEntries covers native PR stacks: every layer is
// queued into the default branch, but upper layers target their parent branch
// and are therefore absent from the base-filtered open PR list.
func TestMilestoneGateStackedQueueEntries(t *testing.T) {
	const stackedHead = "eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee"
	for _, tc := range []struct {
		mode, state, message string
	}{
		{mode: "open stacked layer", state: "success"},
		{mode: "stacked layer fails policy", state: "failure", message: "PR #2 needs patch, minor, major or no-release"},
		{mode: "closed stacked layer", state: "failure", message: "queue PR #2 is missing from live open PRs"},
		{mode: "lookup error", state: "error", message: statusEvidenceError},
	} {
		t.Run(tc.mode, func(t *testing.T) {
			bottom := policyPR("no-release", nil)
			bottom.Number = 1
			bottom.Head.SHA = githubtest.SHA
			bottom.Base.Ref = "main"
			stacked := policyPR("no-release", nil)
			if tc.mode == "stacked layer fails policy" {
				stacked.Labels = nil
			}
			if tc.mode == "closed stacked layer" {
				stacked.State = "closed"
			}
			stacked.Number = 2
			stacked.Head.SHA = stackedHead
			stacked.Base.Ref = "feat/bottom-layer"
			entries := []queueEntry{queueItem(queueSHA, githubtest.Base, 1), queueItem(laterQueueSHA, queueSHA, 2)}
			statuses := map[string]commitStatus{}
			lookups := 0
			c := milestoneHTTP(t, func(w http.ResponseWriter, r *http.Request) {
				if emptyCurrentStatus(t, w, r) {
					return
				}
				switch r.URL.Path {
				case "/repos/cloudposse/atmos":
					githubtest.JSON(t, w, map[string]string{"default_branch": "main"})
				case "/repos/cloudposse/atmos/pulls":
					require.Equal(t, "main", r.URL.Query().Get("base"))
					githubtest.JSON(t, w, []ReleasePR{bottom})
				case "/repos/cloudposse/atmos/pulls/2":
					lookups++
					if tc.mode == "lookup error" {
						http.Error(w, "unavailable", http.StatusServiceUnavailable)
						return
					}
					githubtest.JSON(t, w, stacked)
				case "/graphql":
					githubtest.JSON(t, w, queueResponse(entries, false, ""))
				case "/repos/cloudposse/atmos/milestones":
					githubtest.JSON(t, w, []Milestone{})
				case "/repos/cloudposse/atmos/releases":
					githubtest.JSON(t, w, []any{stableRelease("v1.233.0")})
				default:
					require.True(t, strings.HasPrefix(r.URL.Path, "/repos/cloudposse/atmos/statuses/"), "unexpected request: %s", r.URL.Path)
					var body commitStatus
					require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
					statuses[strings.TrimPrefix(r.URL.Path, "/repos/cloudposse/atmos/statuses/")] = body
					githubtest.JSON(t, w, map[string]any{})
				}
			})
			err := c.reconcileMilestoneStatuses(laterQueueSHA)
			require.Equal(t, 1, lookups, "only the PR missing from the open list is looked up, once")
			// The first group contains only the open bottom layer, so the later
			// layer's outcome must never taint it.
			if tc.mode == "lookup error" {
				require.Error(t, err)
				require.Equal(t, "error", statuses[queueSHA].State, "unreadable evidence revokes every known head")
			} else {
				require.NoError(t, err)
				require.Equal(t, "success", statuses[queueSHA].State)
			}
			group := statuses[laterQueueSHA]
			require.Equal(t, tc.state, group.State)
			if tc.message == "" {
				require.Equal(t, "Milestone ordering and release-label compatibility verified", group.Description)
			} else {
				require.Contains(t, group.Description, tc.message)
			}
			if tc.mode == "open stacked layer" || tc.mode == "stacked layer fails policy" {
				require.Equal(t, tc.state, statuses[stackedHead].State, "an open stacked PR's own head gets the same verdict")
			} else if tc.mode == "closed stacked layer" {
				require.NotContains(t, statuses, stackedHead, "a closed PR is not evaluated")
			}
		})
	}
}

// TestMilestoneGateStackedLookupRevokesFetchedHeads ensures a lookup failure
// after an earlier successful lookup still revokes the earlier PR's own head,
// so a success left by a previous run cannot survive this failed run.
func TestMilestoneGateStackedLookupRevokesFetchedHeads(t *testing.T) {
	const (
		fetchedHead = "eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee"
		topQueueSHA = "ffffffffffffffffffffffffffffffffffffffff"
	)
	fetched := policyPR("no-release", nil)
	fetched.Number = 2
	fetched.Head.SHA = fetchedHead
	entries := []queueEntry{
		queueItem(queueSHA, githubtest.Base, 1),
		queueItem(laterQueueSHA, queueSHA, 2),
		queueItem(topQueueSHA, laterQueueSHA, 3),
	}
	statuses := map[string]string{}
	c := milestoneHTTP(t, func(w http.ResponseWriter, r *http.Request) {
		if emptyCurrentStatus(t, w, r) {
			return
		}
		switch r.URL.Path {
		case "/repos/cloudposse/atmos":
			githubtest.JSON(t, w, map[string]string{"default_branch": "main"})
		case "/repos/cloudposse/atmos/pulls":
			githubtest.JSON(t, w, []ReleasePR{})
		case "/repos/cloudposse/atmos/pulls/1", "/repos/cloudposse/atmos/pulls/2":
			pr := fetched
			if r.URL.Path == "/repos/cloudposse/atmos/pulls/1" {
				pr.Number, pr.Head.SHA = 1, githubtest.SHA
			}
			githubtest.JSON(t, w, pr)
		case "/repos/cloudposse/atmos/pulls/3":
			http.Error(w, "unavailable", http.StatusServiceUnavailable)
		case "/graphql":
			githubtest.JSON(t, w, queueResponse(entries, false, ""))
		default:
			require.True(t, strings.HasPrefix(r.URL.Path, "/repos/cloudposse/atmos/statuses/"), "unexpected request: %s", r.URL.Path)
			var body commitStatus
			require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
			statuses[strings.TrimPrefix(r.URL.Path, "/repos/cloudposse/atmos/statuses/")] = body.State
			githubtest.JSON(t, w, map[string]any{})
		}
	})
	require.Error(t, c.reconcileMilestoneStatuses(topQueueSHA))
	require.Equal(t, "error", statuses[fetchedHead], "a PR fetched before the failed lookup must have its head revoked")
	require.Equal(t, "error", statuses[githubtest.SHA], "the first fetched PR's head is revoked too")
	for _, head := range []string{queueSHA, laterQueueSHA, topQueueSHA} {
		require.Equal(t, "error", statuses[head], "queue head %s must be revoked", head)
	}
}
