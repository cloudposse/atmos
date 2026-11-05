//go:build mage

package release

import (
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/cloudposse/atmos/magefiles/internal/githubtest"
	"github.com/stretchr/testify/require"
)

type previewFixture struct {
	milestones             []Milestone
	issues                 map[int][]previewIssue
	pulls                  map[int]releasePull
	pinned                 []string
	writes                 []string
	mutations              []string
	fail                   string
	graphqlError           string
	failUnpin              bool
	createdIdentityMissing bool
}

func newPreviewFixture() *previewFixture {
	return &previewFixture{milestones: []Milestone{{Number: 1, Title: "v1.234.0", State: "open"}}, issues: map[int][]previewIssue{}, pulls: map[int]releasePull{}}
}

func ownedPreview(number int, state string) previewIssue {
	p := previewIssue{Number: number, NodeID: fmt.Sprintf("I_%d", number), Title: "Old title", Body: previewMarker(1) + "\nOld body", State: state}
	p.User.ID = 41898282
	p.User.Login = "github-actions[bot]"
	return p
}

func (f *previewFixture) client(t *testing.T) *milestoneClient {
	t.Helper()
	return milestoneHTTP(t, func(w http.ResponseWriter, r *http.Request) {
		route := planPath(r.URL.Path)
		key := r.Method + " " + route
		if key == f.fail {
			http.Error(w, "injected failure", http.StatusServiceUnavailable)
			return
		}
		if r.URL.Path == "/graphql" {
			f.graphql(t, w, r)
			return
		}
		switch {
		case key == "GET milestones":
			githubtest.JSON(t, w, f.milestones)
		case key == "GET issues":
			require.Equal(t, "all", r.URL.Query().Get("state"))
			number, err := strconv.Atoi(r.URL.Query().Get("milestone"))
			require.NoError(t, err)
			githubtest.JSON(t, w, f.issues[number])
		case strings.HasPrefix(key, "GET pulls/"):
			number, err := strconv.Atoi(strings.TrimPrefix(route, "pulls/"))
			require.NoError(t, err)
			pr, ok := f.pulls[number]
			require.True(t, ok, "unexpected PR lookup %d", number)
			githubtest.JSON(t, w, pr)
		case key == "POST issues" || strings.HasPrefix(key, "PATCH issues/"):
			f.writes = append(f.writes, key)
			var payload struct {
				Title     string `json:"title"`
				Body      string `json:"body"`
				Milestone int    `json:"milestone"`
				State     string `json:"state"`
			}
			require.NoError(t, json.NewDecoder(r.Body).Decode(&payload))
			if r.Method == http.MethodPost {
				issue := ownedPreview(90, payload.State)
				issue.Title = payload.Title
				issue.Body = payload.Body
				f.issues[payload.Milestone] = append(f.issues[payload.Milestone], issue)
				if f.createdIdentityMissing {
					issue.NodeID = ""
				}
				githubtest.JSON(t, w, issue)
			} else {
				number, err := strconv.Atoi(strings.TrimPrefix(route, "issues/"))
				require.NoError(t, err)
				found := false
				for i, issue := range f.issues[payload.Milestone] {
					if issue.Number == number {
						f.issues[payload.Milestone][i].Title = payload.Title
						f.issues[payload.Milestone][i].Body = payload.Body
						f.issues[payload.Milestone][i].State = payload.State
						found = true
					}
				}
				require.True(t, found, "must update only known owned issue")
				githubtest.JSON(t, w, map[string]any{})
			}
		default:
			t.Errorf("unexpected request %s", key)
			http.Error(w, "unexpected request", http.StatusNotFound)
		}
	})
}

