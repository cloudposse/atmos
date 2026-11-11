//go:build mage

package qualitygate

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/magefiles/internal/githubapi"
	"github.com/cloudposse/atmos/magefiles/internal/githubtest"
	"github.com/cloudposse/atmos/magefiles/release/milestone"
)

func TestQualityReleasePolicy(t *testing.T) {
	for _, tc := range []struct {
		name, label, target, next, failure string
		pass                               bool
		historyConflict                    bool
	}{
		{name: "no-release without milestone", label: "no-release", pass: true},
		{name: "missing label"},
		{name: "conflicting labels", label: "no-release,patch"},
		{name: "missing milestone", label: "patch", next: "v1.234.0"},
		{name: "patch after merged minor", label: "patch", target: "v1.233.1", next: "v1.233.1", historyConflict: true},
		{name: "history API failure", label: "patch", target: "v1.233.1", next: "v1.233.1", failure: "/compare/"},
		{name: "patch into patch", label: "patch", target: "v1.233.1", next: "v1.233.1", pass: true},
		{name: "patch into minor", label: "patch", target: "v1.234.0", next: "v1.234.0", pass: true},
		{name: "minor into patch", label: "minor", target: "v1.233.1", next: "v1.233.1"},
		{name: "major into minor", label: "major", target: "v1.234.0", next: "v1.234.0"},
		{name: "minor into major", label: "minor", target: "v2.0.0", next: "v2.0.0", pass: true},
		{name: "future release", label: "patch", target: "v1.235.0", next: "v1.234.0"},
		{name: "closed milestone", label: "patch", target: "v1.234.0", next: "v1.234.0"},
		{name: "milestone API failure", label: "patch", target: "v1.234.0", next: "v1.234.0", failure: "milestones?"},
		{name: "release API failure", label: "patch", target: "v1.234.0", next: "v1.234.0", failure: "releases?"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			pr := githubtest.PR(githubtest.Run())
			pr.Labels = nil
			for _, label := range strings.Split(tc.label, ",") {
				pr.Labels = append(pr.Labels, struct {
					Name string `json:"name"`
				}{label})
			}
			if tc.target != "" {
				pr.Milestone = &githubapi.PRMilestone{Number: 1, Title: tc.target, State: "open"}
				if tc.name == "closed milestone" {
					pr.Milestone.State = "closed"
				}
			}
			q := qualityTestClient(t, func(path string, out any) error {
				require.NotEqual(t, "no-release", tc.label, "exemption must not require release API availability")
				if tc.failure != "" && strings.Contains(path, tc.failure) {
					return errors.New("API unavailable")
				}
				if strings.Contains(path, "milestones?") {
					return githubtest.Respond([]milestone.Milestone{{Number: 1, Title: tc.next, State: "open"}}, out)
				}
				switch {
				case path == "repos/"+githubtest.Repo:
					return githubtest.Respond(map[string]string{"default_branch": "main"}, out)
				case strings.HasSuffix(path, "/commits/main"):
					return githubtest.Respond(map[string]string{"sha": githubtest.SHA}, out)
				case strings.Contains(path, "/compare/"):
					if tc.historyConflict {
						return githubtest.Respond(map[string]any{"status": "ahead", "total_commits": 1, "commits": []map[string]string{{"sha": githubtest.SHA}}}, out)
					}
					return githubtest.Respond(map[string]any{"status": "identical", "total_commits": 0, "commits": []any{}}, out)
				}
				if strings.Contains(path, "/commits/"+githubtest.SHA+"/pulls") {
					return githubtest.Respond([]map[string]int{{"number": 44}}, out)
				}
				if strings.HasSuffix(path, "/pulls/44") {
					merged := milestone.PullRequest{ReleasePR: milestone.ReleasePR{Number: 44, Milestone: &milestone.Milestone{Number: 2, Title: "v1.234.0", State: "open"}}, Merged: true}
					merged.Base.Ref = "main"
					merged.Labels = pr.Labels
					return githubtest.Respond(merged, out)
				}
				require.Contains(t, path, "releases?")
				return githubtest.Respond([]map[string]any{{"tag_name": "v1.233.0", "published_at": "2026-10-01T00:00:00Z"}}, out)
			})
			registry, err := defaultQualityRegistry()
			require.NoError(t, err)
			for i := range registry.definitions {
				if registry.definitions[i].ID != "release-policy" {
					registry.definitions[i].Check = func(*qualityClient, *githubapi.PR) (qualityGate, error) { return qualityGate{Passed: true}, nil }
				}
			}
			gates := registry.evaluate(q, &pr)
			body, ready := qualityMarkdown(&pr, gates, "")
			require.Equal(t, tc.pass, ready)
			require.Contains(t, body, "Release label and milestone")
			if tc.historyConflict {
				require.Contains(t, body, "cannot release v1.233.1")
				require.Contains(t, body, "merged PR #44 assigned to v1.234.0")
			}

			if tc.failure != "" {
				require.Contains(t, body, "GitHub evidence was unavailable")
			}
			if !tc.pass && tc.failure == "" {
				require.Contains(t, body, "Preserve the intended release")
			}
		})
	}
}

