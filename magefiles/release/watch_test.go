//go:build mage

package release

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"os/exec"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/magefiles/internal/githubtest"
	"github.com/cloudposse/atmos/magefiles/internal/testutil"
)

func TestMain(m *testing.M) { testutil.Main(m) }

type stubReleaseWatcher struct {
	checkErr error
	watch    func(context.Context, string, int64, io.Writer) error
}

func (w stubReleaseWatcher) Check() error { return w.checkErr }

func (w stubReleaseWatcher) Watch(ctx context.Context, repo string, id int64, out io.Writer) error {
	if w.watch != nil {
		return w.watch(ctx, repo, id, out)
	}
	return nil
}

func TestGHReleaseWatcher(t *testing.T) {
	argsFile := testutil.SetUpFakePathBinary(t, "gh")
	t.Setenv(testutil.FakeBinStdoutEnv, "live job progress\n")
	w := ghReleaseWatcher{}
	require.NoError(t, w.Check())
	var out bytes.Buffer
	require.NoError(t, w.Watch(context.Background(), githubtest.Repo, 77, &out))
	require.Equal(t, []string{"run", "watch", "77", "--repo", githubtest.Repo, "--exit-status"}, testutil.ReadFakeBinArgs(t, argsFile))
	require.Equal(t, "live job progress\n", out.String())
	t.Setenv(testutil.FakeBinExitEnv, "1")
	var exitErr *exec.ExitError
	require.ErrorAs(t, w.Watch(context.Background(), githubtest.Repo, 77, io.Discard), &exitErr)
	require.Equal(t, 1, exitErr.ExitCode())
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	require.ErrorIs(t, w.Watch(ctx, githubtest.Repo, 77, io.Discard), context.Canceled)
}

func TestGHReleaseWatcherRejectsInvalidIdentity(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	for _, tc := range []struct {
		repo string
		id   int64
	}{{"--help", 77}, {githubtest.Repo, 0}, {githubtest.Repo, -1}} {
		err := (ghReleaseWatcher{}).Watch(context.Background(), tc.repo, tc.id, io.Discard)
		require.ErrorIs(t, err, ErrReleaseVerification, "invalid identities must fail before starting gh")
	}
}

func TestReleaseMissingGHPreventsDispatch(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	for _, mode := range []string{releaseModeCut, releaseModeCandidate} {
		t.Run(mode, func(t *testing.T) {
			setApprovedCut(t, approvedCutFixture())
			c := planHTTP(t, planRoutes(), "unreachable")
			c.watcher = ghReleaseWatcher{}
			var out bytes.Buffer
			err := c.command(mode, &out, func(context.Context) error { t.Fatal("must not poll"); return nil })
			require.ErrorIs(t, err, exec.ErrNotFound)
			require.ErrorContains(t, err, "install it before dispatching")
			require.NotContains(t, out.String(), "Dispatched")
		})
	}
	for _, mode := range []string{"check", "plan-json"} {
		c := planHTTP(t, planRoutes(), "unreachable")
		c.watcher = ghReleaseWatcher{}
		require.NoError(t, c.command(mode, io.Discard, func(context.Context) error { t.Fatal("must not poll"); return nil }))
	}
}

func TestReleaseWatchFailureStopsWithoutStatusOrPublicationReads(t *testing.T) {
	for _, watchErr := range []error{context.Canceled, context.DeadlineExceeded, errors.New("gh exited 1")} {
		t.Run(watchErr.Error(), func(t *testing.T) {
			calls := 0
			c := milestoneHTTP(t, func(w http.ResponseWriter, r *http.Request) {
				calls++
				require.Equal(t, http.MethodGet, r.Method)
				require.Equal(t, planPrefix+"actions/workflows/publish-release.yml/runs", r.URL.Path)
				githubtest.JSON(t, w, map[string]any{"workflow_runs": []dispatchedRun{{ID: 77, Title: "Atmos RC v1.233.2 [request]", Event: "workflow_dispatch"}}})
			})
			c.watcher = stubReleaseWatcher{watch: func(_ context.Context, repo string, id int64, _ io.Writer) error {
				require.Equal(t, githubtest.Repo, repo)
				require.EqualValues(t, 77, id)
				return watchErr
			}}
			var out bytes.Buffer
			id, err := c.followRun("request", &out, func(context.Context) error { t.Fatal("must not poll after discovery"); return nil })
			require.EqualValues(t, 77, id)
			require.ErrorIs(t, err, watchErr)
			require.ErrorContains(t, err, c.releaseRunURL(77))
			require.ErrorContains(t, err, "gh run watch 77 --repo cloudposse/atmos --exit-status")
			require.NotContains(t, out.String(), "succeeded")
			require.Equal(t, 1, calls)
		})
	}
}

func TestReleaseWatchSuccessStillRequiresCompletedRun(t *testing.T) {
	c := milestoneHTTP(t, func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, planPrefix+"actions/runs/77", r.URL.Path)
		githubtest.JSON(t, w, map[string]string{"status": "in_progress"})
	})
	var out bytes.Buffer
	err := c.watchRun(77, &out)
	require.ErrorIs(t, err, ErrReleaseVerification)
	require.ErrorContains(t, err, "gh run watch 77 --repo cloudposse/atmos --exit-status")
	require.Empty(t, out.String())
}

func TestCandidatePlanOutput(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	p := approvedCutFixture()
	p.Mode = releaseModeCandidate
	var out bytes.Buffer
	require.NoError(t, writePlanSummary(&out, p))
	require.Contains(t, out.String(), "Release candidate for 1.234.0\n")
	require.Contains(t, out.String(), "RC tag will be assigned during workflow preparation.")
	require.NotContains(t, out.String(), "-rc.1", "the RC number is not known locally")
}

func TestReleaseImmutabilityPreflightPreventsDispatch(t *testing.T) {
	for _, mode := range []string{releaseModeCut, releaseModeCandidate} {
		for _, failure := range []string{"disabled", "unavailable"} {
			t.Run(mode+"/"+failure, func(t *testing.T) {
				setApprovedCut(t, approvedCutFixture())
				routes := planRoutes()
				routes["immutable-releases"] = map[string]bool{"enabled": false}
				failPath := "unreachable"
				if failure == "unavailable" {
					failPath = "immutable-releases"
				}
				c := planHTTP(t, routes, failPath)
				var out bytes.Buffer
				err := c.command(mode, &out, func(context.Context) error { t.Fatal("must not poll"); return nil })
				require.Error(t, err)
				if failure == "disabled" {
					require.ErrorIs(t, err, ErrReleaseVerification)
					require.ErrorContains(t, err, "immutable releases are disabled")
				} else {
					require.ErrorContains(t, err, "Administration: read is required")
				}
				require.NotContains(t, out.String(), "Dispatched")
			})
		}
	}
}