func TestReleasePreviewsCreatesStableInformationalIssueAndIsIdempotent(t *testing.T) {
	f := newPreviewFixture()
	f.milestones = append(f.milestones, Milestone{Number: 2, Title: "v1.234.0-feature.42.100", State: "open"}, Milestone{Number: 3, Title: "Backlog", State: "open"}, Milestone{Number: 4, Title: "v1.233.0", State: "closed"})
	c := f.client(t)
	require.NoError(t, (Release{}).Previews(githubtest.Repo))
	require.Equal(t, []string{"POST issues"}, f.writes)
	require.Equal(t, []string{"pin I_90"}, f.mutations)
	issue := f.issues[1][0]
	require.Equal(t, "Release v1.234.0", issue.Title)
	require.Equal(t, "open", issue.State)
	require.Contains(t, issue.Body, "https://github.com/cloudposse/atmos/milestone/1")
	require.Contains(t, issue.Body, "preview, not a release approval")
	require.Contains(t, issue.Body, "No pull requests assigned yet.")
	require.Contains(t, issue.Body, previewMarker(1))
	require.NoError(t, c.previews())
	require.Len(t, f.writes, 1, "unchanged preview must not be rewritten")
	require.Len(t, f.mutations, 1, "already pinned issue must not be pinned twice")
}

func TestReleasePreviewsUpdatesOnlyOwnedIssueAndListsPRStates(t *testing.T) {
	f := newPreviewFixture()
	spoof := ownedPreview(80, "open")
	spoof.User.ID = 42
	wrongLogin := ownedPreview(81, "open")
	wrongLogin.User.Login = "another-bot"
	wrongMarker := ownedPreview(82, "open")
	wrongMarker.Body = previewMarker(2) + "\nspoof"
	f.issues[1] = []previewIssue{spoof, wrongLogin, wrongMarker, ownedPreview(90, "open"), {Number: 42, PullRequest: json.RawMessage(`{}`)}, {Number: 43, PullRequest: json.RawMessage(`{}`)}, {Number: 44, PullRequest: json.RawMessage(`{}`)}, {Number: 45, PullRequest: json.RawMessage(`null`)}}
	merged := planPull(42, "patch", nil)
	merged.Title = "  Fix\n credentials  "
	merged.Body = "<!-- hidden -->Useful description\nSecond line"
	f.pulls[42] = merged
	open := planPull(43, "patch", nil)
	open.Merged = false
	open.Title = "Next fix"
	open.Body = ""
	f.pulls[43] = open
	closed := planPull(44, "patch", nil)
	closed.Merged = false
	closed.State = "closed"
	closed.Title = "Abandoned"
	closed.Body = "<!-- This is an auto-generated comment: release notes by coderabbit.ai -->\n## Summary by CodeRabbit\nUseful summary\n<!-- end of auto-generated comment: release notes by coderabbit.ai -->"
	f.pulls[44] = closed
	require.NoError(t, f.client(t).previews())
	require.Equal(t, []string{"PATCH issues/90"}, f.writes)
	require.Equal(t, spoof, f.issues[1][0])
	require.Equal(t, wrongLogin, f.issues[1][1])
	require.Equal(t, wrongMarker, f.issues[1][2])
	body := f.issues[1][3].Body
	require.Contains(t, body, "- #42 — Fix credentials (merged)")
	require.Contains(t, body, "- #43 — Next fix (open)")
	require.Contains(t, body, "- #44 — Abandoned (closed, unmerged — blocks release)")
	require.Contains(t, body, "  Useful description\n  Second line")
	require.Contains(t, body, "  Useful summary")
	require.NotContains(t, body, "<!-- hidden -->")
	require.NotContains(t, body, "#45")
}

func TestReleasePreviewsClosesAndUnpinsWithoutReopeningManualClosure(t *testing.T) {
	for _, tc := range []struct{ name, milestoneState, issueState string }{{"milestone closed", "closed", "open"}, {"issue manually closed", "open", "closed"}} {
		t.Run(tc.name, func(t *testing.T) {
			f := newPreviewFixture()
			f.milestones[0].State = tc.milestoneState
			f.issues[1] = []previewIssue{ownedPreview(90, tc.issueState)}
			f.pinned = []string{"I_unrelated", "I_90"}
			c := f.client(t)
			require.NoError(t, c.previews())
			require.Equal(t, "closed", f.issues[1][0].State)
			require.Equal(t, []string{"unpin I_90"}, f.mutations)
			require.Equal(t, []string{"I_unrelated"}, f.pinned)
			if tc.milestoneState == "closed" {
				require.Contains(t, f.issues[1][0].Body, "Milestone closed.")
			}
			require.NoError(t, c.previews())
			require.Len(t, f.mutations, 1)
			require.Len(t, f.writes, 1)
		})
	}
}

