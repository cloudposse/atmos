//go:build mage

package qualitygate

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/cloudposse/atmos/magefiles/internal/githubapi"
	"github.com/cloudposse/atmos/magefiles/internal/githubtest"
	"github.com/stretchr/testify/require"
)

// runAPI lets a test make one PR hang until its context ends, or react to
// requests, on top of the publish fixtures.
type runAPI struct {
	*qualityGraphAPI
	slow  string
	after func(path string)
}

func (r *runAPI) DoWithContext(ctx context.Context, method, path string, body io.Reader, response interface{}) error {
	if method == http.MethodGet && r.slow != "" && strings.HasSuffix(path, r.slow) {
		<-ctx.Done()
		return ctx.Err()
	}
	err := r.qualityGraphAPI.DoWithContext(ctx, method, path, body, response)
	if r.after != nil {
		r.after(path)
	}
	return err
}

type runFixture struct {
	writes   []string
	requests []string
}

func newRunClient(ctx context.Context, t *testing.T, slow string, after func(string)) (*qualityClient, *runFixture) {
	t.Helper()
	run := githubtest.Run()
	run.Event = "pull_request"
	pr := githubtest.PR(run)
	count, reads := 149, 0
	pr.ChangedFiles = &count
	getter := qualityPublishGetter(t, "create", &pr, &run, &reads)
	fixture := &runFixture{}
	fake := &githubtest.FakeAPI{T: t}
	fake.Get = func(path string, response any) error {
		fixture.requests = append(fixture.requests, path)
		switch {
		case strings.HasSuffix(path, "/pulls/41") || strings.HasSuffix(path, "/pulls/39"):
			return errors.New("API unavailable for " + path[strings.LastIndex(path, "/")+1:])
		case strings.Contains(path, "actions/workflows/"):
			return githubtest.Respond(map[string]any{"workflow_runs": []githubapi.Run{}}, response)
		}
		return getter(path, response)
	}
	fake.Write = func(method, path string, _ io.Reader) error {
		fixture.writes = append(fixture.writes, method+" "+path)
		return nil
	}
	api := &runAPI{qualityGraphAPI: &qualityGraphAPI{FakeAPI: fake, t: t, scenario: "valid"}, slow: slow, after: after}
	t.Setenv("GITHUB_STEP_SUMMARY", "")
	return &qualityClient{ctx: ctx, api: api, repo: githubtest.Repo, prTimeout: 100 * time.Millisecond}, fixture
}

func TestQualityReconcileAllContinuesAfterFailures(t *testing.T) {
	// PR 41 fails, PR 40 hangs until its own deadline, PR 42 must still be
	// published: one bad PR never starves the ones behind it.
	q, fixture := newRunClient(context.Background(), t, "/pulls/40", nil)
	started := time.Now()
	require.NoError(t, q.reconcileAll([]int{41, 40, 42}))
	require.Less(t, time.Since(started), 10*time.Second)
	require.Equal(t, []string{
		"POST repos/" + githubtest.Repo + "/issues/42/labels",
		"POST repos/" + githubtest.Repo + "/issues/42/comments",
	}, fixture.writes)
	require.Contains(t, fixture.requests[0], "/pulls/41")
	require.Contains(t, fixture.requests[len(fixture.requests)-1], "42")
}

func TestQualityReconcileAllFailsOnlyWhenEveryPRFailed(t *testing.T) {
	q, _ := newRunClient(context.Background(), t, "/pulls/40", nil)
	err := q.reconcileAll([]int{41, 40, 39})
	require.Error(t, err)
	for _, want := range []string{"PR #41", "PR #40", "PR #39", "API unavailable for 41", "deadline exceeded"} {
		require.ErrorContains(t, err, want)
	}
	require.NoError(t, q.reconcileAll(nil), "nothing to reconcile is not a failure")
}

func TestQualityReconcileAllRunBudget(t *testing.T) {
	t.Run("exhausted before any PR", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		q, fixture := newRunClient(ctx, t, "", nil)
		require.ErrorContains(t, q.reconcileAll([]int{42, 43}), "run budget exhausted")
		require.Empty(t, fixture.requests)
	})
	t.Run("exhausted midway leaves later PRs for the next run", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		q, fixture := newRunClient(ctx, t, "", func(path string) {
			if strings.HasSuffix(path, "/pulls/42") {
				cancel()
			}
		})
		require.NoError(t, q.reconcileAll([]int{42, 43, 44}))
		for _, request := range fixture.requests {
			require.NotContains(t, request, "/pulls/43")
			require.NotContains(t, request, "/pulls/44")
		}
	})
}

func TestQualityAnnotationEscapesWorkflowCommands(t *testing.T) {
	require.Equal(t, "100%25 failed%0Anext%0Dline", qualityAnnotation("100% failed\nnext\rline"))
}

// stateFixture serves one PR whose published comment and labels are chosen by
// the test, and counts writes.
type stateFixture struct {
	comment     string // Body of the gate comment; empty means none.
	labels      []string
	reviewsFail bool
	writes      []string
	comments    []string
}

