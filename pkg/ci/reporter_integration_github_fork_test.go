package ci_test

import (
	"context"
	"os"
	"strings"
	"testing"

	cockroachdb "github.com/cockroachdb/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ci"
	"github.com/cloudposse/atmos/pkg/ci/providers/generic"
	"github.com/cloudposse/atmos/pkg/ci/providers/github"
	"github.com/cloudposse/atmos/pkg/ci/providers/github/ghtest"
)

// elevatedPayload overlays a pull_request payload on an elevated event. The head and base
// arguments are the repository objects; a nil head stands for a deleted fork.
func elevatedPayload(head, base map[string]any) map[string]any {
	headObj := map[string]any{"ref": "feature"}
	if head != nil {
		headObj["repo"] = head
	} else {
		headObj["repo"] = nil
	}
	return map[string]any{
		"action": "synchronize",
		"pull_request": map[string]any{
			"number": 42,
			"head":   headObj,
			"base":   map[string]any{"ref": "main", "repo": base},
		},
	}
}

func repoObj(fullName string, fork bool) map[string]any {
	return map[string]any{"full_name": fullName, "fork": fork}
}

func postComment(t *testing.T, h *harness) (ci.Receipt, error) {
	t.Helper()
	return h.reporter.Comment(context.Background(), ci.CommentRequest{Body: "plan body", Key: "plan", Target: ci.CommentTargetPR})
}

// TestReporter_ElevatedEventsUseThePayloadForThePullRequest verifies the pull request number comes
// from the event payload. GitHub runs pull_request_target and workflow_run against the base branch, so
// GITHUB_REF is refs/heads/<base> and names no pull request.
func TestReporter_ElevatedEventsUseThePayloadForThePullRequest(t *testing.T) {
	t.Run("pull_request_target for a same-repository pull request posts to the pull request", func(t *testing.T) {
		h := newHarness(
			t, fullCIConfig(), nil,
			ghtest.WithRepository("owner/repo"),
			ghtest.WithEvent("pull_request_target", map[string]any{"action": "synchronize"}),
			ghtest.WithPullRequest(42, "feature", "main"),
		)
		assert.Equal(t, "refs/heads/main", os.Getenv("GITHUB_REF"), "the fixture matches what GitHub sets")

		rc, err := postComment(t, h)

		require.NoError(t, err)
		assert.False(t, rc.Local)
		assert.Empty(t, rc.Gate)
		require.Len(t, h.server.CommentsFor("owner", "repo", 42), 1)
	})

	t.Run("workflow_run for a same-repository pull request keeps the pull request", func(t *testing.T) {
		h := newHarness(
			t, fullCIConfig(), nil,
			ghtest.WithRepository("owner/repo"),
			ghtest.WithEvent("workflow_run", nil),
			ghtest.WithPullRequest(42, "feature", "main"),
		)

		c, err := h.reporter.Context()
		require.NoError(t, err)
		assert.True(t, c.ElevatedEvent)
		require.NotNil(t, c.PullRequest)
		assert.Equal(t, 42, c.PullRequest.Number)
		assert.False(t, c.PullRequest.Fork)

		rc, err := postComment(t, h)

		require.NoError(t, err)
		assert.Empty(t, rc.Gate)
		require.Len(t, h.server.CommentsFor("owner", "repo", 42), 1)
	})

	t.Run("workflow_run for a fork is held", func(t *testing.T) {
		h := newHarness(
			t, fullCIConfig(), nil,
			ghtest.WithRepository("owner/repo"),
			ghtest.WithEvent("workflow_run", nil),
			ghtest.WithForkPullRequest(42, "feature", "main"),
		)

		rc, err := h.reporter.Comment(context.Background(), ci.CommentRequest{Body: "b", Key: "plan", PR: 42})

		require.NoError(t, err)
		assert.Equal(t, ci.FeatureForkGate, rc.Gate)
		assert.True(t, rc.Local)
		h.assertNothingWritten(t)
	})
}