func TestReleasePreviewPinCapacityNeverEvictsExistingIssues(t *testing.T) {
	f := newPreviewFixture()
	f.pinned = []string{"I_first", "I_second", "I_third"}
	require.NoError(t, f.client(t).previews())
	require.Empty(t, f.mutations)
	require.Equal(t, []string{"I_first", "I_second", "I_third"}, f.pinned)
	require.Len(t, f.issues[1], 1, "preview is still created when all pin slots are occupied")
}

func TestReleasePreviewsRejectDuplicateOwnedMarkers(t *testing.T) {
	f := newPreviewFixture()
	f.issues[1] = []previewIssue{ownedPreview(90, "open"), ownedPreview(91, "closed")}
	err := f.client(t).previews()
	require.ErrorIs(t, err, ErrMilestonePolicy)
	require.ErrorContains(t, err, "duplicate release preview")
	require.Empty(t, f.writes)
	require.Empty(t, f.mutations)
}

func TestReleasePreviewsStopsOnFailures(t *testing.T) {
	for _, tc := range []struct {
		name, fail, want string
		setup            func(*previewFixture)
	}{
		{name: "milestones API", fail: "GET milestones", want: "HTTP 503"},
		{name: "issues API", fail: "GET issues", want: "HTTP 503"},
		{name: "PR API", fail: "GET pulls/42", want: "HTTP 503", setup: func(f *previewFixture) {
			f.issues[1] = []previewIssue{{Number: 42, PullRequest: json.RawMessage(`{}`)}}
		}},
		{name: "create API", fail: "POST issues", want: "HTTP 503"},
		{name: "update API", fail: "PATCH issues/90", want: "HTTP 503", setup: func(f *previewFixture) { f.issues[1] = []previewIssue{ownedPreview(90, "open")} }},
		{name: "missing created identity", want: "missing created preview identity", setup: func(f *previewFixture) { f.createdIdentityMissing = true }},
		{name: "graphql authorization", want: "preview GraphQL: forbidden", setup: func(f *previewFixture) { f.graphqlError = "forbidden" }},
		{name: "oversized preview", want: "exceeds issue body limit", setup: func(f *previewFixture) {
			for i := 1; i <= 60; i++ {
				f.issues[1] = append(f.issues[1], previewIssue{Number: i, PullRequest: json.RawMessage(`{}`)})
				pr := planPull(i, "patch", nil)
				pr.Body = strings.Repeat("x", 1200)
				f.pulls[i] = pr
			}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newPreviewFixture()
			f.fail = tc.fail
			if tc.setup != nil {
				tc.setup(f)
			}
			err := f.client(t).previews()
			require.ErrorContains(t, err, tc.want)
			require.Empty(t, f.mutations)
		})
	}
}

func TestReleasePreviewGraphQLValidation(t *testing.T) {
	t.Run("missing identity", func(t *testing.T) {
		c := newPreviewFixture().client(t)
		require.ErrorContains(t, c.pinPreview("", true), "preview node ID missing")
	})
	t.Run("invalid repository", func(t *testing.T) { require.Error(t, (Release{}).Previews("invalid")) })
	t.Run("invalid variables", func(t *testing.T) {
		c := newPreviewFixture().client(t)
		err := c.previewGraphQL("query", map[string]any{"unsupported": make(chan int)}, nil)
		require.ErrorContains(t, err, "unsupported type")
	})
	for _, tc := range []struct {
		name     string
		response string
		status   int
		want     string
	}{
		{"transport error", "unavailable", 503, "503"},
		{"invalid data", `{"data":"wrong shape"}`, 200, "cannot unmarshal"},
		{"missing data", `{}`, 200, "unexpected end of JSON input"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := milestoneHTTP(t, func(w http.ResponseWriter, r *http.Request) {
				require.Equal(t, "/graphql", r.URL.Path)
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tc.status)
				_, err := w.Write([]byte(tc.response))
				require.NoError(t, err)
			})
			require.ErrorContains(t, c.pinPreview("I_90", true), tc.want)
		})
	}
}

