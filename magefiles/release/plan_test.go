//go:build mage

package release

import (
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/internal/ci/releasenotes"
	"github.com/cloudposse/atmos/magefiles/internal/githubtest"
)

const planPrefix = "/repos/cloudposse/atmos/"

// planPath maps repository and nested endpoints to release fixture routes.
func planPath(path string) string {
	return strings.TrimPrefix(strings.TrimPrefix(path, strings.TrimSuffix(planPrefix, "/")), "/")
}

func planPull(number int, label string, milestone *Milestone) releasePull {
	pr := releasePull{ReleasePR: policyPR(label, milestone), Title: "  Fix\n credentials\t handling  ", Body: "PR body is data", Merged: true, MergeCommit: githubtest.SHA}
	pr.Number = number
	pr.Base.Ref = "main"
	pr.User.Login = "contributor"
	return pr
}

func planRoutes() map[string]any {
	m := Milestone{Number: 1, Title: "1.234.0", State: "open"}
	return map[string]any{
		"immutable-releases":                    map[string]bool{"enabled": true},
		"milestones":                            []Milestone{{Number: 2, Title: "v1.235.0", State: "open"}, m},
		"releases":                              []any{stableRelease("v1.233.0")},
		"":                                      map[string]string{"default_branch": "main"},
		"commits/main":                          map[string]string{"sha": githubtest.SHA},
		"commits/" + githubtest.SHA:             map[string]string{"sha": githubtest.SHA},
		"compare/" + githubtest.SHA + "...main": map[string]string{"status": "identical"},
		"issues":                                []map[string]any{{"number": 100, "state": "closed"}, {"number": 101, "state": "closed", "pull_request": nil}, {"number": 42, "state": "closed", "pull_request": map[string]string{"url": "https://api.github.com/repos/cloudposse/atmos/pulls/42"}}},
		"pulls/42":                              planPull(42, "patch", &m),
		"pulls/43":                              planPull(43, "no-release", nil),
		"compare/" + githubtest.SHA + "..." + githubtest.SHA: map[string]string{"status": "identical"},
		"compare/v1.233.0..." + githubtest.SHA:               map[string]any{"status": "ahead", "total_commits": 2, "commits": []map[string]string{{"sha": githubtest.SHA}, {"sha": githubtest.Base}}},
		"commits/" + githubtest.SHA + "/pulls":               []map[string]int{{"number": 43}, {"number": 42}},
		"commits/" + githubtest.Base + "/pulls":              []map[string]int{{"number": 42}},
	}
}

func planHTTP(t *testing.T, routes map[string]any, failPath string) *milestoneClient {
	t.Helper()
	return milestoneHTTP(t, func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodGet, r.Method, "release planning must never mutate remote state")
		route := planPath(r.URL.Path)
		if route == failPath {
			http.Error(w, "unavailable "+route, http.StatusServiceUnavailable)
			return
		}
		value, ok := routes[route]
		if !ok {
			t.Errorf("unexpected request %s", r.URL)
			http.Error(w, "unexpected request", http.StatusNotFound)
			return
		}
		if route == "issues" {
			require.Equal(t, "all", r.URL.Query().Get("state"))
			require.Equal(t, "1", r.URL.Query().Get("milestone"))
		}
		githubtest.JSON(t, w, value)
	})
}

func TestReleasePlanFreezesMilestoneVersionAndSource(t *testing.T) {
	for _, source := range []string{"", githubtest.SHA} {
		t.Run("source "+source, func(t *testing.T) {
			c := planHTTP(t, planRoutes(), "unreachable")
			plan, err := c.plan("cut", source)
			require.NoError(t, err)
			require.Equal(t, &Plan{Repository: githubtest.Repo, Milestone: Milestone{Number: 1, Title: "1.234.0", State: "open"}, Version: "v1.234.0", Commit: githubtest.SHA, PreviousTag: "v1.233.0", Branch: "main", Mode: "cut", Entries: []releasenotes.PREntry{
				{Number: 43, Category: "", Summary: "Fix credentials handling @contributor (#43)", Body: "PR body is data"},
				{Number: 42, Category: "## 🐛 Bug Fixes", Summary: "Fix credentials handling @contributor (#42)", Body: "PR body is data"},
			}}, plan, "milestone sets the version, even when all release changes are patch-sized; PR notes are sorted and deduplicated")
		})
	}
}