// TestReporter_ForkClassification covers the cases where the payload makes the fork status
// doubtful. A repository that is itself a fork hosts same-repository pull requests, and a deleted
// fork or missing repository cannot be proven safe.
func TestReporter_ForkClassification(t *testing.T) {
	const repo = "owner/repo"
	cases := []struct {
		name     string
		head     map[string]any
		base     map[string]any
		wantHeld bool
	}{
		{"head in another repository", repoObj("forker/repo", true), repoObj(repo, false), true},
		{"deleted fork (head repo is null)", nil, repoObj(repo, false), true},
		{"repository that is itself a fork, same-repository pull request", repoObj(repo, true), repoObj(repo, true), false},
		{"head flagged as fork but same full name", repoObj(repo, true), repoObj(repo, false), false},
		{"ordinary same-repository pull request", repoObj(repo, false), repoObj(repo, false), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := newHarness(
				t, fullCIConfig(), nil,
				ghtest.WithRepository(repo),
				ghtest.WithEvent("pull_request_target", elevatedPayload(tc.head, tc.base)),
				ghtest.WithPullRequest(42, "feature", "main"),
			)

			rc, err := postComment(t, h)

			require.NoError(t, err)
			if tc.wantHeld {
				assert.Equal(t, ci.FeatureForkGate, rc.Gate)
				assert.True(t, rc.Local)
				h.assertNothingWritten(t)
				return
			}
			assert.Empty(t, rc.Gate)
			assert.False(t, rc.Local)
			assert.Len(t, h.server.CommentsFor("owner", "repo", 42), 1)
		})
	}

	t.Run("an unreadable payload holds the write", func(t *testing.T) {
		h := newHarness(
			t, fullCIConfig(), nil,
			ghtest.WithRepository(repo),
			ghtest.WithEvent("pull_request_target", map[string]any{"action": "synchronize"}),
			ghtest.WithPullRequest(42, "feature", "main"),
		)
		t.Setenv("GITHUB_EVENT_PATH", "")

		rc, err := h.reporter.Comment(context.Background(), ci.CommentRequest{Body: "b", Key: "plan", PR: 42})

		require.NoError(t, err)
		assert.Equal(t, ci.FeatureForkGate, rc.Gate)
		h.assertNothingWritten(t)
	})
}

// TestReporter_ForkGateCoversEnvPathAndSARIF verifies that writes which publish beyond the job are held
// for a fork pull request on an elevated event, and that job-local writes are not.
func TestReporter_ForkGateCoversEnvPathAndSARIF(t *testing.T) {
	forkEnv := func() []ghtest.EnvOption {
		return []ghtest.EnvOption{
			ghtest.WithRepository("owner/repo"),
			ghtest.WithEvent("pull_request_target", map[string]any{"action": "synchronize"}),
			ghtest.WithForkPullRequest(42, "fork-branch", "main"),
		}
	}
	report := ci.SARIFReport{Body: []byte(`{"version":"2.1.0","runs":[]}`), Category: "tfsec"}

	t.Run("env path and sarif are held", func(t *testing.T) {
		h := newHarness(t, fullCIConfig(), nil, forkEnv()...)

		rcEnv, err := h.reporter.Env("INJECTED", "1")
		require.NoError(t, err)
		rcPath, err := h.reporter.Path("/tmp/evil")
		require.NoError(t, err)
		rcSARIF, err := h.reporter.SARIF(context.Background(), report)
		require.NoError(t, err)

		for _, rc := range []ci.Receipt{rcEnv, rcPath, rcSARIF} {
			assert.Equal(t, ci.FeatureForkGate, rc.Gate)
			assert.True(t, rc.Local)
			assert.Equal(t, generic.ProviderName, rc.Provider)
		}
		assert.Contains(t, h.local.String(), "export INJECTED=")
		assert.Contains(t, h.local.String(), "held for a fork pull request")
		h.assertNothingWritten(t)
	})

	t.Run("summary output and annotations still reach the job", func(t *testing.T) {
		h := newHarness(t, fullCIConfig(), nil, forkEnv()...)

		rc, err := h.reporter.Summary("## Plan")
		require.NoError(t, err)
		assert.Empty(t, rc.Gate)
		rc, err = h.reporter.Output("k", "v")
		require.NoError(t, err)
		assert.Empty(t, rc.Gate)
		rc, err = h.reporter.Annotate(ci.Annotation{Path: "main.tf", StartLine: 1, Level: ci.AnnotationWarning, Message: "m"})
		require.NoError(t, err)
		assert.Empty(t, rc.Gate)

		assert.Contains(t, ghtest.ReadFile(t, h.env.Summary), "## Plan")
		assert.Contains(t, ghtest.ReadFile(t, h.env.Output), "k=v")
		assert.Contains(t, h.uiStderr.String(), "::warning file=main.tf,line=1::m")
		assert.Empty(t, ghtest.ReadFile(t, h.env.EnvFile))
		assert.Empty(t, ghtest.ReadFile(t, h.env.Path))
		assert.Empty(t, h.server.SARIFUploads())
	})

	t.Run("unsafe fork execution releases them", func(t *testing.T) {
		cfg := fullCIConfig()
		cfg.CI.AllowUnsafeForkExecution = true
		h := newHarness(t, cfg, nil, forkEnv()...)

		_, err := h.reporter.Env("OK", "1")
		require.NoError(t, err)
		_, err = h.reporter.Path("/opt/bin")
		require.NoError(t, err)
		_, err = h.reporter.SARIF(context.Background(), report)
		require.NoError(t, err)

		assert.Contains(t, ghtest.ReadFile(t, h.env.EnvFile), "OK=1\n")
		assert.Equal(t, "/opt/bin\n", ghtest.ReadFile(t, h.env.Path))
		assert.Len(t, h.server.SARIFUploads(), 1)
	})

	t.Run("a plain pull_request from a fork is not gated", func(t *testing.T) {
		h := newHarness(
			t, fullCIConfig(), nil,
			ghtest.WithRepository("owner/repo"),
			ghtest.WithForkPullRequest(42, "fork-branch", "main"),
		)
		c, err := h.reporter.Context()
		require.NoError(t, err)
		require.NotNil(t, c.PullRequest)
		assert.True(t, c.PullRequest.Fork)
		assert.False(t, c.ElevatedEvent)

		rc, err := postComment(t, h)

		require.NoError(t, err)
		assert.Empty(t, rc.Gate)
		assert.Len(t, h.server.CommentsFor("owner", "repo", 42), 1)
	})
}

