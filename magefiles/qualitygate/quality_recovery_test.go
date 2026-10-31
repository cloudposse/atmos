//go:build mage

package qualitygate

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/cloudposse/atmos/magefiles/internal/githubapi"
	"github.com/cloudposse/atmos/magefiles/internal/githubtest"
	"github.com/stretchr/testify/require"
)

func TestQualityReconcileContinuesAfterPRFailure(t *testing.T) {
	t.Setenv("GITHUB_EVENT_NAME", "workflow_dispatch")
	t.Setenv("GITHUB_EVENT_PATH", "")
	cases := []struct {
		name         string
		secondFails  bool
		wantErr      []string
		wantInspects []string
	}{
		// One failing PR is logged and the later PR is still reconciled; the run
		// fails only when every PR failed.
		{name: "later PR succeeds", wantInspects: []string{"42", "43"}},
		{name: "every PR fails", secondFails: true, wantErr: []string{"PR #42", "PR #43"}, wantInspects: []string{"42", "43"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var inspected []string
			githubtest.Server(t, func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/repos/cloudposse/atmos/pulls":
					first := githubtest.PR(githubtest.Run())
					second := first
					second.Number = 43
					githubtest.JSON(t, w, []githubapi.PR{first, second})
				case "/repos/cloudposse/atmos/pulls/42":
					inspected = append(inspected, "42")
					w.WriteHeader(http.StatusInternalServerError)
				case "/repos/cloudposse/atmos/pulls/43":
					inspected = append(inspected, "43")
					if tc.secondFails {
						w.WriteHeader(http.StatusInternalServerError)
						return
					}
					pr := githubtest.PR(githubtest.Run())
					pr.State = "closed"
					githubtest.JSON(t, w, pr)
				default:
					t.Errorf("unexpected request: %s", r.URL.Path)
					w.WriteHeader(http.StatusInternalServerError)
				}
			})
			err := (Quality{}).Reconcile(githubtest.Repo)
			if len(tc.wantErr) == 0 {
				require.NoError(t, err)
			}
			for _, want := range tc.wantErr {
				require.ErrorContains(t, err, want)
			}
			require.Equal(t, tc.wantInspects, inspected)
		})
	}
}

func TestQualityBenchmarkReportErrorStillRefreshes(t *testing.T) {
	for _, scenario := range []string{"missing artifact", "malformed artifact", "newer attempt", "stale head", "refresh failure"} {
		t.Run(scenario, func(t *testing.T) {
			run := githubtest.Run()
			run.Event = "pull_request"
			pr := githubtest.PR(run)
			count := 20
			pr.ChangedFiles = &count
			reads := 0
			getter := qualityPublishGetter(t, scenario, &pr, &run, &reads)
			fake := &githubtest.FakeAPI{T: t, Archive: []byte("malformed zip")}
			fake.Get = func(path string, response any) error {
				switch {
				case strings.Contains(path, "actions/workflows/"):
					return githubtest.Respond(map[string]any{"workflow_runs": []githubapi.Run{run}}, response)
				case strings.Contains(path, "/commits/") && strings.Contains(path, "/pulls?"):
					return githubtest.Respond([]githubapi.PR{pr}, response)
				case strings.Contains(path, "/artifacts?"):
					artifacts := []map[string]any{}
					if scenario == "malformed artifact" {
						artifacts = append(artifacts, map[string]any{"id": 1, "name": "benchmarks-10-2"})
					}
					return githubtest.Respond(map[string]any{"artifacts": artifacts}, response)
				default:
					return getter(path, response)
				}
			}
			var published string
			var labels []string
			writeErr := errors.New("publish unavailable")
			fake.Write = func(method, path string, body io.Reader) error {
				if scenario == "refresh failure" {
					return writeErr
				}
				if strings.Contains(path, "comments") {
					var payload map[string]string
					require.NoError(t, json.NewDecoder(body).Decode(&payload))
					published = payload["body"]
				}
				if method == http.MethodPost && strings.HasSuffix(path, "/labels") {
					var payload struct {
						Labels []string `json:"labels"`
					}
					require.NoError(t, json.NewDecoder(body).Decode(&payload))
					labels = payload.Labels
				}
				return nil
			}
			q := qualityClient{ctx: context.Background(), api: &qualityGraphAPI{FakeAPI: fake, t: t, scenario: "valid"}, repo: githubtest.Repo}
			err := q.reconcile(42)
			require.Error(t, err, "malformed/missing results must still fail the reporter job")
			if scenario == "refresh failure" {
				require.ErrorIs(t, err, writeErr)
				require.ErrorContains(t, err, "benchmark report:")
				return
			}
			if scenario == "newer attempt" || scenario == "stale head" {
				require.Empty(t, published)
				require.Empty(t, labels)
				return
			}
			require.Contains(t, published, "Quality Gate not ready")
			require.Contains(t, published, "Comparison unavailable")
			require.Contains(t, published, "CodeRabbit approval (current head)")
			require.Contains(t, published, "Inspect the benchmark reporter")
			require.Equal(t, []string{qualityNotReady}, labels)
			require.False(t, qualityReported(&githubapi.Comment{Body: published}, &pr, &run), "a failed report must remain retryable")
		})
	}
}

