//go:build mage

package release

import (
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/magefiles/internal/githubtest"
	"github.com/cloudposse/atmos/pkg/data"
	"github.com/cloudposse/atmos/pkg/ui"
)

func captureReleaseList(t *testing.T, repo, version string, closedOutput bool) (string, error) {
	t.Helper()
	t.Setenv("NO_COLOR", "1")
	file, err := os.Create(filepath.Join(t.TempDir(), "release-list.txt"))
	require.NoError(t, err)
	previous := os.Stdout
	os.Stdout = file
	t.Cleanup(func() {
		os.Stdout = previous
		_ = file.Close()
		data.Reset()
		ui.Reset()
	})
	if closedOutput {
		require.NoError(t, file.Close())
	}
	listErr := (Release{}).List(repo, version)
	content, err := os.ReadFile(file.Name())
	require.NoError(t, err)
	return string(content), listErr
}

func TestReleaseListCommandOutput(t *testing.T) {
	for _, mode := range []string{"next", "closed", "empty"} {
		t.Run(mode, func(t *testing.T) {
			version, state := "", "open"
			if mode == "closed" {
				version, state = "v1.233.2", "closed"
			}
			githubtest.Server(t, func(w http.ResponseWriter, r *http.Request) {
				require.Equal(t, http.MethodGet, r.Method)
				switch r.URL.Path {
				case planPrefix + "milestones":
					githubtest.JSON(t, w, []Milestone{{Number: 13, Title: "1.233.2", State: state}})
				case planPrefix + "issues":
					require.Equal(t, "13", r.URL.Query().Get("milestone"))
					items := []map[string]any{}
					if mode != "empty" {
						items = append(items, map[string]any{"number": 42, "pull_request": map[string]any{}})
					}
					githubtest.JSON(t, w, items)
				case planPrefix + "pulls/42":
					githubtest.JSON(t, w, listedReleasePR{Number: 42, Title: "Included fix", Merged: true, State: "closed"})
				default:
					t.Errorf("list must not check eligibility: %s", r.URL)
					http.NotFound(w, r)
				}
			})
			output, err := captureReleaseList(t, githubtest.Repo, version, false)
			require.NoError(t, err)
			require.Contains(t, output, "Release 1.233.2\n")
			require.NotContains(t, output, "\x1b")
			if mode == "empty" {
				require.Contains(t, output, "No pull requests assigned to this milestone.")
			} else {
				require.Contains(t, output, "●  #42")
				require.Contains(t, output, "Included fix")
				require.Contains(t, output, "merged")
			}
		})
	}
}

func TestReleaseListCommandFailures(t *testing.T) {
	for _, mode := range []string{"invalid repository", "missing milestone", "API unavailable", "closed output"} {
		t.Run(mode, func(t *testing.T) {
			githubtest.Server(t, func(w http.ResponseWriter, r *http.Request) {
				require.NotEqual(t, "invalid repository", mode, "invalid repositories must fail before making requests")
				require.Equal(t, http.MethodGet, r.Method)
				if mode == "API unavailable" {
					http.Error(w, "unavailable", http.StatusServiceUnavailable)
					return
				}
				if r.URL.Path == planPrefix+"milestones" {
					githubtest.JSON(t, w, []Milestone{{Number: 13, Title: "1.233.2", State: "open"}})
					return
				}
				require.Equal(t, planPrefix+"issues", r.URL.Path)
				githubtest.JSON(t, w, []any{})
			})
			repo, version := githubtest.Repo, ""
			switch mode {
			case "invalid repository":
				repo = "invalid"
			case "missing milestone":
				version = "1.233.3"
			}
			output, err := captureReleaseList(t, repo, version, mode == "closed output")
			require.Error(t, err)
			require.Empty(t, output, "failed listing must not print a successful or partial result")
			switch mode {
			case "closed output":
				require.ErrorIs(t, err, os.ErrClosed)
			case "API unavailable":
				require.ErrorContains(t, err, "503")
			default:
				require.ErrorIs(t, err, ErrMilestonePolicy)
			}
		})
	}
}