// TestReporter_CommitComments drives target=commit and target=auto against the fake GitHub API.
func TestReporter_CommitComments(t *testing.T) {
	const sha = "abcdef0123456789abcdef0123456789abcdef01"
	ctx := context.Background()
	pushEnv := func() []ghtest.EnvOption {
		return []ghtest.EnvOption{ghtest.WithRepository("owner/repo"), ghtest.WithSHA(sha)}
	}

	t.Run("auto on a push comments on the commit, then updates it in place", func(t *testing.T) {
		h := newHarness(t, fullCIConfig(), nil, pushEnv()...)

		first, err := h.reporter.Comment(ctx, ci.CommentRequest{Body: "deployed v1", Key: "deploy"})
		require.NoError(t, err)
		assert.False(t, first.Local)
		require.NotNil(t, first.Comment)
		assert.Equal(t, ci.CommentTargetCommit, first.Comment.Target)
		assert.True(t, first.Comment.Created)

		second, err := h.reporter.Comment(ctx, ci.CommentRequest{Body: "deployed v2", Key: "deploy"})
		require.NoError(t, err)
		assert.False(t, second.Comment.Created)
		assert.Equal(t, first.Comment.ID, second.Comment.ID)

		writes := h.server.Comments()
		require.Len(t, writes, 2)
		assert.Equal(t, sha, writes[0].SHA)
		assert.False(t, writes[0].Edited)
		assert.True(t, writes[1].Edited)
		assert.Contains(t, writes[1].Body, "<!-- atmos:ci:deploy -->")

		current := h.server.CommitCommentsFor("owner", "repo", sha)
		require.Len(t, current, 1)
		assert.Contains(t, current[0].Body, "deployed v2")
		assert.NotContains(t, current[0].Body, "deployed v1")
	})

	t.Run("update without an existing commit comment is not found", func(t *testing.T) {
		h := newHarness(t, fullCIConfig(), nil, pushEnv()...)

		_, err := h.reporter.Comment(ctx, ci.CommentRequest{Body: "b", Key: "deploy", Behavior: ci.CommentBehaviorUpdate})

		require.ErrorIs(t, err, errUtils.ErrCICommentPostFailed)
		require.ErrorIs(t, err, errUtils.ErrCICommentNotFound)
		assert.Empty(t, h.server.Comments())
	})

	t.Run("target commit on a pull request run still comments on the commit", func(t *testing.T) {
		h := newHarness(t, fullCIConfig(), nil, append(prEnv(), ghtest.WithSHA(sha))...)

		rc, err := h.reporter.Comment(ctx, ci.CommentRequest{Body: "b", Key: "k", Target: ci.CommentTargetCommit})

		require.NoError(t, err)
		assert.Equal(t, ci.CommentTargetCommit, rc.Comment.Target)
		assert.Len(t, h.server.CommitCommentsFor("owner", "repo", sha), 1)
		assert.Empty(t, h.server.CommentsFor("owner", "repo", 42))
	})

	t.Run("auto on a pull request run comments on the pull request", func(t *testing.T) {
		h := newHarness(t, fullCIConfig(), nil, append(prEnv(), ghtest.WithSHA(sha))...)

		rc, err := h.reporter.Comment(ctx, ci.CommentRequest{Body: "b", Key: "k"})

		require.NoError(t, err)
		assert.Equal(t, ci.CommentTargetPR, rc.Comment.Target)
		assert.Empty(t, h.server.CommitCommentsFor("owner", "repo", sha))
		assert.Len(t, h.server.CommentsFor("owner", "repo", 42), 1)
	})

	t.Run("target pr on a push is an error with a host-neutral hint", func(t *testing.T) {
		h := newHarness(t, fullCIConfig(), nil, pushEnv()...)

		_, err := h.reporter.Comment(ctx, ci.CommentRequest{Body: "b", Target: ci.CommentTargetPR})

		require.ErrorIs(t, err, errUtils.ErrCIPullRequestUnknown)
		hints := strings.Join(cockroachdb.GetAllHints(err), "\n")
		assert.Contains(t, hints, "pull request number")
		assert.NotContains(t, hints, "pr=123")
		assert.Empty(t, h.server.Requests())
	})

	t.Run("a commit comment for a fork pull request on an elevated event is held", func(t *testing.T) {
		h := newHarness(
			t, fullCIConfig(), nil,
			ghtest.WithRepository("owner/repo"),
			ghtest.WithSHA(sha),
			ghtest.WithEvent("pull_request_target", map[string]any{"action": "synchronize"}),
			ghtest.WithForkPullRequest(42, "fork-branch", "main"),
		)

		rc, err := h.reporter.Comment(ctx, ci.CommentRequest{Body: "b", Key: "k", Target: ci.CommentTargetCommit})

		require.NoError(t, err)
		assert.Equal(t, ci.FeatureForkGate, rc.Gate)
		assert.Contains(t, h.local.String(), "commit comment preview")
		h.assertNothingWritten(t)
	})

	t.Run("comments disabled renders a commit comment preview locally", func(t *testing.T) {
		cfg := fullCIConfig()
		cfg.CI.Comments.Enabled = ptr(false)
		h := newHarness(t, cfg, nil, pushEnv()...)

		rc, err := h.reporter.Comment(ctx, ci.CommentRequest{Body: "b", Key: "k"})

		require.NoError(t, err)
		assert.Equal(t, ci.FeatureComments, rc.Gate)
		assert.Contains(t, h.local.String(), "commit comment preview")
		h.assertNothingWritten(t)
	})
}