func TestReleasePlanIncludesLandedStackMembers(t *testing.T) {
	routes := planRoutes()
	for _, key := range []string{"pulls/42", "pulls/43"} {
		pr := routes[key].(releasePull)
		pr.Base.Ref = "stack/parent"
		routes[key] = pr
	}
	c := planHTTP(t, routes, "unreachable")
	plan, err := c.plan("cut", githubtest.SHA)
	require.NoError(t, err)
	require.Equal(t, []releasenotes.PREntry{
		{Number: 43, Category: "", Summary: "Fix credentials handling @contributor (#43)", Body: "PR body is data"},
		{Number: 42, Category: "## 🐛 Bug Fixes", Summary: "Fix credentials handling @contributor (#42)", Body: "PR body is data"},
	}, plan.Entries, "landed stack members retain their release notes and release-label policy")
}

func TestReleasePlanCandidateAllowsUnfinishedWorkAndNoChanges(t *testing.T) {
	routes := planRoutes()
	routes["issues"] = []map[string]any{{"number": 42, "state": "open", "pull_request": map[string]any{}}, {"number": 100, "state": "open"}}
	routes["compare/v1.233.0..."+githubtest.SHA] = map[string]any{"status": "identical", "total_commits": 0, "commits": []any{}}
	c := planHTTP(t, routes, "unreachable")
	p, err := c.plan("candidate", githubtest.SHA)
	require.NoError(t, err)
	require.Equal(t, []int{42}, p.Unfinished)
	require.Empty(t, p.Entries)
	require.Equal(t, "v1.234.0", p.Version)
	require.Equal(t, "candidate", p.Mode)
	_, err = c.plan("cut", githubtest.SHA)
	require.ErrorIs(t, err, ErrMilestonePolicy)
	require.ErrorContains(t, err, "unmerged PRs: [42]")
}

func TestReleasePlanValidationFailures(t *testing.T) {
	for _, tc := range []struct {
		name, mode, commit, want string
		change                   func(map[string]any)
	}{
		{name: "unsupported mode", mode: "publish", want: "unsupported release mode"},
		{name: "no release train", want: "no open versioned milestone", change: func(r map[string]any) { r["milestones"] = []Milestone{{Number: 1, Title: "Backlog", State: "open"}} }},
		{name: "already published", want: "must exceed", change: func(r map[string]any) { r["releases"] = []any{stableRelease("v1.234.0")} }},
		{name: "missing branch", want: "default branch missing", change: func(r map[string]any) { r[""] = map[string]any{} }},
		{name: "short source", commit: "abc", want: "full source SHA required"},
		{name: "malformed resolved source", want: "source SHA did not resolve exactly", change: func(r map[string]any) { r["commits/main"] = map[string]string{"sha": "short"} }},
		{name: "different resolved source", commit: githubtest.SHA, want: "source SHA did not resolve exactly", change: func(r map[string]any) { r["commits/"+githubtest.SHA] = map[string]string{"sha": githubtest.Base} }},
		{name: "unreviewed fork source", want: "source is not on the default branch", change: func(r map[string]any) {
			r["compare/"+githubtest.SHA+"...main"] = map[string]string{"status": "diverged"}
		}},
		{name: "merged milestone PR missing source", want: "milestone PR #42 is absent from source SHA", change: func(r map[string]any) {
			r["compare/"+githubtest.SHA+"..."+githubtest.SHA] = map[string]string{"status": "behind"}
		}},
		{name: "merged PR missing merge SHA", want: "has no merge SHA", change: func(r map[string]any) { pr := r["pulls/42"].(releasePull); pr.MergeCommit = ""; r["pulls/42"] = pr }},
		{name: "empty cut", want: "no changes since", change: func(r map[string]any) {
			r["compare/v1.233.0..."+githubtest.SHA] = map[string]any{"status": "identical", "total_commits": 0, "commits": []any{}}
		}},
		{name: "direct commit without PR", want: "has no associated PR", change: func(r map[string]any) { r["commits/"+githubtest.Base+"/pulls"] = []any{} }},
		{name: "unmerged associated PR", want: "no eligible merged PR", change: func(r map[string]any) {
			r["issues"] = []map[string]any{} // Exercise commit association independently of milestone membership.
			pr := r["pulls/42"].(releasePull)
			pr.Merged = false
			r["pulls/42"] = pr
			r["commits/"+githubtest.SHA+"/pulls"] = []map[string]int{{"number": 42}}
		}},
		{name: "unlanded branch associated PR", want: "no eligible merged PR", change: func(r map[string]any) {
			r["issues"] = []map[string]any{} // Exercise commit association independently of milestone membership.
			pr := r["pulls/42"].(releasePull)
			pr.Base.Ref = "other"
			pr.MergeCommit = strings.Repeat("c", 40)
			r["pulls/42"] = pr
			r["commits/"+githubtest.SHA+"/pulls"] = []map[string]int{{"number": 42}}
		}},
		{name: "major change into minor", want: "labeled major cannot enter minor", change: func(r map[string]any) {
			m := Milestone{Number: 1, Title: "1.234.0", State: "open"}
			r["pulls/42"] = planPull(42, "major", &m)
		}},
		{name: "future milestone included", want: "must target lowest open milestone", change: func(r map[string]any) {
			m := Milestone{Number: 2, Title: "v1.235.0", State: "open"}
			r["pulls/42"] = planPull(42, "patch", &m)
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			routes := planRoutes()
			if tc.change != nil {
				tc.change(routes)
			}
			c := planHTTP(t, routes, "unreachable")
			mode := tc.mode
			if mode == "" {
				mode = "cut"
			}
			p, err := c.plan(mode, tc.commit)
			require.Nil(t, p)
			require.ErrorIs(t, err, ErrMilestonePolicy)
			require.ErrorContains(t, err, tc.want)
		})
	}
}

