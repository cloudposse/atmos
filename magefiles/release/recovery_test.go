//go:build mage

package release

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/cloudposse/atmos/magefiles/internal/githubapi"
	"github.com/cloudposse/atmos/magefiles/internal/githubtest"
	"github.com/stretchr/testify/require"
)

func recoveryArchive(t *testing.T, name string, value any) []byte {
	t.Helper()
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	file, err := writer.Create(name)
	require.NoError(t, err)
	require.NoError(t, json.NewEncoder(file).Encode(value))
	require.NoError(t, writer.Close())
	return buffer.Bytes()
}

type recoveryFixture struct {
	run       recoveryRun
	plan      preparedRelease
	draft     githubRelease
	artifacts []recoveryArtifact
	writes    int
	published bool
	noDraft   bool
	duplicate bool
	// duplicateRelease lists the reserved tag's release twice.
	duplicateRelease bool
	lostReply        bool
	forbidden        bool
}

func newRecoveryFixture() *recoveryFixture {
	plan := preparedRelease{Plan: *approvedCutFixture(), Tag: "v1.234.0", RunID: "77"}
	run := recoveryRun{Run: githubapi.Run{ID: 77, Attempt: 1, Event: "workflow_dispatch", Branch: "main", Path: ".github/workflows/publish-release.yml", Status: "completed", Conclusion: "failure"}, Title: "Atmos release v1.234.0 [request]"}
	run.HeadRepository.FullName = githubtest.Repo
	return &recoveryFixture{
		run: run, plan: plan,
		draft:     githubRelease{ID: 123, Tag: plan.Tag, Commit: plan.Commit, Draft: true},
		artifacts: []recoveryArtifact{{ID: 900, Name: "release-plan-77-1"}},
	}
}

// release returns the fixture's release as GitHub reports it before or after publication.
func (f *recoveryFixture) release() githubRelease {
	release := f.draft
	if f.published {
		release.Draft, release.Immutable = false, true
	}
	return release
}

func (f *recoveryFixture) client(t *testing.T) *milestoneClient {
	t.Helper()
	routes := planRoutes()
	return milestoneHTTP(t, func(w http.ResponseWriter, r *http.Request) {
		path := planPath(r.URL.Path)
		switch {
		case r.URL.Path == "/user":
			id := ostermanGitHubUserID
			if f.forbidden {
				id = 1
			}
			githubtest.JSON(t, w, map[string]any{"id": id, "type": "User"})
		case path == "git/ref/tags/v1.234.0":
			githubtest.JSON(t, w, tagObject("commit", githubtest.SHA))
		case path == "releases/tags/v1.234.0":
			// GitHub's tag endpoint returns published releases only; drafts are listed.
			if f.noDraft || !f.published {
				http.Error(w, "not found", http.StatusNotFound)
				return
			}
			githubtest.JSON(t, w, f.release())
		case path == "releases":
			// The shared baseline stays listed beside the reserved tag's release.
			releases := []any{stableRelease("v1.233.0")}
			if !f.noDraft {
				releases = append(releases, f.release())
				if f.duplicateRelease {
					releases = append(releases, f.release())
				}
			}
			githubtest.JSON(t, w, releases)
		case path == "actions/workflows/publish-release.yml/runs":
			runs := []recoveryRun{f.run}
			if f.duplicate {
				runs = append(runs, f.run)
			}
			githubtest.JSON(t, w, map[string]any{"workflow_runs": runs})
		case path == "actions/runs/77/artifacts":
			githubtest.JSON(t, w, map[string]any{"artifacts": f.artifacts})
		case strings.HasPrefix(path, "actions/artifacts/"):
			_, _ = w.Write(recoveryArchive(t, "release-plan.json", &f.plan))
		case path == "actions/runs/77/rerun-failed-jobs":
			require.Equal(t, http.MethodPost, r.Method)
			f.writes++
			f.run.Attempt++
			f.run.Status, f.run.Conclusion = "queued", ""
			if f.lostReply {
				http.Error(w, "lost response", http.StatusBadGateway)
				return
			}
			w.WriteHeader(http.StatusCreated)
		case path == "actions/runs/77":
			githubtest.JSON(t, w, f.run)
		case path == "commits/main":
			t.Error("resume must never select today's main instead of the reserved source")
			http.Error(w, "main moved", http.StatusConflict)
		default:
			require.Equal(t, http.MethodGet, r.Method, "recovery must never dispatch, create tags, replace assets, or edit releases")
			value, ok := routes[path]
			require.True(t, ok, "unexpected %s", r.URL)
			githubtest.JSON(t, w, value)
		}
	})
}

