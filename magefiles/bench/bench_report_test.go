//go:build mage

package bench

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cloudposse/atmos/magefiles/internal/githubapi"
	"github.com/cloudposse/atmos/magefiles/internal/githubtest"
	"github.com/stretchr/testify/require"
)

func TestBenchReportRunValidationAndPublication(t *testing.T) {
	for _, scenario := range []string{"success", "failure", "stale", "invalid run", "missing artifact", "malformed", "wrong provenance", "base not on main", "API error", "publish error"} {
		t.Run(scenario, func(t *testing.T) {
			benchSetUpRun(t)
			run := githubtest.Run()
			run.Event = "pull_request"
			pr := githubtest.PR(run)
			if scenario == "failure" {
				run.Conclusion = "failure"
			}
			if scenario == "stale" {
				pr.Head.SHA = githubtest.Base
			}
			if scenario == "invalid run" {
				run.Event = "push"
			}
			archive := benchSnapshotArchive(t, run)
			if scenario == "malformed" {
				archive = []byte("not a zip")
			}
			if scenario == "wrong provenance" {
				other := run
				other.Attempt++
				archive = benchSnapshotArchive(t, other)
			}
			fake := &githubtest.FakeAPI{T: t, Archive: archive}
			fake.Get = func(path string, response any) error {
				switch {
				case strings.HasSuffix(path, "actions/runs/10"):
					if scenario == "API error" {
						return errors.New("unavailable")
					}
					return githubtest.Respond(run, response)
				case strings.Contains(path, "/pulls?"):
					return githubtest.Respond([]githubapi.PR{pr}, response)
				case strings.Contains(path, "/artifacts?"):
					artifacts := []benchArtifact{{ID: 1, Name: benchArtifactName(&run)}}
					if scenario == "missing artifact" {
						artifacts = nil
					}
					return githubtest.Respond(map[string]any{"artifacts": artifacts}, response)
				case strings.Contains(path, "/compare/"):
					status := "identical"
					if scenario == "base not on main" {
						status = "diverged"
					}
					return githubtest.Respond(map[string]string{"status": status}, response)
				case strings.Contains(path, "/workflows/"):
					return githubtest.Respond(map[string]any{"workflow_runs": []githubapi.Run{}}, response)
				default:
					t.Fatalf("unexpected path %s", path)
					return nil
				}
			}
			published := 0
			err := ReportRun(context.Background(), fake, githubtest.Repo, 10, func(number int, body string, reported *githubapi.Run) error {
				published++
				require.Equal(t, 42, number)
				require.Equal(t, run.ID, reported.ID)
				require.Contains(t, body, "Comparison unavailable")
				require.Contains(t, body, "<!-- atmos-benchmark-run:10:2 -->")
				if scenario == "failure" {
					require.Contains(t, body, "execution failure")
				}
				if scenario == "publish error" {
					return errors.New("write failed")
				}
				return nil
			})
			switch scenario {
			case "success", "failure":
				require.NoError(t, err)
				require.Equal(t, 1, published)
			case "stale":
				require.NoError(t, err)
				require.Zero(t, published)
			default:
				require.Error(t, err)
			}
		})
	}
}

func TestBenchPRAndArtifactPaginationErrors(t *testing.T) {
	run := githubtest.Run()
	fake := &githubtest.FakeAPI{T: t, Get: func(_ string, _ any) error { return errors.New("API unavailable") }}
	_, err := benchAssociatedPRs(context.Background(), fake, githubtest.Repo, &run)
	require.ErrorContains(t, err, "API unavailable")
	_, err = benchFindArtifact(context.Background(), fake, githubtest.Repo, &run)
	require.ErrorContains(t, err, "API unavailable")
	fake.Get = func(_ string, response any) error { return githubtest.Respond(make([]githubapi.PR, 100), response) }
	_, err = benchAssociatedPRs(context.Background(), fake, githubtest.Repo, &run)
	require.ErrorContains(t, err, "pagination limit")
	fake.Get = func(_ string, response any) error {
		return githubtest.Respond(map[string]any{"artifacts": make([]benchArtifact, 100)}, response)
	}
	_, err = benchFindArtifact(context.Background(), fake, githubtest.Repo, &run)
	require.ErrorContains(t, err, "pagination limit")
}

func TestBenchArchiveRejectsBeforeWriting(t *testing.T) {
	for _, scenario := range []string{"traversal", "duplicate", "symlink", "oversized", "malformed", "missing"} {
		t.Run(scenario, func(t *testing.T) {
			var raw bytes.Buffer
			writer := zip.NewWriter(&raw)
			first, err := writer.Create("manifest.json")
			require.NoError(t, err)
			_, err = first.Write([]byte("{}"))
			require.NoError(t, err)
			name := "bench.txt"
			if scenario == "traversal" {
				name = "../outside"
			}
			if scenario == "duplicate" {
				name = "manifest.json"
			}
			if scenario != "missing" {
				header := &zip.FileHeader{Name: name, Method: zip.Deflate}
				if scenario == "symlink" {
					header.SetMode(os.ModeSymlink | 0o777)
				}
				entry, err := writer.CreateHeader(header)
				require.NoError(t, err)
				contents := []byte("data")
				if scenario == "oversized" {
					contents = bytes.Repeat([]byte("x"), benchArchiveLimit/2+1)
				}
				_, err = entry.Write(contents)
				require.NoError(t, err)
			}
			require.NoError(t, writer.Close())
			data := raw.Bytes()
			if scenario == "malformed" {
				data = data[:10]
			}
			dir := t.TempDir()
			require.Error(t, benchExtract(data, dir))
			entries, err := os.ReadDir(dir)
			require.NoError(t, err)
			require.Empty(t, entries)
		})
	}
	dir := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(dir, "manifest.json"), 0o700))
	require.Error(t, benchExtract(benchSnapshotArchive(t, githubtest.Run()), dir))
}

func TestBenchFetchRejectsInvalidDestinationsAndInputs(t *testing.T) {
	candidate := benchTestSnapshot(100)
	for _, repo := range []string{"invalid", githubtest.Repo} {
		err := benchFetch(context.Background(), nil, repo, "bad sha", t.TempDir(), candidate)
		require.ErrorContains(t, err, "immutable")
	}
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "existing"), nil, 0o600))
	require.ErrorContains(t, benchFetch(context.Background(), nil, githubtest.Repo, githubtest.Base, dir, candidate), "not empty")
	require.Error(t, benchFetch(context.Background(), nil, githubtest.Repo, githubtest.Base, filepath.Join(dir, "existing"), candidate))
	require.Error(t, (Bench{}).Fetch(githubtest.Repo, githubtest.Base, filepath.Join(dir, "snapshots")))
}

func TestBenchFetchHistorySearchBound(t *testing.T) {
	candidate := benchTestSnapshot(100)
	fake := &githubtest.FakeAPI{T: t, Get: func(_ string, response any) error {
		return githubtest.Respond(map[string]any{"workflow_runs": make([]githubapi.Run, 100)}, response)
	}}
	dir := t.TempDir()
	require.NoError(t, benchFetch(context.Background(), fake, githubtest.Repo, githubtest.Base, dir, candidate))
	data, err := os.ReadFile(filepath.Join(dir, "unavailable.json"))
	require.NoError(t, err)
	require.Contains(t, string(data), "1,000 runs")
}
