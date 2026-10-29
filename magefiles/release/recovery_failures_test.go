//go:build mage

package release

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"testing"

	"github.com/cloudposse/atmos/magefiles/internal/githubtest"
	"github.com/stretchr/testify/require"
)

const manyPages = 1000 // Beyond githubapi.PageLimit, so the real listing is never reached.

// resumeScenario drives a complete, successful resume of a failed release.
// The watcher stands in for the rerun finishing and publishing the draft.
func resumeScenario(t *testing.T, f *recoveryFixture) (*bytes.Buffer, error) {
	t.Helper()
	t.Setenv("RELEASE_RESUME", "true")
	c := f.client(t)
	plan, err := c.commandPlan("plan-json", "", "")
	if err != nil {
		return nil, err
	}
	setApprovedCut(t, plan)
	c.watcher = stubReleaseWatcher{watch: func(context.Context, string, int64, io.Writer) error {
		f.run.Status, f.run.Conclusion = "completed", "success"
		f.published = true
		return nil
	}}
	var out bytes.Buffer
	err = c.command("cut", &out, func(context.Context) error { t.Fatal("unexpected poll"); return nil })
	return &out, err
}

func TestResumeScenarioSucceedsWithoutFaults(t *testing.T) {
	f := newRecoveryFixture()
	out, err := resumeScenario(t, f)
	require.NoError(t, err)
	require.Contains(t, out.String(), "Published release: v1.234.0")
	require.Equal(t, 1, f.writes)
}

// TestResumeFailsClosedAtEveryRequest makes each route unavailable from its Nth
// request on, for every N the resume path reaches. Every such outage must stop the
// resume with an error and never publish or dispatch anything.
func TestResumeFailsClosedAtEveryRequest(t *testing.T) {
	routes := []string{
		"user", "milestones", "git/ref/tags/v1.234.0", "releases", "releases/tags/v1.234.0",
		"actions/workflows/publish-release.yml/runs", "actions/runs/77", "actions/runs/77/artifacts",
		"actions/artifacts/900/zip", "actions/runs/77/rerun-failed-jobs", "commits/" + githubtest.SHA,
	}
	for _, route := range routes {
		attempts := 0
		for n := 1; ; n++ {
			f := newRecoveryFixture()
			f.failFrom = map[string]int{route: n}
			_, err := resumeScenario(t, f)
			if f.hits[route] < n {
				// The route was not reached n times, so no later outage can occur.
				require.NoError(t, err, "%s from request %d", route, n)
				break
			}
			require.Error(t, err, "%s from request %d", route, n)
			attempts++
			require.LessOrEqual(t, f.writes, 1, "%s from request %d", route, n)
		}
		// Not every route is requested (the tag lookup of a draft, for example), but
		// the resume path must reach each of the others at least once.
		if route != "releases/tags/v1.234.0" {
			require.Positive(t, attempts, route)
		}
	}
}

func TestResumeRejectsInvalidMilestones(t *testing.T) {
	for name, milestones := range map[string][]Milestone{
		"no milestones":     {},
		"no release window": {{Number: 3, Title: "planning", State: "open"}},
	} {
		t.Run(name, func(t *testing.T) {
			f := newRecoveryFixture()
			f.mutateRoutes = func(routes map[string]any) { routes["milestones"] = milestones }
			_, err := f.client(t).recoveryPlan()
			require.Error(t, err)
		})
	}
}

func TestResumeRunListing(t *testing.T) {
	t.Run("ignores unrelated runs", func(t *testing.T) {
		f := newRecoveryFixture()
		f.otherRun = true
		p, err := f.client(t).recoveryPlan()
		require.NoError(t, err)
		require.Equal(t, int64(77), p.Resume.RunID)
	})
	t.Run("finds the run on a later page", func(t *testing.T) {
		f := newRecoveryFixture()
		f.runPages = 2
		p, err := f.client(t).recoveryPlan()
		require.NoError(t, err)
		require.Equal(t, int64(77), p.Resume.RunID)
	})
	t.Run("no original workflow", func(t *testing.T) {
		f := newRecoveryFixture()
		f.noRuns = true
		_, err := f.client(t).recoveryPlan()
		require.ErrorContains(t, err, "no original release workflow")
	})
	t.Run("pagination limit", func(t *testing.T) {
		f := newRecoveryFixture()
		f.runPages = manyPages
		_, err := f.client(t).recoveryPlan()
		require.ErrorContains(t, err, "pagination limit")
	})
}

func TestResumeArtifactListing(t *testing.T) {
	t.Run("finds the artifact on a later page", func(t *testing.T) {
		f := newRecoveryFixture()
		f.artifactPages = 2
		p, err := f.client(t).recoveryPlan()
		require.NoError(t, err)
		require.Equal(t, int64(900), p.Resume.ArtifactID)
	})
	t.Run("pagination limit", func(t *testing.T) {
		f := newRecoveryFixture()
		f.artifactPages = manyPages
		_, err := f.client(t).recoveryPlan()
		require.ErrorContains(t, err, "pagination limit")
	})
}