func TestQualityRejectsReleaseAssignmentChangedDuringEvaluation(t *testing.T) {
	for _, change := range []string{"label", "milestone assigned", "milestone removed", "number", "title", "state", "unrelated label"} {
		t.Run(change, func(t *testing.T) {
			pr := githubtest.PR(githubtest.Run())
			pr.Milestone = &githubapi.PRMilestone{Number: 1, Title: "v1.234.0", State: "open"}
			current := githubtest.PR(githubtest.Run())
			copy := *pr.Milestone
			current.Milestone = &copy
			switch change {
			case "label":
				current.Labels[0].Name = "patch"
			case "milestone assigned":
				pr.Milestone = nil
			case "milestone removed":
				current.Milestone = nil
			case "number":
				current.Milestone.Number++
			case "title":
				current.Milestone.Title = "v1.235.0"
			case "state":
				current.Milestone.State = "closed"
			case "unrelated label":
				current.Labels = append(current.Labels, struct {
					Name string `json:"name"`
				}{qualityReady})
			}
			require.Equal(t, change == "unrelated label", qualitySameRevision(&pr, &current, githubtest.Repo))
		})
	}
}

func TestQualityReleasePolicyUnavailableInvalidatesPublishedReadiness(t *testing.T) {
	for _, scenario := range []string{"assignment changed with missing milestones", "same assignment with missing baseline", "new baseline with missing reviews", "incompatible label with missing reviews"} {
		t.Run(scenario, func(t *testing.T) {
			state := &stateFixture{labels: []string{qualityReady}, reviewsFail: strings.Contains(scenario, "missing reviews")}
			current := state.pr()
			current.Labels = []struct {
				Name string `json:"name"`
			}{{Name: "patch"}}
			current.Milestone = &githubapi.PRMilestone{Number: 1, Title: "v1.234.0", State: "open"}
			previous := current
			if scenario == "assignment changed with missing milestones" || scenario == "incompatible label with missing reviews" {
				previous = state.pr()
				current.Labels[0].Name = "minor"
				current.Milestone.Title = "v1.233.1"
			}
			state.comment, _ = qualityMarkdown(&previous, nil, "Performance measurements pending for this commit.")
			q := state.client(t)
			api := q.api.(*qualityGraphAPI)
			get := api.Get
			api.Get = func(path string, response any) error {
				switch {
				case strings.HasSuffix(path, "/pulls/42"):
					return githubtest.Respond(current, response)
				case strings.Contains(path, "/milestones?"):
					if scenario == "assignment changed with missing milestones" {
						return errors.New("milestone evidence unavailable")
					}
					return githubtest.Respond([]githubapi.PRMilestone{*current.Milestone}, response)
				case strings.Contains(path, "/releases?"):
					if scenario == "same assignment with missing baseline" {
						return errors.New("published baseline unavailable")
					}
					baseline := "v1.233.0"
					if scenario == "new baseline with missing reviews" {
						baseline = "v1.234.0"
					}
					return githubtest.Respond([]map[string]string{{"tag_name": baseline, "published_at": "2026-10-01T00:00:00Z"}}, response)
				default:
					return get(path, response)
				}
			}
			require.NoError(t, q.refresh(current.Number, "", nil))
			require.Contains(t, state.writes, "DELETE repos/"+githubtest.Repo+"/issues/42/labels/"+qualityReady)
			require.Len(t, state.comments, 1)
			require.Contains(t, state.comments[0], "Quality Gate not ready")
			require.Contains(t, state.comments[0], "Release label and milestone")
			require.Contains(t, state.comments[0], "Unavailable:", "missing evidence must remain honestly reported")
			if state.reviewsFail {
				require.Contains(t, state.comments[0], "Preserve the intended release")
			}
		})
	}
}

func TestQualityReleaseAssignmentIgnoresLabelOrdering(t *testing.T) {
	pr := githubtest.PR(githubtest.Run())
	pr.Labels = []struct {
		Name string `json:"name"`
	}{{Name: "major"}, {Name: "patch"}}
	current := pr
	current.Labels = []struct {
		Name string `json:"name"`
	}{{Name: "patch"}, {Name: "major"}, {Name: qualityReady}}
	require.True(t, qualitySameRevision(&pr, &current, githubtest.Repo))
	current.Labels[0].Name = "minor"
	require.False(t, qualitySameRevision(&pr, &current, githubtest.Repo), "a policy change must invalidate the snapshot even if both label sets are already conflicting")
}

func TestQualityReleaseMetadataEventsSelectAffectedPRs(t *testing.T) {
	for _, tc := range []struct {
		name, event, payload string
		want                 []int
	}{
		{"merged PR", "pull_request_target", `{"action":"closed","number":42,"pull_request":{"merged":true}}`, []int{42, 43}},
		{"PR milestone assignment", "pull_request_target", `{"action":"milestoned","number":42,"pull_request":{"number":42}}`, []int{42}},
		{"PR release label", "pull_request_target", `{"action":"labeled","number":42,"label":{"name":"minor"}}`, []int{42}},
		{"lower milestone created", "milestone", `{"action":"created","milestone":{"number":99}}`, []int{42, 43}},
		{"stable release published", "release", `{"action":"published","release":{"id":99,"tag_name":"v1.234.0"}}`, []int{42, 43}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "event.json")
			require.NoError(t, os.WriteFile(path, []byte(tc.payload), 0o600))
			t.Setenv("GITHUB_EVENT_NAME", tc.event)
			t.Setenv("GITHUB_EVENT_PATH", path)
			q := qualityTestClient(t, func(path string, out any) error {
				require.Len(t, tc.want, 2, "repository state changes should select all open PRs")
				require.Contains(t, path, "pulls?state=open")
				pr := githubtest.PR(githubtest.Run())
				pr.Updated = time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
				other := pr
				other.Number = 43
				return githubtest.Respond([]githubapi.PR{pr, other}, out)
			})
			q.trueUp = true
			got, err := q.eventPRs(time.Now())
			require.NoError(t, err)
			require.Equal(t, tc.want, got, "repository release metadata invalidates all open PRs regardless of their last commit age")
		})
	}
}