func TestReleasePlanAPIFailures(t *testing.T) {
	for _, path := range []string{"milestones", "releases", "", "commits/main", "compare/" + githubtest.SHA + "...main", "issues", "pulls/42", "compare/" + githubtest.SHA + "..." + githubtest.SHA, "compare/v1.233.0..." + githubtest.SHA, "commits/" + githubtest.SHA + "/pulls", "pulls/43"} {
		t.Run(path, func(t *testing.T) {
			c := planHTTP(t, planRoutes(), path)
			p, err := c.plan("cut", "")
			require.Nil(t, p)
			require.ErrorContains(t, err, "503")
		})
	}
}

func TestReleasePlanRejectsClosedUnmergedMilestonePR(t *testing.T) {
	routes := planRoutes()
	items := routes["issues"].([]map[string]any)
	routes["issues"] = append(items, map[string]any{"number": 44, "state": "closed", "pull_request": map[string]any{}})
	pr := planPull(44, "patch", nil)
	pr.Merged = false
	routes["pulls/44"] = pr
	c := planHTTP(t, routes, "unreachable")
	p, err := c.plan("cut", "")
	require.Nil(t, p)
	require.ErrorIs(t, err, ErrMilestonePolicy)
	require.ErrorContains(t, err, "milestone PR #44 is not merged; merge it or move it out of the milestone")
}

func TestReleasePlanPreviewIssueDoesNotBlockCut(t *testing.T) {
	routes := planRoutes()
	items := routes["issues"].([]map[string]any)
	routes["issues"] = append(items, map[string]any{"number": 104, "state": "open", "title": "Release preview v1.234.0"}, map[string]any{"number": 105, "state": "open", "pull_request": nil})
	c := planHTTP(t, routes, "unreachable")
	p, err := c.plan("cut", "")
	require.NoError(t, err)
	require.Empty(t, p.Unfinished, "informational issues are not unmerged PRs")
	require.Equal(t, "v1.234.0", p.Version)
	require.Equal(t, githubtest.SHA, p.Commit)
	require.Equal(t, []int{43, 42}, []int{p.Entries[0].Number, p.Entries[1].Number})
}

func TestReleasePreflightRejectsPatchAfterMinorPRMerged(t *testing.T) {
	for _, mode := range []string{"check", "cut", "candidate"} {
		t.Run(mode, func(t *testing.T) {
			routes := planRoutes()
			patch := Milestone{Number: 1, Title: "v1.233.1", State: "open"}
			minor := Milestone{Number: 2, Title: "v1.234.0", State: "open"}
			routes["milestones"] = []Milestone{patch, minor}
			routes["issues"] = []any{}
			routes["pulls/42"] = planPull(42, "minor", &minor)
			c := planHTTP(t, routes, "unreachable")
			p, err := c.commandPlan(mode, "true", "")
			require.Nil(t, p)
			require.ErrorIs(t, err, ErrMilestonePolicy)
			require.ErrorContains(t, err, "cannot release v1.233.1: source already includes merged PR #42 assigned to v1.234.0")
		})
	}
}
