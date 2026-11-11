//go:build mage

package qualitygate

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/magefiles/internal/githubapi"
	"github.com/cloudposse/atmos/magefiles/internal/githubtest"
)

func TestQualityStatusRecoveryMetadata(t *testing.T) {
	pr := githubtest.PR(githubtest.Run())
	pr.MergeSHA = githubtest.Base
	for _, tc := range []struct{ name, body, reason string }{
		{"missing marker", "review required", ""},
		{"empty snapshot", qualityStatusBlockersMarker(&pr, nil), ""},
		{"malformed JSON", qualityStatusBlockersPrefix + "oops -->", qualityReasonStatusChanged},
		{"unfinished marker", qualityStatusBlockersPrefix + "[]", qualityReasonStatusChanged},
		{"foreign commit", qualityStatusBlockersPrefix + `[{"sha":"foreign","context":"external","state":"failure"}] -->`, qualityReasonStatusChanged},
	} {
		t.Run(tc.name, func(t *testing.T) {
			q := qualityTestClient(t, func(path string, _ any) error { t.Fatalf("unexpected read %s", path); return nil })
			reason, err := q.changedBlockingStatus(&pr, tc.body)
			require.NoError(t, err)
			require.Equal(t, tc.reason, reason)
		})
	}
}

func TestQualityStatusRecoveryMergeContextsAndPagination(t *testing.T) {
	pr := githubtest.PR(githubtest.Run())
	pr.MergeSHA = githubtest.Base
	context := "context with --> and <html>"
	gate := qualityCIGate(map[string]string{"status: external": "failure", "merge: status: " + context: "pending", "check: test": "failure"})
	gate.ID = "ci"
	marker := qualityStatusBlockersMarker(&pr, []qualityGate{gate, {ID: "review"}, {ID: "ci", Passed: true}})
	require.NotContains(t, marker, context, "third-party names cannot end the hidden comment")
	reads := map[string]int{}
	changed := false
	q := qualityTestClient(t, func(path string, response any) error {
		reads[path]++
		statuses := []map[string]string{}
		switch {
		case strings.Contains(path, pr.Head.SHA):
			statuses = append(statuses, map[string]string{"context": "external", "state": "failure"})
		case strings.HasSuffix(path, "page=1"):
			for i := range githubapi.PageSize {
				statuses = append(statuses, map[string]string{"context": fmt.Sprint(i), "state": "success"})
			}
		default:
			require.Contains(t, path, pr.MergeSHA)
			require.True(t, strings.HasSuffix(path, "page=2"))
			state := "pending"
			if changed {
				state = "success"
			}
			statuses = append(statuses, map[string]string{"context": context, "state": state})
		}
		return githubtest.Respond(map[string]any{"statuses": statuses}, response)
	})
	reason, err := q.changedBlockingStatus(&pr, marker)
	require.NoError(t, err)
	require.Empty(t, reason)
	require.Len(t, reads, 3)
	for _, count := range reads {
		require.Equal(t, 1, count)
	}
	changed = true
	reason, err = q.changedBlockingStatus(&pr, marker)
	require.NoError(t, err)
	require.Equal(t, qualityReasonStatusChanged, reason)
}

func TestQualityStatusRecoveryPaginationLimit(t *testing.T) {
	reads := 0
	q := qualityTestClient(t, func(_ string, response any) error {
		reads++
		return githubtest.Respond(map[string]any{"statuses": make([]map[string]string, githubapi.PageSize)}, response)
	})
	_, err := q.currentStatusStates(githubtest.SHA)
	require.ErrorContains(t, err, "pagination limit")
	require.Equal(t, githubapi.PageLimit, reads)
}

func TestQualityLegacyStatusReportsRefreshOnce(t *testing.T) {
	cutoff := time.Date(2026, 10, 26, 0, 0, 0, 0, time.UTC)
	for _, detail := range []string{"workflow: " + releaseMilestonesWorkflow + ": cancelled", "status: Release milestone: failure", "status: external: pending"} {
		t.Run(detail, func(t *testing.T) {
			server := &scheduleServer{prs: []schedulePR{{number: 42, labeled: true, notReady: true, updated: cutoff.Add(-time.Hour)}}}
			pr := server.listed(&server.prs[0])
			body := qualityMarker + "\n" + qualityHeadMarker(pr.Head.SHA) + "\n" + qualityBaseMarker(&pr) + "\n" + detail
			q := qualityTestClient(t, func(path string, response any) error {
				if strings.Contains(path, "/comments?") {
					comment := githubapi.Comment{ID: 17, Body: body}
					comment.User.Login, comment.User.Type = "github-actions[bot]", "Bot"
					return githubtest.Respond([]githubapi.Comment{comment}, response)
				}
				return server.get(path, response)
			})
			reason, err := q.scheduledReason(&pr, cutoff)
			require.NoError(t, err)
			require.Equal(t, qualityReasonLegacyStatus, reason)
			body, _ = qualityMarkdown(&pr, []qualityGate{{ID: "review", Name: "Review", Detail: "Missing approval"}}, "")
			reason, err = q.scheduledReason(&pr, cutoff)
			require.NoError(t, err)
			require.Empty(t, reason, "the remaining non-status blocker must not cause a recurring refresh")
			require.Empty(t, server.statuses)
		})
	}
}