func TestSelectRecoveryArtifact(t *testing.T) {
	run := &recoveryRun{}
	run.ID, run.Attempt = 77, 3
	artifact := func(id int64, name string) recoveryArtifact { return recoveryArtifact{ID: id, Name: name} }

	t.Run("prefers the latest attempt and ignores other artifacts", func(t *testing.T) {
		found, latest, err := selectRecoveryArtifact(run, []recoveryArtifact{
			artifact(1, "release-plan-77-1"), artifact(2, "release-plan-77-3"),
			artifact(3, "release-plan-77-2"), artifact(4, "release-plan-88-3"), artifact(5, "coverage"),
		}, nil, 0)
		require.NoError(t, err)
		require.Equal(t, 3, latest)
		require.Equal(t, int64(2), found.ID)
	})
	t.Run("keeps a newer artifact found on an earlier page", func(t *testing.T) {
		earlier := artifact(9, "release-plan-77-3")
		found, latest, err := selectRecoveryArtifact(run, []recoveryArtifact{artifact(1, "release-plan-77-1")}, &earlier, 3)
		require.NoError(t, err)
		require.Equal(t, 3, latest)
		require.Equal(t, int64(9), found.ID)
	})
	for name, artifacts := range map[string][]recoveryArtifact{
		"non-numeric attempt":    {artifact(1, "release-plan-77-latest")},
		"attempt zero":           {artifact(1, "release-plan-77-0")},
		"attempt after the run":  {artifact(1, "release-plan-77-4")},
		"missing artifact ID":    {artifact(0, "release-plan-77-1")},
		"duplicate same attempt": {artifact(1, "release-plan-77-2"), artifact(2, "release-plan-77-2")},
	} {
		t.Run(name, func(t *testing.T) {
			_, _, err := selectRecoveryArtifact(run, artifacts, nil, 0)
			require.ErrorIs(t, err, ErrMilestonePolicy)
		})
	}
}

func TestResumeArtifactDownload(t *testing.T) {
	t.Run("HTTP error", func(t *testing.T) {
		f := newRecoveryFixture()
		f.downloadStatus = 404
		_, err := f.client(t).recoveryPlan()
		require.ErrorContains(t, err, "HTTP 404")
	})
	t.Run("oversized archive", func(t *testing.T) {
		f := newRecoveryFixture()
		f.rawArchive = make([]byte, recoveryPlanLimit+1)
		_, err := f.client(t).recoveryPlan()
		require.ErrorContains(t, err, "archive too large")
	})
}

func TestDecodeRecoveryPlanRejections(t *testing.T) {
	zipOf := func(t *testing.T, files func(*zip.Writer)) []byte {
		t.Helper()
		var buffer bytes.Buffer
		writer := zip.NewWriter(&buffer)
		files(writer)
		require.NoError(t, writer.Close())
		return buffer.Bytes()
	}
	valid := newRecoveryFixture().plan
	cases := map[string][]byte{
		"two files": zipOf(t, func(w *zip.Writer) {
			for _, name := range []string{"release-plan.json", "extra.json"} {
				_, err := w.Create(name)
				require.NoError(t, err)
			}
		}),
		"not a regular file": zipOf(t, func(w *zip.Writer) {
			header := &zip.FileHeader{Name: "release-plan.json"}
			header.SetMode(os.ModeSymlink | 0o777)
			_, err := w.CreateHeader(header)
			require.NoError(t, err)
		}),
		"oversized plan": zipOf(t, func(w *zip.Writer) {
			file, err := w.Create("release-plan.json")
			require.NoError(t, err)
			_, err = file.Write(bytes.Repeat([]byte(" "), recoveryPlanLimit+1))
			require.NoError(t, err)
		}),
		"not an object": recoveryArchive(t, "release-plan.json", "text"),
	}
	for name, data := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := decodeRecoveryPlan(data)
			require.Error(t, err)
		})
	}
	prepared, err := decodeRecoveryPlan(recoveryArchive(t, "release-plan.json", &valid))
	require.NoError(t, err)
	require.Equal(t, valid.Tag, prepared.Tag)
}

func TestResumeRerunAndWatchFailures(t *testing.T) {
	t.Run("unexpected rerun status", func(t *testing.T) {
		f := newRecoveryFixture()
		f.rerunStatus = 200
		_, err := resumeScenario(t, f)
		require.ErrorContains(t, err, "release rerun returned HTTP 200")
		require.ErrorContains(t, err, "resume result uncertain")
	})
	t.Run("watch fails", func(t *testing.T) {
		f := newRecoveryFixture()
		t.Setenv("RELEASE_RESUME", "true")
		c := f.client(t)
		plan, err := c.commandPlan("plan-json", "", "")
		require.NoError(t, err)
		setApprovedCut(t, plan)
		c.watcher = stubReleaseWatcher{watch: func(context.Context, string, int64, io.Writer) error { return errors.New("watch failed") }}
		err = c.command("cut", io.Discard, func(context.Context) error { return nil })
		require.ErrorContains(t, err, "watch failed")
	})
	t.Run("rerun observed only after polling", func(t *testing.T) {
		f := newRecoveryFixture()
		c := f.client(t)
		previous := f.run
		polls := 0
		err := c.waitRecoveryAttempt(&previous, func(context.Context) error {
			polls++
			f.run.Attempt = previous.Attempt + 1
			return nil
		})
		require.NoError(t, err)
		require.Equal(t, 1, polls)
	})
	t.Run("status lookup fails while waiting", func(t *testing.T) {
		f := newRecoveryFixture()
		f.failFrom = map[string]int{"actions/runs/77": 1}
		err := f.client(t).waitRecoveryAttempt(&f.run, func(context.Context) error { return nil })
		require.ErrorContains(t, err, "rerun requested; inspect")
	})
}

func TestResumeIsProductionOnly(t *testing.T) {
	t.Setenv("RELEASE_RESUME", "true")
	_, err := newRecoveryFixture().client(t).commandPlan(releaseModeCandidate, "", "")
	require.ErrorContains(t, err, "only supported for production releases")
}

func TestResumeRejectsRunFinishedAfterConfirmation(t *testing.T) {
	f := newRecoveryFixture()
	c := f.client(t)
	p, err := c.recoveryPlan()
	require.NoError(t, err)
	f.run.Conclusion = "success"
	_, err = c.confirmedRecoveryRun(p)
	require.ErrorIs(t, err, ErrMilestonePolicy)
	require.Zero(t, f.writes)
}