func TestReleasePreviewReopensAfterMilestoneReopens(t *testing.T) {
	f := newPreviewFixture()
	f.issues[1] = []previewIssue{ownedPreview(90, "open")}
	f.pinned = []string{"I_90"}
	c := f.client(t)
	f.milestones[0].State = "closed"
	require.NoError(t, c.previews())
	require.Equal(t, "closed", f.issues[1][0].State)
	require.Contains(t, f.issues[1][0].Body, "<!-- atmos-release-preview:milestone-closed -->")
	f.milestones[0].State = "open"
	require.NoError(t, c.previews())
	require.Equal(t, "open", f.issues[1][0].State)
	require.NotContains(t, f.issues[1][0].Body, "<!-- atmos-release-preview:milestone-closed -->")
	require.Equal(t, []string{"unpin I_90", "pin I_90"}, f.mutations)
}

func TestReleasePreviewNeverTreatsBotPullRequestAsOwnedIssue(t *testing.T) {
	f := newPreviewFixture()
	pr := ownedPreview(42, "open")
	pr.PullRequest = json.RawMessage(`{}`)
	f.issues[1] = []previewIssue{pr}
	f.pulls[42] = planPull(42, "patch", nil)
	require.NoError(t, f.client(t).previews())
	require.Equal(t, []string{"POST issues"}, f.writes)
	require.Equal(t, pr, f.issues[1][0], "a PR with a copied marker is never rewritten as a release issue")
	require.Contains(t, f.issues[1][1].Body, "#42")
}

func (f *previewFixture) graphql(t *testing.T, w http.ResponseWriter, r *http.Request) {
	t.Helper()
	require.Equal(t, http.MethodPost, r.Method)
	var request struct {
		Query     string            `json:"query"`
		Variables map[string]string `json:"variables"`
	}
	require.NoError(t, json.NewDecoder(r.Body).Decode(&request))
	if f.graphqlError != "" {
		githubtest.JSON(t, w, map[string]any{"errors": []map[string]string{{"message": f.graphqlError}}})
		return
	}
	if strings.HasPrefix(request.Query, "query") {
		require.Equal(t, map[string]string{"owner": "cloudposse", "name": "atmos"}, request.Variables)
		require.Contains(t, request.Query, "nodes{issue{id}}", "pinnedIssues contains PinnedIssue wrappers, not direct issues")
		nodes := []map[string]any{}
		for _, id := range f.pinned {
			nodes = append(nodes, map[string]any{"issue": map[string]string{"id": id}})
		}
		githubtest.JSON(t, w, map[string]any{"data": map[string]any{"repository": map[string]any{"pinnedIssues": map[string]any{"nodes": nodes}}}})
		return
	}
	id := request.Variables["id"]
	if strings.Contains(request.Query, "{unpinIssue(") {
		if f.failUnpin {
			githubtest.JSON(t, w, map[string]any{"errors": []map[string]string{{"message": "temporary unpin failure"}}})
			return
		}
		f.mutations = append(f.mutations, "unpin "+id)
		f.pinned = slices.DeleteFunc(f.pinned, func(value string) bool { return value == id })
	} else {
		require.Contains(t, request.Query, "{pinIssue(")
		f.mutations = append(f.mutations, "pin "+id)
		f.pinned = append(f.pinned, id)
	}
	githubtest.JSON(t, w, map[string]any{"data": map[string]any{}})
}

