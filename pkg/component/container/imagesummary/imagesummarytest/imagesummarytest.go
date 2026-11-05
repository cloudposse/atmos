// Package imagesummarytest provides a CI harness for tests of container image reporting. It runs the
// real CI reporter against the real GitHub provider and the fake GitHub API, so tests exercise the
// same gating, rendering, and comment upsert path as a build or push in GitHub Actions.
package imagesummarytest

import (
	"testing"

	"github.com/cloudposse/atmos/pkg/ci"
	"github.com/cloudposse/atmos/pkg/ci/providers/generic"
	"github.com/cloudposse/atmos/pkg/ci/providers/github"
	"github.com/cloudposse/atmos/pkg/ci/providers/github/ghtest"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// PullRequest is the number of the pull request the harness runs against.
const PullRequest = 42

// GitHub is a fake GitHub Actions run on a pull request.
type GitHub struct {
	// Server records the comments and requests the run made.
	Server *ghtest.Server
	// Env holds the runner files, such as the step summary.
	Env ghtest.Env
}

// NewGitHub points the process at a fake GitHub Actions pull request run for the duration of the test.
// It changes the working directory and the CI provider registry, which the test cleanup restores.
func NewGitHub(t testing.TB, serverOpts ...ghtest.Option) *GitHub {
	defer perf.Track(nil, "imagesummarytest.NewGitHub")()

	t.Helper()

	for _, k := range []string{"ATMOS_CI_OUTPUT", "ATMOS_CI_SUMMARY", "ATMOS_CI_PR", "ATMOS_CI_REPOSITORY"} {
		t.Setenv(k, "")
	}
	t.Setenv("NO_COLOR", "1")
	s := ghtest.NewServer(t, serverOpts...)
	env := ghtest.SetEnv(
		t, s,
		ghtest.WithRepository("owner/repo"),
		ghtest.WithPullRequest(PullRequest, "feature", "main"),
		ghtest.WithRunID("777"),
	)
	// Resolve the SHA from GITHUB_SHA rather than the surrounding git checkout.
	t.Chdir(t.TempDir())
	ghtest.RegisterProvider(t, github.NewProvider())
	ci.Register(generic.NewProvider())
	return &GitHub{Server: s, Env: env}
}

// Summary returns what the run appended to the job summary.
func (g *GitHub) Summary(t testing.TB) string {
	defer perf.Track(nil, "imagesummarytest.GitHub.Summary")()

	t.Helper()
	return ghtest.ReadFile(t, g.Env.Summary)
}

// Comments returns the current comments of the pull request.
func (g *GitHub) Comments() []ghtest.Comment {
	defer perf.Track(nil, "imagesummarytest.GitHub.Comments")()

	return g.Server.CommentsFor("owner", "repo", PullRequest)
}

// Config returns a CI configuration with ci.enabled on and ci.comments.enabled set to commentsEnabled.
func Config(commentsEnabled bool) *schema.AtmosConfiguration {
	defer perf.Track(nil, "imagesummarytest.Config")()

	cfg := &schema.AtmosConfiguration{CI: schema.CIConfig{Enabled: true}}
	cfg.CI.Comments.Enabled = &commentsEnabled
	return cfg
}
