//go:build mage

package qualitygate

import (
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/cloudposse/atmos/magefiles/internal/githubtest"
	"github.com/stretchr/testify/require"
)

func TestQualityReconcileEntrypoint(t *testing.T) {
	t.Setenv("GITHUB_EVENT_NAME", "workflow_dispatch")
	for _, scenario := range []string{"no PRs", "closed PR", "API failure", "invalid event", "reconcile failure"} {
		t.Run(scenario, func(t *testing.T) {
			t.Setenv("GITHUB_EVENT_PATH", "")
			if scenario == "invalid event" {
				path := filepath.Join(t.TempDir(), "event.json")
				require.NoError(t, os.WriteFile(path, []byte("{"), 0o600))
				t.Setenv("GITHUB_EVENT_PATH", path)
			}
			githubtest.Server(t, func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/repos/cloudposse/atmos/pulls":
					switch scenario {
					case "no PRs":
						githubtest.JSON(t, w, []any{})
					case "API failure":
						w.WriteHeader(500)
					default:
						githubtest.JSON(t, w, []any{githubtest.PR(githubtest.Run())})
					}
				case "/repos/cloudposse/atmos/pulls/42":
					if scenario == "reconcile failure" {
						w.WriteHeader(500)
						return
					}
					pr := githubtest.PR(githubtest.Run())
					pr.State = "closed"
					githubtest.JSON(t, w, pr)
				default:
					t.Errorf("unexpected request %s", r.URL)
					w.WriteHeader(500)
				}
			})
			err := (Quality{}).Reconcile(githubtest.Repo)
			if scenario == "no PRs" || scenario == "closed PR" {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}
		})
	}
}

// Each entry point owns its events: a per-PR report skips repository-wide
// events, and the true-up refuses events that name a single PR.
func TestQualityEntrypointScopes(t *testing.T) {
	cases := []struct {
		name, event, payload string
		repository           bool
	}{
		{"schedule", "schedule", `{}`, true},
		{"manual", "workflow_dispatch", `{}`, true},
		{"milestone", "milestone", `{"action":"created"}`, true},
		{"release", "release", `{"action":"published"}`, true},
		{"release removed", "release", `{"action":"unpublished"}`, true},
		{"merged PR", "pull_request_target", `{"action":"closed","number":42}`, true},
		{"label", "pull_request_target", `{"action":"labeled","number":42}`, false},
		{"PR milestone", "pull_request_target", `{"action":"milestoned","number":42}`, false},
		{"CI completion", "workflow_run", `{"workflow_run":{"head_sha":"` + githubtest.SHA + `"}}`, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "event.json")
			require.NoError(t, os.WriteFile(path, []byte(tc.payload), 0o600))
			t.Setenv("GITHUB_EVENT_NAME", tc.event)
			t.Setenv("GITHUB_EVENT_PATH", path)
			require.Equal(t, tc.repository, qualityRepositoryEvent())
			githubtest.Server(t, func(w http.ResponseWriter, r *http.Request) {
				t.Errorf("out-of-scope entry point must not call GitHub: %s", r.URL)
				w.WriteHeader(500)
			})
			if tc.repository {
				require.NoError(t, (Quality{}).Report(githubtest.Repo), "a per-PR report skips repository-wide events")
			} else {
				require.ErrorContains(t, (Quality{}).Reconcile(githubtest.Repo), "repository-wide events only")
			}
		})
	}
}

// A per-PR report reconciles exactly the PR its event names.
func TestQualityReportProcessesEventPR(t *testing.T) {
	path := filepath.Join(t.TempDir(), "event.json")
	require.NoError(t, os.WriteFile(path, []byte(`{"action":"labeled","number":42}`), 0o600))
	t.Setenv("GITHUB_EVENT_NAME", "pull_request_target")
	t.Setenv("GITHUB_EVENT_PATH", path)
	var requested []string
	githubtest.Server(t, func(w http.ResponseWriter, r *http.Request) {
		requested = append(requested, r.URL.Path)
		pr := githubtest.PR(githubtest.Run())
		pr.State = "closed"
		githubtest.JSON(t, w, pr)
	})
	require.NoError(t, (Quality{}).Report(githubtest.Repo))
	require.Equal(t, []string{"/repos/cloudposse/atmos/pulls/42"}, requested)
}

func TestBenchmarkReportEntrypointRejectsMainRun(t *testing.T) {
	githubtest.Server(t, func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/repos/cloudposse/atmos/actions/runs/10", r.URL.Path)
		githubtest.JSON(t, w, githubtest.Run())
	})
	require.ErrorContains(t, (Bench{}).Report(githubtest.Repo, "10"), "not a completed PR benchmark run")
}