// Reports published with a status snapshot still recorded the reporter's own
// cancelled run, or an unapproved Quality Gate Signal run, as a CI blocker. Those
// refresh once at any age, including reports carrying the first exclusion marker;
// the fresh report omits the run, so the PR is not selected again.
func TestQualitySelfBlockedReportsRefreshOnce(t *testing.T) {
	cutoff := time.Date(2026, 10, 26, 0, 0, 0, 0, time.UTC)
	conclusions := map[string]string{qualityReportWorkflow: "cancelled", qualityReconcileWorkflow: "cancelled", qualitySignalWorkflow: "action_required"}
	for _, workflow := range []string{qualityReportWorkflow, qualityReconcileWorkflow, qualitySignalWorkflow} {
		t.Run(workflow, func(t *testing.T) {
			server := &scheduleServer{prs: []schedulePR{{number: 42, labeled: true, notReady: true, updated: cutoff.Add(-time.Hour)}}}
			pr := server.listed(&server.prs[0])
			body := qualityMarker + "\n" + qualityHeadMarker(pr.Head.SHA) + "\n" + qualityBaseMarker(&pr) + "\n" +
				qualityStatusBlockersPrefix + "[] -->\n<!-- atmos-quality-reporter-runs-excluded -->\n" +
				"| All CI checks and workflows | Not ready | workflow: " + workflow + ": " + conclusions[workflow] + " |"
			q := qualityTestClient(t, func(path string, response any) error {
				if strings.Contains(path, "/comments?") {
					comment := githubapi.Comment{ID: 17, Body: body}
					comment.User.Login, comment.User.Type = "github-actions[bot]", "Bot"
					return githubtest.Respond([]githubapi.Comment{comment}, response)
				}
				return server.get(path, response)
			})
			reason, err := q.scheduledReason(&pr, cutoff)
			require.NoError(t, err)
			require.Equal(t, qualityReasonLegacyStatus, reason)
			body, _ = qualityMarkdown(&pr, []qualityGate{{ID: "review", Name: "Review", Detail: "Missing approval"}}, "")
			reason, err = q.scheduledReason(&pr, cutoff)
			require.NoError(t, err)
			require.Empty(t, reason, "a fresh report without the reporter's run must not refresh again")
			// An untrusted run of the reporter's path stays a blocker in the fresh
			// report, but its unchanged verdict must not recur on every schedule.
			body, _ = qualityMarkdown(&pr, []qualityGate{{ID: "ci", Name: "All CI checks and workflows", Detail: "workflow: " + workflow + ": failure", Signals: []qualitySignal{{Name: "workflow: " + workflow, State: "failure"}}}}, "")
			require.Contains(t, body, "workflow: "+workflow+": failure")
			reason, err = q.scheduledReason(&pr, cutoff)
			require.NoError(t, err)
			require.Empty(t, reason, "an untrusted reporter-path failure must not select the PR on every schedule")
		})
	}
}

func TestQualityRecoveredStatusPublicationIsStable(t *testing.T) {
	state := &stateFixture{labels: []string{qualityNotReady}}
	q := state.client(t)
	api := q.api.(*qualityGraphAPI).FakeAPI
	get := api.Get
	status := "failure"
	api.Get = func(path string, response any) error {
		statuses := []map[string]string{{"context": "Release milestone", "state": status}}
		switch {
		case strings.Contains(path, "/statuses?"):
			return githubtest.Respond(statuses, response)
		case strings.Contains(path, "/status?"):
			return githubtest.Respond(map[string]any{"state": status, "statuses": statuses}, response)
		default:
			return get(path, response)
		}
	}
	pr := state.pr()
	pr.Updated = time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	pr.Labels = append(pr.Labels, struct {
		Name string `json:"name"`
	}{Name: qualityNotReady})
	require.NoError(t, q.refresh(pr.Number, "", nil))
	state.comment, state.comments, state.writes = state.comments[0], nil, nil
	status = "success"
	reason, err := q.scheduledReason(&pr, pr.Updated.Add(48*time.Hour))
	require.NoError(t, err)
	require.Equal(t, qualityReasonStatusChanged, reason)
	require.NoError(t, q.refresh(pr.Number, "", nil))
	require.Len(t, state.comments, 1)
	require.Contains(t, state.comments[0], "Quality Gate passed")
	state.comment, state.labels = state.comments[0], []string{qualityReady}
	state.comments, state.writes = nil, nil
	require.NoError(t, q.refresh(pr.Number, "", nil))
	require.Empty(t, state.writes, "identical evidence must not rewrite labels or comments")
}