// TestReporter_NestedGroupRendersAPlainHeading verifies a group opened inside another does not emit a
// nested ::group:: marker, which GitHub does not support.
func TestReporter_NestedGroupRendersAPlainHeading(t *testing.T) {
	t.Setenv("ATMOS_CI_LOG_GROUP_ACTIVE", "")
	h := newHarness(t, fullCIConfig(), nil, prEnv()...)

	endOuter, rcOuter, err := h.reporter.Group("Outer")
	require.NoError(t, err)
	endInner, rcInner, err := h.reporter.Group("Inner")
	require.NoError(t, err)
	endInner()
	endOuter()

	assert.False(t, rcOuter.Local)
	assert.True(t, rcInner.Local)
	assert.Equal(t, "::group::Outer\n::endgroup::\n", h.stdout.String(), "the inner group emits no provider marker")
	assert.Contains(t, h.local.String(), "Inner", "the inner title is rendered as a plain heading")

	// The slot is free again once the outer group closes.
	end, rc, err := h.reporter.Group("Again")
	require.NoError(t, err)
	end()
	assert.False(t, rc.Local)
	assert.Contains(t, h.stdout.String(), "::group::Again\n::endgroup::\n")
}

// TestReporter_CheckReceiptsCarryTheDetailsURL verifies the receipt names where the check links to,
// and that the GitHub state mapping is lossy in the documented direction.
func TestReporter_CheckReceiptsCarryTheDetailsURL(t *testing.T) {
	h := newHarness(t, fullCIConfig(), nil, prEnv()...)
	runURL, err := runURLOf(h)
	require.NoError(t, err)
	require.NotEmpty(t, runURL)

	created, err := h.reporter.Check(context.Background(), ci.CheckRequest{Name: "atmos/plan", State: ci.CheckRunStateInProgress})
	require.NoError(t, err)
	assert.Equal(t, runURL, created.Check.DetailsURL)

	updated, err := h.reporter.UpdateCheck(context.Background(), ci.CheckRequest{Name: "atmos/plan", ID: created.Check.ID, State: ci.CheckRunStateCancelled})
	require.NoError(t, err)
	assert.Equal(t, runURL, updated.Check.DetailsURL)

	statuses := h.server.Statuses()
	require.Len(t, statuses, 2)
	assert.Equal(t, "pending", statuses[0].State, "in_progress is reported as pending")
	assert.Equal(t, "error", statuses[1].State, "cancelled is reported as error")
}

var _ = github.ProviderName
