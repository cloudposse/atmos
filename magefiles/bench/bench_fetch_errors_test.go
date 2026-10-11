//go:build mage

package bench

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cli/go-gh/v2/pkg/api"
	"github.com/cloudposse/atmos/magefiles/internal/githubapi"
	"github.com/cloudposse/atmos/magefiles/internal/githubtest"
	"github.com/stretchr/testify/require"
)

func TestBenchDownloadBoundsAndReadErrors(t *testing.T) {
	for _, scenario := range []string{"request error", "body error", "oversized"} {
		t.Run(scenario, func(t *testing.T) {
			fake := &githubtest.FakeAPI{T: t, Download: func(_ string) (*http.Response, error) {
				if scenario == "request error" {
					return nil, errors.New("download failed")
				}
				var body io.ReadCloser
				if scenario == "body error" {
					reader, writer := io.Pipe()
					require.NoError(t, writer.CloseWithError(errors.New("body failed")))
					body = reader
				} else {
					body = io.NopCloser(bytes.NewReader(bytes.Repeat([]byte("x"), benchArchiveLimit+1)))
				}
				return &http.Response{Body: body}, nil
			}}
			dir := t.TempDir()
			require.Error(t, benchDownload(context.Background(), fake, githubtest.Repo, benchArtifact{ID: 1}, dir))
			entries, err := os.ReadDir(dir)
			require.NoError(t, err)
			require.Empty(t, entries)
		})
	}
}

func TestBenchFetchFailuresAndExpirationRace(t *testing.T) {
	for _, scenario := range []string{"runs", "ancestor", "artifacts", "download", "expired", "malformed", "provenance", "incompatible"} {
		t.Run(scenario, func(t *testing.T) {
			run := githubtest.Run()
			candidate := benchTestSnapshot(100)
			candidate.Manifest.Started = run.Updated
			if scenario == "incompatible" {
				candidate.Manifest.CPU = "another CPU"
			}
			archive := benchSnapshotArchive(t, run)
			if scenario == "malformed" {
				archive = benchZip(t, map[string][]byte{"manifest.json": []byte("{}"), "bench.txt": []byte("bad")})
			}
			if scenario == "provenance" {
				other := run
				other.Attempt++
				archive = benchSnapshotArchive(t, other)
			}
			fake := &githubtest.FakeAPI{T: t, Archive: archive}
			fake.Get = func(path string, response any) error {
				switch {
				case strings.Contains(path, "/workflows/"):
					if scenario == "runs" {
						return errors.New("runs failed")
					}
					return githubtest.Respond(map[string]any{"workflow_runs": []githubapi.Run{run}}, response)
				case strings.Contains(path, "/compare/"):
					if scenario == "ancestor" {
						return errors.New("ancestor failed")
					}
					return githubtest.Respond(map[string]string{"status": "identical"}, response)
				default:
					if scenario == "artifacts" {
						return errors.New("artifacts failed")
					}
					return githubtest.Respond(map[string]any{"artifacts": []benchArtifact{{ID: 1, Name: benchArtifactName(&run)}}}, response)
				}
			}
			if scenario == "download" || scenario == "expired" {
				fake.Download = func(_ string) (*http.Response, error) {
					if scenario == "expired" {
						return nil, &api.HTTPError{StatusCode: 410}
					}
					return nil, errors.New("download failed")
				}
			}
			dir := t.TempDir()
			err := benchFetch(context.Background(), fake, githubtest.Repo, githubtest.Base, dir, candidate)
			if scenario == "expired" || scenario == "incompatible" {
				require.NoError(t, err)
				data, err := os.ReadFile(filepath.Join(dir, "unavailable.json"))
				require.NoError(t, err)
				require.Contains(t, string(data), scenario)
			} else {
				require.Error(t, err)
			}
			entries, err := os.ReadDir(dir)
			require.NoError(t, err)
			for _, entry := range entries {
				require.False(t, entry.IsDir(), "failed downloads must be cleaned up")
			}
		})
	}
}
