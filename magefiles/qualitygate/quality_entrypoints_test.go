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

func TestQualityReportEntrypoint(t *testing.T) {
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
			err := (Quality{}).Report(githubtest.Repo)
			if scenario == "no PRs" || scenario == "closed PR" {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}
		})
	}
}

func TestBenchmarkReportEntrypointRejectsMainRun(t *testing.T) {
	githubtest.Server(t, func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/repos/cloudposse/atmos/actions/runs/10", r.URL.Path)
		githubtest.JSON(t, w, githubtest.Run())
	})
	require.ErrorContains(t, (Bench{}).Report(githubtest.Repo, "10"), "not a completed PR benchmark run")
}
