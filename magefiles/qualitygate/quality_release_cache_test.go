//go:build mage

package qualitygate

import (
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/magefiles/internal/githubapi"
	"github.com/cloudposse/atmos/magefiles/internal/githubtest"
	"github.com/cloudposse/atmos/magefiles/release/milestone"
)

type releaseEvidenceFixture struct {
	calls    map[string]int
	failure  string
	conflict bool
}

func (f *releaseEvidenceFixture) client(t *testing.T) *qualityClient {
	t.Helper()
	f.calls = map[string]int{}
	q := qualityTestClient(t, func(path string, out any) error {
		key := ""
		var value any
		switch {
		case strings.Contains(path, "milestones?"):
			key, value = "milestones", []milestone.Milestone{{Number: 1, Title: "v1.234.0", State: "open"}}
		case strings.Contains(path, "releases?"):
			key, value = "releases", []map[string]string{{"tag_name": "v1.233.0", "published_at": "2026-10-01T00:00:00Z"}}
		case path == "repos/"+githubtest.Repo:
			key, value = "repository", map[string]string{"default_branch": "main"}
		case strings.HasSuffix(path, "/commits/main"):
			key, value = "head", map[string]string{"sha": githubtest.SHA}
		case strings.Contains(path, "/compare/"):
			key, value = "compare", map[string]any{"status": "ahead", "total_commits": 1, "commits": []map[string]string{{"sha": githubtest.SHA}}}
		case strings.Contains(path, "/commits/"+githubtest.SHA+"/pulls"):
			key, value = "associated", []map[string]int{{"number": 44}}
		case strings.HasSuffix(path, "/pulls/44"):
			pr := milestone.PullRequest{ReleasePR: milestone.ReleasePR{Number: 44, Milestone: &milestone.Milestone{Number: 1, Title: "v1.234.0", State: "open"}}, Merged: true}
			pr.Base.Ref = "main"
			pr.Labels = []struct {
				Name string `json:"name"`
			}{{Name: "patch"}}
			if f.conflict {
				pr.Milestone.Title = "v1.235.0"
			}
			key, value = "included", pr
		default:
			return errors.New("unrelated evidence unavailable")
		}
		f.calls[key]++
		if key == f.failure {
			return errors.New("release evidence unavailable")
		}
		return githubtest.Respond(value, out)
	})
	q.api.(*githubtest.FakeAPI).Write = func(_, _ string, _ io.Reader) error {
		return errors.New("unrelated GraphQL evidence unavailable")
	}
	return q
}

func releaseEvidencePR() githubapi.PR {
	pr := githubtest.PR(githubtest.Run())
	pr.Labels = []struct {
		Name string `json:"name"`
	}{{Name: "patch"}}
	pr.Milestone = &githubapi.PRMilestone{Number: 1, Title: "v1.234.0", State: "open"}
	return pr
}

func TestQualityReleaseEvidenceSharedAcrossPRsAndResetBetweenRuns(t *testing.T) {
	f := &releaseEvidenceFixture{}
	q := f.client(t)
	for run := 1; run <= 2; run++ {
		require.NoError(t, q.reconcileAll(nil))
		for _, number := range []int{42, 43} {
			pr := releaseEvidencePR()
			pr.Number = number
			// Reporter and render both copy the client before evaluating a PR.
			scoped := *q
			report := scoped.render(&pr, nil, "", nil)
			require.True(t, report.releasePolicyPassed)
		}
		for _, key := range []string{"milestones", "releases", "repository", "head", "compare", "associated", "included"} {
			require.Equal(t, run, f.calls[key], key)
		}
		invalid := releaseEvidencePR()
		invalid.Milestone.Title = "v1.235.0"
		gate, err := q.releasePolicy(&invalid)
		require.NoError(t, err)
		require.False(t, gate.Passed, "per-PR assignment must still be checked against shared evidence")
	}
}

func TestQualityReleaseEvidenceCachesFailuresWithoutBlockingNoRelease(t *testing.T) {
	for _, failure := range []string{"milestones", "releases", "compare", "policy"} {
		t.Run(failure, func(t *testing.T) {
			f := &releaseEvidenceFixture{failure: failure, conflict: failure == "policy"}
			q := f.client(t)
			pr := releaseEvidencePR()
			for range 2 {
				report := q.render(&pr, nil, "", nil)
				require.False(t, report.releasePolicyPassed)
				if failure == "policy" {
					require.Contains(t, report.body, "merged PR #44")
				} else {
					require.Contains(t, report.body, "release evidence unavailable")
				}
			}
			for key, calls := range f.calls {
				require.Equal(t, 1, calls, key)
			}
			pr.Labels[0].Name = "no-release"
			gate, err := q.releasePolicy(&pr)
			require.NoError(t, err)
			require.True(t, gate.Passed)
			// A subsequent run must retry previously unavailable repository state.
			f.failure, f.conflict = "", false
			require.NoError(t, q.reconcileAll(nil))
			pr.Labels[0].Name = "patch"
			require.True(t, q.render(&pr, nil, "", nil).releasePolicyPassed)
		})
	}
}