func TestReleasePreviewFinalizedSkipsPRReadsAndRepairsUnpin(t *testing.T) {
	for _, pinned := range []bool{false, true} {
		t.Run(fmt.Sprintf("pinned=%t", pinned), func(t *testing.T) {
			f := newPreviewFixture()
			f.milestones[0].State = previewClosed
			preview := ownedPreview(90, previewClosed)
			preview.Body += "\n" + milestoneClosedMarker
			f.issues[1] = []previewIssue{preview, {Number: 42, PullRequest: json.RawMessage(`{}`)}}
			f.fail = "GET pulls/42"
			if pinned {
				f.pinned = []string{"I_unrelated", preview.NodeID}
				f.failUnpin = true
			}
			c := f.client(t)
			if pinned {
				require.ErrorContains(t, c.previews(), "temporary unpin failure")
				f.failUnpin = false
			}
			require.NoError(t, c.previews(), "finalized previews must not fetch old PRs")
			require.Equal(t, preview, f.issues[1][0])
			require.Empty(t, f.writes)
			if pinned {
				require.Equal(t, []string{"unpin I_90"}, f.mutations)
				require.Equal(t, []string{"I_unrelated"}, f.pinned)
			} else {
				require.Empty(t, f.mutations)
			}
		})
	}
}

func TestReleasePreviewUnfinishedStillRenders(t *testing.T) {
	for _, state := range []string{previewOpen, previewClosed} {
		t.Run(state, func(t *testing.T) {
			f := newPreviewFixture()
			f.milestones[0].State = previewClosed
			preview := ownedPreview(90, state)
			if state == previewOpen {
				preview.Body += "\n" + milestoneClosedMarker
			}
			f.issues[1] = []previewIssue{preview, {Number: 42, PullRequest: json.RawMessage(`{}`)}}
			f.fail = "GET pulls/42"
			require.ErrorContains(t, f.client(t).previews(), "HTTP 503", "both closure and the finalization marker are required to skip rendering")
		})
	}
}

func TestReleasePreviewGroupsPRsLikeReleaseNotes(t *testing.T) {
	f := newPreviewFixture()
	f.issues[1] = []previewIssue{{Number: 45, PullRequest: json.RawMessage(`{}`)}, {Number: 44, PullRequest: json.RawMessage(`{}`)}, {Number: 42, PullRequest: json.RawMessage(`{}`)}, {Number: 43, PullRequest: json.RawMessage(`{}`)}, {Number: 41, PullRequest: json.RawMessage(`{}`)}}
	for number, label := range map[int]string{41: "minor", 42: "patch", 43: "no-release", 44: "minor", 45: "patch"} {
		pr := planPull(number, label, nil)
		pr.Title = fmt.Sprintf("Change %d", number)
		pr.Body = ""
		f.pulls[number] = pr
	}
	require.NoError(t, f.client(t).previews())
	body := createdPreviewBody(t, f)
	// Same order as the published release: uncategorized first, then
	// configured categories, then PR number within each category.
	order := []string{
		"### 📦 Other Changes", "- #43 — Change 43",
		"### ✨ Features", "- #41 — Change 41", "- #44 — Change 44",
		"### 🐛 Bug Fixes", "- #42 — Change 42", "- #45 — Change 45",
	}
	last := -1
	for _, want := range order {
		index := strings.Index(body, want)
		require.Greater(t, index, last, "%q out of order in:\n%s", want, body)
		last = index
	}
	require.Equal(t, 1, strings.Count(body, "### ✨ Features"))
	require.NotContains(t, body, "No pull requests assigned yet.")
}

func TestReleasePreviewOmitsOtherChangesHeadingWhenNothingIsCategorized(t *testing.T) {
	f := newPreviewFixture()
	f.issues[1] = []previewIssue{{Number: 43, PullRequest: json.RawMessage(`{}`)}, {Number: 42, PullRequest: json.RawMessage(`{}`)}}
	f.pulls[42] = planPull(42, "no-release", nil)
	f.pulls[43] = planPull(43, "no-release", nil)
	require.NoError(t, f.client(t).previews())
	body := createdPreviewBody(t, f)
	require.NotContains(t, body, "###")
	require.Less(t, strings.Index(body, "- #42 —"), strings.Index(body, "- #43 —"))
}

func createdPreviewBody(t *testing.T, f *previewFixture) string {
	t.Helper()
	i := slices.IndexFunc(f.issues[1], func(issue previewIssue) bool { return issue.Number == 90 })
	require.GreaterOrEqual(t, i, 0, "preview issue must be created")
	return f.issues[1][i].Body
}