func (s *stateFixture) client(t *testing.T) *qualityClient {
	t.Helper()
	run := githubtest.Run()
	run.Event = "pull_request"
	pr := githubtest.PR(run)
	count, reads := 149, 0
	pr.ChangedFiles = &count
	getter := qualityPublishGetter(t, "create", &pr, &run, &reads)
	fake := &githubtest.FakeAPI{T: t}
	fake.Get = func(path string, response any) error {
		switch {
		case strings.Contains(path, "/comments?"):
			if s.comment == "" {
				return githubtest.Respond([]map[string]any{}, response)
			}
			return githubtest.Respond([]map[string]any{{"id": 17, "body": s.comment, "user": map[string]string{"login": "github-actions[bot]", "type": "Bot"}}}, response)
		case strings.HasSuffix(path, "/issues/42"):
			labels := []map[string]string{{"name": "unrelated"}}
			for _, name := range s.labels {
				labels = append(labels, map[string]string{"name": name})
			}
			return githubtest.Respond(map[string]any{"labels": labels}, response)
		case strings.Contains(path, "/reviews?") && s.reviewsFail:
			return errors.New("reviews unavailable")
		case strings.HasSuffix(path, "/commits/"+githubtest.Base):
			// Scenarios that set MergeSHA to the base use GitHub's synthetic merge commit shape.
			return githubtest.Respond(qualityCommitParents(githubtest.Base, pr.Head.SHA), response)
		}
		return getter(path, response)
	}
	fake.Write = func(method, path string, body io.Reader) error {
		s.writes = append(s.writes, method+" "+path)
		if strings.Contains(path, "comments") {
			var payload map[string]string
			require.NoError(t, json.NewDecoder(body).Decode(&payload))
			s.comments = append(s.comments, payload["body"])
		}
		return nil
	}
	t.Setenv("GITHUB_STEP_SUMMARY", "")
	return &qualityClient{ctx: context.Background(), api: &qualityGraphAPI{FakeAPI: fake, t: t, scenario: "valid"}, repo: githubtest.Repo}
}

func (s *stateFixture) pr() githubapi.PR {
	pr := githubtest.PR(githubtest.Run())
	count := 149
	pr.ChangedFiles = &count
	return pr
}

func TestQualityUnchangedStatePerformsNoWrites(t *testing.T) {
	cases := []struct {
		name       string
		withRun    bool
		reviewFail bool // Renders an unavailable gate, hence a not-ready report.
	}{
		{name: "ready without benchmark marker"},
		{name: "ready with benchmark marker", withRun: true},
		{name: "not ready", reviewFail: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Render the report once, then present it as already published with
			// matching labels: a second pass must change nothing.
			seed := &stateFixture{reviewsFail: tc.reviewFail}
			q := seed.client(t)
			pr := seed.pr()
			var run *githubapi.Run
			if tc.withRun {
				r := githubtest.Run()
				run = &r
			}
			rendered := q.render(&pr, nil, "", run)
			body, ready := rendered.body, rendered.ready
			require.Equal(t, !tc.reviewFail, ready)
			label := qualityNotReady
			if ready {
				label = qualityReady
			}
			published := &stateFixture{comment: body, labels: []string{label}, reviewsFail: tc.reviewFail}
			q = published.client(t)
			if tc.reviewFail {
				// Unavailable evidence keeps the report; the pass must still not write.
				require.ErrorContains(t, q.refresh(42, "", nil), "evidence unavailable")
			} else {
				require.NoError(t, q.refresh(42, "", nil))
			}
			require.Empty(t, published.writes, "an unchanged report and labels must not be rewritten")
			require.Empty(t, published.comments)
		})
	}
}

// TestQualityUnavailableEvidenceKeepsPublishedReport preserves transient failures only for an already reported revision.
func TestQualityUnavailableEvidenceKeepsPublishedReport(t *testing.T) {
	pr := (&stateFixture{}).pr()
	current := qualityMarker + "\n" + qualityHeadMarker(pr.Head.SHA) + "\n" + qualityBaseMarker(&pr) + "\nPassed report\n"
	previous := qualityMarker + "\n" + qualityHeadMarker(githubtest.Base) + "\nOld report\n"
	cases := []struct {
		name       string
		comment    string
		labels     []string
		reviewFail bool
		wantErr    string
		wantWrites []string
		wantBody   string
	}{
		{
			name: "same head and base keeps the last report and labels", comment: current, labels: []string{qualityReady}, reviewFail: true,
			wantErr: "evidence unavailable for CodeRabbit approval (current head)",
		},
		{
			name: "new head publishes the unavailable state", comment: previous, labels: []string{qualityReady}, reviewFail: true,
			wantWrites: []string{
				"DELETE repos/" + githubtest.Repo + "/issues/42/labels/" + qualityReady,
				"POST repos/" + githubtest.Repo + "/issues/42/labels",
				"PATCH repos/" + githubtest.Repo + "/issues/comments/17",
			},
			wantBody: "Unavailable: reviews unavailable",
		},
		{
			name: "first report publishes the unavailable state", reviewFail: true,
			wantWrites: []string{
				"POST repos/" + githubtest.Repo + "/issues/42/labels",
				"POST repos/" + githubtest.Repo + "/issues/42/comments",
			},
			wantBody: "Unavailable: reviews unavailable",
		},
		{
			name: "available evidence replaces the report", comment: current, labels: []string{qualityNotReady},
			wantWrites: []string{
				"DELETE repos/" + githubtest.Repo + "/issues/42/labels/" + qualityNotReady,
				"POST repos/" + githubtest.Repo + "/issues/42/labels",
				"PATCH repos/" + githubtest.Repo + "/issues/comments/17",
			},
			wantBody: "Quality Gate passed",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			state := &stateFixture{comment: tc.comment, labels: tc.labels, reviewsFail: tc.reviewFail}
			err := state.client(t).refresh(42, "", nil)
			if tc.wantErr != "" {
				require.ErrorContains(t, err, tc.wantErr)
			} else {
				require.NoError(t, err)
			}
			require.Equal(t, tc.wantWrites, state.writes)
			if tc.wantBody != "" {
				require.Len(t, state.comments, 1)
				require.Contains(t, state.comments[0], tc.wantBody)
			}
		})
	}
}