func TestProductionResumeUsesOriginalRunAndSource(t *testing.T) {
	for _, scenario := range []string{"failed draft", "cancelled draft", "active draft", "before draft", "second attempt"} {
		t.Run(scenario, func(t *testing.T) {
			t.Setenv("RELEASE_RESUME", "true")
			f := newRecoveryFixture()
			switch scenario {
			case "cancelled draft":
				f.run.Conclusion = "cancelled"
			case "active draft":
				f.run.Status, f.run.Conclusion = "in_progress", ""
			case "before draft":
				f.noDraft = true
			case "second attempt":
				f.run.Attempt = 2 // Failed-job retries retain the first preparation artifact.
			}
			c := f.client(t)
			plan, err := c.commandPlan("plan-json", "", "")
			require.NoError(t, err)
			require.Equal(t, githubtest.SHA, plan.Commit)
			require.Equal(t, int64(77), plan.Resume.RunID)
			setApprovedCut(t, plan)
			watched := 0
			c.watcher = stubReleaseWatcher{watch: func(_ context.Context, repo string, run int64, _ io.Writer) error {
				watched++
				require.Equal(t, githubtest.Repo, repo)
				require.Equal(t, int64(77), run)
				f.run.Status, f.run.Conclusion = "completed", "success"
				f.published, f.noDraft = true, false
				return nil
			}}
			var out bytes.Buffer
			err = c.command("cut", &out, func(context.Context) error { t.Fatal("unexpected poll"); return nil })
			require.NoError(t, err)
			require.Equal(t, 1, watched)
			wantWrites := 1
			if scenario == "active draft" {
				wantWrites = 0
			}
			require.Equal(t, wantWrites, f.writes)
			require.Contains(t, out.String(), "Resuming v1.234.0 at "+githubtest.SHA)
			require.Contains(t, out.String(), "Published release: v1.234.0")
			require.NotContains(t, out.String(), "Dispatched request")
		})
	}
}

func TestProductionResumeRejectsUnprovenIdentity(t *testing.T) {
	for _, scenario := range []string{"published", "wrong draft commit", "prerelease", "wrong workflow", "other repo", "wrong run", "wrong source", "missing artifact", "expired artifact", "duplicate runs", "duplicate releases", "successful run"} {
		t.Run(scenario, func(t *testing.T) {
			f := newRecoveryFixture()
			switch scenario {
			case "published":
				f.published = true
			case "wrong draft commit":
				f.draft.Commit = githubtest.Base
			case "prerelease":
				f.draft.Prerelease = true
			case "wrong workflow":
				f.run.Path = ".github/workflows/test.yml"
			case "other repo":
				f.run.HeadRepository.FullName = "other/repository"
			case "wrong run":
				f.plan.RunID = "88"
			case "wrong source":
				f.plan.Commit = githubtest.Base
			case "missing artifact":
				f.artifacts = nil
			case "expired artifact":
				f.artifacts[0].Expired = true
			case "duplicate runs":
				f.duplicate = true
			case "duplicate releases":
				f.duplicateRelease = true
			case "successful run":
				f.run.Conclusion = "success"
			}
			_, err := f.client(t).recoveryPlan()
			require.Error(t, err)
			require.Zero(t, f.writes)
		})
	}
}

func TestProductionResumeConfirmationAndPublicationBoundary(t *testing.T) {
	for _, scenario := range []string{"declined", "attempt changed", "published after confirmation", "unauthorized", "lost rerun response"} {
		t.Run(scenario, func(t *testing.T) {
			t.Setenv("RELEASE_RESUME", "true")
			f := newRecoveryFixture()
			c := f.client(t)
			p, err := c.recoveryPlan()
			require.NoError(t, err)
			setApprovedCut(t, p)
			switch scenario {
			case "declined":
				t.Setenv("RELEASE_CONFIRMED", "false")
			case "attempt changed":
				f.run.Attempt++
			case "published after confirmation":
				f.published = true
			case "unauthorized":
				f.forbidden = true
			case "lost rerun response":
				f.lostReply = true
			}
			var out bytes.Buffer
			err = c.command("cut", &out, func(context.Context) error { t.Fatal("unexpected poll"); return nil })
			require.Error(t, err)
			if scenario == "lost rerun response" {
				require.Equal(t, 1, f.writes)
				require.ErrorContains(t, err, "resume result uncertain")
			} else {
				require.Zero(t, f.writes)
			}
		})
	}
}

func TestRecoveryArchiveRejectsInvalidContents(t *testing.T) {
	for _, name := range []string{"../release-plan.json", "/release-plan.json", "other.json"} {
		_, err := decodeRecoveryPlan(recoveryArchive(t, name, &newRecoveryFixture().plan))
		require.Error(t, err)
	}
	_, err := decodeRecoveryPlan([]byte("not a zip"))
	require.Error(t, err)
	_, err = decodeRecoveryPlan(recoveryArchive(t, "release-plan.json", map[string]string{"tag": "v1.234.0"}))
	require.Error(t, err)
}

func TestResumeWaitDoesNotRedispatchOnInterruption(t *testing.T) {
	f := newRecoveryFixture()
	c := f.client(t)
	err := c.waitRecoveryAttempt(&f.run, func(context.Context) error { return context.Canceled })
	require.ErrorIs(t, err, context.Canceled)
	require.ErrorContains(t, err, "reconnect")
	require.Zero(t, f.writes)
	err = c.waitRecoveryAttempt(&f.run, func(context.Context) error { return nil })
	require.ErrorContains(t, err, "not yet observed")
	require.Zero(t, f.writes)
}

func TestResumeConfirmationBindsWorkflowIdentity(t *testing.T) {
	p := approvedCutFixture()
	p.Resume = &releaseRecovery{RunID: 77, Attempt: 1, ArtifactID: 900}
	approved, err := json.Marshal(p)
	require.NoError(t, err)
	for _, changed := range []releaseRecovery{{RunID: 88, Attempt: 1, ArtifactID: 900}, {RunID: 77, Attempt: 2, ArtifactID: 900}, {RunID: 77, Attempt: 1, ArtifactID: 901}, {}} {
		copy := *p
		copy.Resume = &changed
		require.Error(t, validateApprovedPlan(string(approved), &copy), fmt.Sprint(changed))
	}
	require.NoError(t, validateApprovedPlan(string(approved), p))
}
