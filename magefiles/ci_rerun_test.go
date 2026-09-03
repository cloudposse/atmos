//go:build mage

package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cli/go-gh/v2/pkg/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/cloudposse/atmos/internal/ci/rerun"
)

// zombieRunJobs is a single page (one HTTP response body); FetchJobs' own
// multi-page pagination is covered separately in internal/ci/rerun/fetch_test.go.
const zombieRunJobs = `{"jobs":[
  {"name":"Build (linux)","status":"completed","conclusion":"success","completed_at":"2026-09-03T16:09:36Z","steps":[
    {"name":"Complete job","status":"completed","conclusion":"success","completed_at":"2026-09-03T16:09:36Z"}]},
  {"name":"Build (windows)","status":"completed","conclusion":"cancelled","completed_at":"2026-09-03T16:43:17Z","steps":[
    {"name":"Set up job","status":"completed","conclusion":"success","completed_at":"2026-09-03T16:00:00Z"},
    {"name":"Complete job","status":"completed","conclusion":"success","completed_at":"2026-09-03T16:09:36Z"}]},
  {"name":"Acceptance Tests (windows)","status":"completed","conclusion":"failure","completed_at":"2026-09-03T16:43:31Z","steps":[
    {"name":"Check per-OS test matrix result","status":"completed","conclusion":"failure","completed_at":"2026-09-03T16:43:28Z"}]}
]}`

func jobsResponse(body string) *http.Response {
	return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body)), Header: http.Header{}}
}

// expectFetchJobs stubs a single-page jobs response for the given run.
func expectFetchJobs(t *testing.T, client *rerun.MockRESTClient, run rerun.RunRef, body string) {
	t.Helper()
	path := "repos/" + run.Repo + "/actions/runs/" + run.RunID + "/attempts/" + run.RunAttempt + "/jobs?per_page=100"
	client.EXPECT().RequestWithContext(gomock.Any(), http.MethodGet, path, nil).Return(jobsResponse(body), nil) //nolint:bodyclose // closed by the code under test, not this fixture
}

