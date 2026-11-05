//go:build mage

package bench

import (
	"net/http"
	"path/filepath"
	"testing"
	"time"

	"github.com/cloudposse/atmos/magefiles/internal/githubtest"
	"github.com/stretchr/testify/require"
)

func TestBenchFetchEntrypointFreezesRunCutoff(t *testing.T) {
	for _, scenario := range []string{"local", "CI", "PR merge", "invalid merge", "invalid provenance", "API failure"} {
		t.Run(scenario, func(t *testing.T) {
			dir := t.TempDir()
			candidate := benchTestSnapshot(100)
			run := githubtest.Run()
			if scenario == "PR merge" || scenario == "invalid merge" {
				run.Event = "pull_request"
				candidate.Manifest.SHA, candidate.Manifest.PRHeadSHA, candidate.Manifest.BaseSHA = benchTestMerge, run.HeadSHA, githubtest.Base
			}
			if scenario != "local" {
				candidate.Manifest.Repository = githubtest.Repo
				candidate.Manifest.RunID = "10"
				candidate.Manifest.Attempt = "2"
				candidate.Manifest.Event = run.Event
				if scenario == "invalid provenance" {
					candidate.Manifest.Attempt = "1"
				}
			}
			githubtest.Server(t, func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/repos/cloudposse/atmos/actions/runs/10":
					if scenario == "API failure" {
						w.WriteHeader(500)
						return
					}
					githubtest.JSON(t, w, run)
				case "/repos/cloudposse/atmos/git/commits/" + benchTestMerge:
					head := run.HeadSHA
					if scenario == "invalid merge" {
						head = githubtest.Base
					}
					githubtest.JSON(t, w, map[string]any{"sha": benchTestMerge, "parents": []map[string]string{{"sha": githubtest.Base}, {"sha": head}}})
				case "/repos/cloudposse/atmos/actions/workflows/benchmarks.yml/runs":
					cutoff := candidate.Manifest.Started
					if scenario != "local" {
						cutoff = run.Created
					}
					require.Equal(t, "<="+cutoff.Format(time.RFC3339), r.URL.Query().Get("created"))
					githubtest.JSON(t, w, map[string]any{"workflow_runs": []any{}})
				default:
					t.Errorf("unexpected request %s", r.URL)
					w.WriteHeader(500)
				}
			})
			candidateDir := filepath.Join(dir, "candidate")
			benchSaveTestSnapshot(t, candidateDir, candidate)
			t.Setenv("ATMOS_BENCH_CANDIDATE_DIR", candidateDir)
			err := (Bench{}).Fetch(githubtest.Repo, githubtest.Base, filepath.Join(dir, "snapshots"))
			if scenario == "local" || scenario == "CI" || scenario == "PR merge" {
				require.NoError(t, err)
				require.FileExists(t, filepath.Join(dir, "snapshots", "unavailable.json"))
			} else {
				require.Error(t, err)
			}
		})
	}
}