// A benchmark run that ReportRun cannot associate with any PR must not leave the
// gate unpublished: recovery falls back to a plain refresh and warns.
func TestQualityBenchmarkRecoveryPublishesWhenNoPRAssociated(t *testing.T) {
	cases := []struct {
		name          string
		headLookup    func(pr githubapi.PR) []githubapi.PR
		wantWarning   bool
		wantPublished int
	}{
		{
			name:          "fork PR with no association falls back to a refresh",
			headLookup:    func(githubapi.PR) []githubapi.PR { return []githubapi.PR{} },
			wantWarning:   true,
			wantPublished: 1,
		},
		{
			// Negative path: when ReportRun resolves the fork PR through its head
			// branch it publishes itself, so the fallback must not run twice.
			name:          "fork PR resolved through its head branch publishes once without warning",
			headLookup:    func(pr githubapi.PR) []githubapi.PR { return []githubapi.PR{pr} },
			wantPublished: 1,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			run := githubtest.Run()
			run.Event, run.Branch = "pull_request", "fix/example-branch"
			run.HeadRepository.FullName = "contributor/atmos"
			run.Conclusion = "failure"
			pr := githubtest.PR(run)
			count, reads := 20, 0
			pr.ChangedFiles = &count
			getter := qualityPublishGetter(t, "create", &pr, &run, &reads)
			fake := &githubtest.FakeAPI{T: t}
			fake.Get = func(path string, response any) error {
				switch {
				case strings.Contains(path, "actions/workflows/"):
					return githubtest.Respond(map[string]any{"workflow_runs": []githubapi.Run{run}}, response)
				case strings.Contains(path, "/commits/"+run.HeadSHA+"/pulls?"):
					return githubtest.Respond([]githubapi.PR{}, response)
				case strings.Contains(path, "/pulls?") && strings.Contains(path, "head="):
					return githubtest.Respond(tc.headLookup(pr), response)
				default:
					return getter(path, response)
				}
			}
			published := 0
			fake.Write = func(method, path string, _ io.Reader) error {
				if method == http.MethodPost && strings.HasSuffix(path, "/issues/42/comments") {
					published++
				}
				return nil
			}
			q := qualityClient{ctx: context.Background(), api: &qualityGraphAPI{FakeAPI: fake, t: t, scenario: "valid"}, repo: githubtest.Repo}
			output := captureStdout(t, func() { require.NoError(t, q.reconcile(42)) })
			require.Equal(t, tc.wantPublished, published)
			require.Equal(t, tc.wantWarning, strings.Contains(output, "::warning title=Quality gate benchmark recovery::"), output)
		})
	}
}

func captureStdout(t *testing.T, run func()) string {
	t.Helper()
	reader, writer, err := os.Pipe()
	require.NoError(t, err)
	original := os.Stdout
	os.Stdout = writer
	defer func() { os.Stdout = original }()
	done := make(chan string)
	go func() {
		data, _ := io.ReadAll(reader)
		done <- string(data)
	}()
	run()
	require.NoError(t, writer.Close())
	return <-done
}

func TestQualityReportFailurePersistsUntilRecovery(t *testing.T) {
	run := githubtest.Run()
	run.Event = "pull_request"
	pr := githubtest.PR(run)
	count, reads := 20, 0
	pr.ChangedFiles = &count
	fake := &githubtest.FakeAPI{T: t, Get: qualityPublishGetter(t, "create", &pr, &run, &reads)}
	q := qualityClient{ctx: context.Background(), api: &qualityGraphAPI{FakeAPI: fake, t: t, scenario: "valid"}, repo: githubtest.Repo}
	failed := q.render(&pr, nil, qualityBenchmarkError+"\nComparison unavailable", &run)
	require.False(t, failed.ready)
	existing := &githubapi.Comment{Body: failed.body}
	retained := q.render(&pr, existing, "", nil)
	require.False(t, retained.ready, "ordinary check refresh must preserve the reporting failure")
	require.Contains(t, retained.body, qualityBenchmarkError)
	require.False(t, qualityReported(existing, &pr, &run))
	older := run
	older.Attempt--
	require.True(t, qualityReported(existing, &pr, &older), "failed newer reports still protect against older attempts")

	recovered := q.render(&pr, existing, "**warning**: performance regression", &run)
	require.True(t, recovered.ready, "a valid report clears the integrity failure; timing warnings stay advisory")
	require.NotContains(t, recovered.body, qualityBenchmarkError)
	require.True(t, qualityReported(&githubapi.Comment{Body: recovered.body}, &pr, &run))
	missing := q.render(&pr, nil, "Comparison unavailable: no historical baselines", nil)
	require.True(t, missing.ready, "missing history must not block readiness")
}