func TestClassifyInfraFailures(t *testing.T) {
	t.Parallel()

	run := rerun.RunRef{Repo: "cloudposse/atmos", RunID: "1", RunAttempt: "1"}

	t.Run("zombie run reruns and records GITHUB_OUTPUT", func(t *testing.T) {
		t.Parallel()
		ctrl := gomock.NewController(t)
		client := rerun.NewMockRESTClient(ctrl)
		expectFetchJobs(t, client, run, zombieRunJobs)
		outputPath := filepath.Join(t.TempDir(), "output")
		var stdout, stderr bytes.Buffer

		require.NoError(t, classifyInfraFailures(context.Background(), client, &stdout, &stderr, &classifyConfig{run: run, outputPath: outputPath}))

		wantTable := "Build (windows)\tcancelled\trunner-stuck-after-complete\n" +
			"Acceptance Tests (windows)\tfailure\tcheck-cascade\n"
		assert.Equal(t, wantTable, stdout.String())
		assert.Equal(t, "verdict: rerun (all 2 non-success job(s) are infrastructure zombies or cascades of one)\n", stderr.String())

		output, err := os.ReadFile(outputPath)
		require.NoError(t, err)
		assert.Equal(t, "verdict=rerun\n"+
			"reason=all 2 non-success job(s) are infrastructure zombies or cascades of one\n"+
			"table<<"+githubOutputDelimiter+"\n"+wantTable+githubOutputDelimiter+"\n", string(output))
	})

	t.Run("appends to an existing GITHUB_OUTPUT", func(t *testing.T) {
		t.Parallel()
		ctrl := gomock.NewController(t)
		client := rerun.NewMockRESTClient(ctrl)
		expectFetchJobs(t, client, run, `{"jobs":[]}`)
		outputPath := filepath.Join(t.TempDir(), "output")
		require.NoError(t, os.WriteFile(outputPath, []byte("earlier=1\n"), 0o644))

		require.NoError(t, classifyInfraFailures(context.Background(), client, &bytes.Buffer{}, &bytes.Buffer{}, &classifyConfig{run: run, outputPath: outputPath}))

		output, err := os.ReadFile(outputPath)
		require.NoError(t, err)
		assert.Equal(t, "earlier=1\nverdict=no-jobs\nreason=no non-success jobs\ntable<<"+githubOutputDelimiter+"\n"+githubOutputDelimiter+"\n", string(output))
	})

	t.Run("without GITHUB_OUTPUT only stdout and stderr are written", func(t *testing.T) {
		t.Parallel()
		ctrl := gomock.NewController(t)
		client := rerun.NewMockRESTClient(ctrl)
		expectFetchJobs(t, client, run, zombieRunJobs)
		var stdout, stderr bytes.Buffer

		require.NoError(t, classifyInfraFailures(context.Background(), client, &stdout, &stderr, &classifyConfig{run: run}))

		assert.Contains(t, stdout.String(), "runner-stuck-after-complete")
		assert.Contains(t, stderr.String(), "verdict: rerun")
	})

	t.Run("stuck gap override changes the verdict", func(t *testing.T) {
		t.Parallel()
		ctrl := gomock.NewController(t)
		client := rerun.NewMockRESTClient(ctrl)
		expectFetchJobs(t, client, run, zombieRunJobs)
		var stdout, stderr bytes.Buffer

		// 34 minutes elapsed between the last step and the job completing.
		require.NoError(t, classifyInfraFailures(context.Background(), client, &stdout, &stderr, &classifyConfig{run: run, stuckGap: "1h"}))

		assert.Contains(t, stdout.String(), "Build (windows)\tcancelled\tsuperseded\n")
		assert.Equal(t, "verdict: no-rerun (no runner-stuck-after-complete or runner-lost jobs)\n", stderr.String())
	})

	t.Run("invalid stuck gap is rejected before any fetch", func(t *testing.T) {
		t.Parallel()
		ctrl := gomock.NewController(t)
		client := rerun.NewMockRESTClient(ctrl) // no EXPECT: must not be called
		for _, value := range []string{"soon", "-5s", "0"} {
			err := classifyInfraFailures(context.Background(), client, &bytes.Buffer{}, &bytes.Buffer{}, &classifyConfig{run: run, stuckGap: value})
			require.ErrorIs(t, err, errInvalidStuckGap, value)
		}
	})

	t.Run("fetch failure is an error", func(t *testing.T) {
		t.Parallel()
		ctrl := gomock.NewController(t)
		client := rerun.NewMockRESTClient(ctrl)
		client.EXPECT().RequestWithContext(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, errors.New("network unreachable"))

		err := classifyInfraFailures(context.Background(), client, &bytes.Buffer{}, &bytes.Buffer{}, &classifyConfig{run: run})
		require.Error(t, err)
	})

	t.Run("malformed jobs JSON is an error", func(t *testing.T) {
		t.Parallel()
		ctrl := gomock.NewController(t)
		client := rerun.NewMockRESTClient(ctrl)
		expectFetchJobs(t, client, run, `{"jobs":[`)

		err := classifyInfraFailures(context.Background(), client, &bytes.Buffer{}, &bytes.Buffer{}, &classifyConfig{run: run})
		require.Error(t, err)
	})

	t.Run("unwritable GITHUB_OUTPUT is an error", func(t *testing.T) {
		t.Parallel()
		ctrl := gomock.NewController(t)
		client := rerun.NewMockRESTClient(ctrl)
		expectFetchJobs(t, client, run, zombieRunJobs)

		err := classifyInfraFailures(context.Background(), client, &bytes.Buffer{}, &bytes.Buffer{}, &classifyConfig{
			run:        run,
			outputPath: filepath.Join(t.TempDir(), "no-such-dir", "output"),
		})
		require.Error(t, err)
	})

	t.Run("zombie run records GITHUB_STEP_SUMMARY", func(t *testing.T) {
		t.Parallel()
		summaryRun := rerun.RunRef{Repo: "cloudposse/atmos", RunID: "123", RunAttempt: "2"}
		ctrl := gomock.NewController(t)
		client := rerun.NewMockRESTClient(ctrl)
		expectFetchJobs(t, client, summaryRun, zombieRunJobs)
		summaryPath := filepath.Join(t.TempDir(), "summary")

		require.NoError(t, classifyInfraFailures(context.Background(), client, &bytes.Buffer{}, &bytes.Buffer{}, &classifyConfig{run: summaryRun, summaryPath: summaryPath}))

		summary, err := os.ReadFile(summaryPath)
		require.NoError(t, err)
		want := "### Infra-failure classification: run [123 attempt 2](https://github.com/cloudposse/atmos/actions/runs/123/attempts/2)\n\n" +
			"| Job | Conclusion | Class |\n" +
			"|---|---|---|\n" +
			"| Build (windows) | cancelled | runner-stuck-after-complete |\n" +
			"| Acceptance Tests (windows) | failure | check-cascade |\n" +
			"\nVerdict: **rerun** (all 2 non-success job(s) are infrastructure zombies or cascades of one)\n"
		assert.Equal(t, want, string(summary))
	})

	t.Run("appends to an existing GITHUB_STEP_SUMMARY", func(t *testing.T) {
		t.Parallel()
		ctrl := gomock.NewController(t)
		client := rerun.NewMockRESTClient(ctrl)
		expectFetchJobs(t, client, run, `{"jobs":[]}`)
		summaryPath := filepath.Join(t.TempDir(), "summary")
		require.NoError(t, os.WriteFile(summaryPath, []byte("earlier content\n"), 0o644))

		require.NoError(t, classifyInfraFailures(context.Background(), client, &bytes.Buffer{}, &bytes.Buffer{}, &classifyConfig{run: run, summaryPath: summaryPath}))

		summary, err := os.ReadFile(summaryPath)
		require.NoError(t, err)
		assert.True(t, strings.HasPrefix(string(summary), "earlier content\n### Infra-failure classification"), string(summary))
	})

	t.Run("without GITHUB_STEP_SUMMARY nothing extra is written", func(t *testing.T) {
		t.Parallel()
		ctrl := gomock.NewController(t)
		client := rerun.NewMockRESTClient(ctrl)
		expectFetchJobs(t, client, run, zombieRunJobs)

		require.NoError(t, classifyInfraFailures(context.Background(), client, &bytes.Buffer{}, &bytes.Buffer{}, &classifyConfig{run: run}))
	})

	t.Run("unwritable GITHUB_STEP_SUMMARY is an error", func(t *testing.T) {
		t.Parallel()
		ctrl := gomock.NewController(t)
		client := rerun.NewMockRESTClient(ctrl)
		expectFetchJobs(t, client, run, zombieRunJobs)

		err := classifyInfraFailures(context.Background(), client, &bytes.Buffer{}, &bytes.Buffer{}, &classifyConfig{
			run:         run,
			summaryPath: filepath.Join(t.TempDir(), "no-such-dir", "summary"),
		})
		require.Error(t, err)
	})
}

func TestStuckGapOptions(t *testing.T) {
	t.Parallel()

	opts, err := stuckGapOptions("")
	require.NoError(t, err)
	assert.Equal(t, rerun.Options{}, opts)

	opts, err = stuckGapOptions("300s")
	require.NoError(t, err)
	assert.Equal(t, rerun.DefaultStuckGap, opts.StuckGap)
}

func TestRerunInfraFailures(t *testing.T) {
	t.Parallel()

	baseParams := rerunParams{repo: "cloudposse/atmos", runID: "999", event: "push", headSHA: "deadbeef"}

	t.Run("push event skips the PR check and reruns", func(t *testing.T) {
		t.Parallel()
		ctrl := gomock.NewController(t)
		client := rerun.NewMockRESTClient(ctrl)
		client.EXPECT().RequestWithContext(gomock.Any(), http.MethodPost, "repos/cloudposse/atmos/actions/runs/999/rerun-failed-jobs", nil).
			Return(&http.Response{StatusCode: 201, Body: http.NoBody}, nil)
		var stdout bytes.Buffer

		require.NoError(t, rerunInfraFailures(context.Background(), client, &stdout, "", &baseParams))
		assert.Contains(t, stdout.String(), "Rerun requested for run 999.")
	})

	t.Run("pull_request with no PR numbers is not rerun", func(t *testing.T) {
		t.Parallel()
		ctrl := gomock.NewController(t)
		client := rerun.NewMockRESTClient(ctrl) // no EXPECT: must not call the API
		var stdout bytes.Buffer
		p := baseParams
		p.event = "pull_request"

		require.NoError(t, rerunInfraFailures(context.Background(), client, &stdout, "", &p))
		assert.Contains(t, stdout.String(), "no associated pull request")
	})

	t.Run("pull_request whose head still matches reruns", func(t *testing.T) {
		t.Parallel()
		ctrl := gomock.NewController(t)
		client := rerun.NewMockRESTClient(ctrl)
		client.EXPECT().RequestWithContext(gomock.Any(), http.MethodGet, "repos/cloudposse/atmos/pulls/42", nil).
			Return(&http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"head":{"sha":"deadbeef"}}`))}, nil)
		client.EXPECT().RequestWithContext(gomock.Any(), http.MethodPost, "repos/cloudposse/atmos/actions/runs/999/rerun-failed-jobs", nil).
			Return(&http.Response{StatusCode: 201, Body: http.NoBody}, nil)
		var stdout bytes.Buffer
		p := baseParams
		p.event = "pull_request"
		p.prNumbers = []string{"42"}

		require.NoError(t, rerunInfraFailures(context.Background(), client, &stdout, "", &p))
		assert.Contains(t, stdout.String(), "Rerun requested for run 999.")
	})

	t.Run("pull_request whose head moved on is superseded, not rerun", func(t *testing.T) {
		t.Parallel()
		ctrl := gomock.NewController(t)
		client := rerun.NewMockRESTClient(ctrl)
		client.EXPECT().RequestWithContext(gomock.Any(), http.MethodGet, "repos/cloudposse/atmos/pulls/42", nil).
			Return(&http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"head":{"sha":"newsha"}}`))}, nil)
		var stdout bytes.Buffer
		p := baseParams
		p.event = "pull_request"
		p.prNumbers = []string{"42"}

		require.NoError(t, rerunInfraFailures(context.Background(), client, &stdout, "", &p))
		assert.Contains(t, stdout.String(), "superseded, not rerunning")
	})

	t.Run("invalid PR number is an error", func(t *testing.T) {
		t.Parallel()
		ctrl := gomock.NewController(t)
		client := rerun.NewMockRESTClient(ctrl) // no EXPECT: must not call the API
		p := baseParams
		p.event = "pull_request"
		p.prNumbers = []string{"not-a-number"}

		err := rerunInfraFailures(context.Background(), client, &bytes.Buffer{}, "", &p)
		require.Error(t, err)
	})

	t.Run("PR head lookup failure is an error", func(t *testing.T) {
		t.Parallel()
		ctrl := gomock.NewController(t)
		client := rerun.NewMockRESTClient(ctrl)
		client.EXPECT().RequestWithContext(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, errors.New("not found"))
		p := baseParams
		p.event = "pull_request"
		p.prNumbers = []string{"42"}

		err := rerunInfraFailures(context.Background(), client, &bytes.Buffer{}, "", &p)
		require.Error(t, err)
	})

	t.Run("still-running run is skipped, not an error, and warns", func(t *testing.T) {
		t.Parallel()
		ctrl := gomock.NewController(t)
		client := rerun.NewMockRESTClient(ctrl)
		client.EXPECT().RequestWithContext(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
			Return(nil, &api.HTTPError{StatusCode: 403, Message: "Run is already running", RequestURL: &url.URL{}})
		var stdout bytes.Buffer

		require.NoError(t, rerunInfraFailures(context.Background(), client, &stdout, "", &baseParams))
		assert.Contains(t, stdout.String(), "::warning::")
		assert.Contains(t, stdout.String(), "Rerun skipped")
	})

	t.Run("a genuine rerun API failure is returned and annotated", func(t *testing.T) {
		t.Parallel()
		ctrl := gomock.NewController(t)
		client := rerun.NewMockRESTClient(ctrl)
		client.EXPECT().RequestWithContext(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
			Return(nil, &api.HTTPError{StatusCode: 404, Message: "Not Found", RequestURL: &url.URL{}})
		var stdout bytes.Buffer

		err := rerunInfraFailures(context.Background(), client, &stdout, "", &baseParams)
		require.Error(t, err)
		assert.Contains(t, stdout.String(), "::error::")
	})

	t.Run("appends the decision to GITHUB_STEP_SUMMARY", func(t *testing.T) {
		t.Parallel()
		ctrl := gomock.NewController(t)
		client := rerun.NewMockRESTClient(ctrl)
		client.EXPECT().RequestWithContext(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
			Return(&http.Response{StatusCode: 201, Body: http.NoBody}, nil)
		summaryPath := filepath.Join(t.TempDir(), "summary")

		require.NoError(t, rerunInfraFailures(context.Background(), client, &bytes.Buffer{}, summaryPath, &baseParams))

		summary, err := os.ReadFile(summaryPath)
		require.NoError(t, err)
		assert.Equal(t, "Rerun requested for run 999.\n", string(summary))
	})

	t.Run("unwritable GITHUB_STEP_SUMMARY is an error", func(t *testing.T) {
		t.Parallel()
		ctrl := gomock.NewController(t)
		client := rerun.NewMockRESTClient(ctrl)
		client.EXPECT().RequestWithContext(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
			Return(&http.Response{StatusCode: 201, Body: http.NoBody}, nil)

		err := rerunInfraFailures(context.Background(), client, &bytes.Buffer{}, filepath.Join(t.TempDir(), "no-such-dir", "summary"), &baseParams)
		require.Error(t, err)
	})
}
