package terraform

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/ci/internal/plugin"
	"github.com/cloudposse/atmos/pkg/ci/internal/provider"
	"github.com/cloudposse/atmos/pkg/schema"
)

// Compile-time sentinels for the schema fields these tests set.
var _ = schema.CIConfig{AllowUnsafeForkExecution: true, Checks: schema.CIChecksConfig{Enabled: boolPtr(true)}}

// markElevatedFork turns the hook context into a fork pull request on an elevated event
// (pull_request_target or workflow_run), the case the posting gate holds.
func markElevatedFork(ctx *plugin.HookContext) {
	ctx.CICtx.ElevatedEvent = true
	if ctx.CICtx.PullRequest == nil {
		ctx.CICtx.PullRequest = &provider.PRInfo{Number: 42}
	}
	ctx.CICtx.PullRequest.Fork = true
}

// checksHookContext is a hook context with commit statuses enabled and a resolved component.
func checksHookContext() *plugin.HookContext {
	return &plugin.HookContext{
		Config: &schema.AtmosConfiguration{CI: schema.CIConfig{
			Enabled: true,
			Checks:  schema.CIChecksConfig{Enabled: boolPtr(true)},
		}},
		Provider: newMockProvider(),
		CICtx:    &provider.Context{RepoOwner: "owner", RepoName: "repo", SHA: "abc123", RunURL: "https://ci.example/run/1", PullRequest: &provider.PRInfo{Number: 42}},
		Command:  "plan",
		Info:     &schema.ConfigAndStacksInfo{Stack: "dev", ComponentFromArg: "vpc"},
	}
}

// TestNativeComments_ForkGate verifies native plan comments go through the same posting gate as scripts.
func TestNativeComments_ForkGate(t *testing.T) {
	t.Run("a fork pull request on an elevated event is not commented on", func(t *testing.T) {
		ctx := commentsHookContext(t)
		markElevatedFork(ctx)
		mp := ctx.Provider.(*mockProvider)

		require.NoError(t, (&Plugin{}).onAfterPlan(ctx))

		assert.Empty(t, mp.commentCalls, "the comment must be held, not posted")
	})

	t.Run("unsafe fork execution releases the comment", func(t *testing.T) {
		ctx := commentsHookContext(t)
		markElevatedFork(ctx)
		ctx.Config.CI.AllowUnsafeForkExecution = true
		mp := ctx.Provider.(*mockProvider)

		require.NoError(t, (&Plugin{}).onAfterPlan(ctx))

		assert.Len(t, mp.commentCalls, 1)
	})

	t.Run("a same-repository pull request on an elevated event is commented on", func(t *testing.T) {
		ctx := commentsHookContext(t)
		ctx.CICtx.ElevatedEvent = true
		mp := ctx.Provider.(*mockProvider)

		require.NoError(t, (&Plugin{}).onAfterPlan(ctx))

		assert.Len(t, mp.commentCalls, 1)
	})

	t.Run("a fork pull request on a plain pull_request event is commented on", func(t *testing.T) {
		ctx := commentsHookContext(t)
		ctx.CICtx.PullRequest.Fork = true
		mp := ctx.Provider.(*mockProvider)

		require.NoError(t, (&Plugin{}).onAfterPlan(ctx))

		assert.Len(t, mp.commentCalls, 1)
	})
}

func TestNativeAggregateComment_ForkGate(t *testing.T) {
	t.Run("held for a fork pull request on an elevated event", func(t *testing.T) {
		ctx := newAggregateHookContext()
		markElevatedFork(ctx)
		mp := ctx.Provider.(*mockProvider)

		require.NoError(t, (&Plugin{}).postAggregateComment(ctx, "summary"))

		assert.Empty(t, mp.commentCalls)
	})

	t.Run("released by unsafe fork execution", func(t *testing.T) {
		ctx := newAggregateHookContext()
		markElevatedFork(ctx)
		ctx.Config.CI.AllowUnsafeForkExecution = true
		mp := ctx.Provider.(*mockProvider)

		require.NoError(t, (&Plugin{}).postAggregateComment(ctx, "summary"))

		require.Len(t, mp.commentCalls, 1)
		assert.Contains(t, mp.commentCalls[0].Marker, "aggregate")
	})
}

func TestNativeChecks_ForkGate(t *testing.T) {
	t.Run("statuses are held for a fork pull request on an elevated event", func(t *testing.T) {
		ctx := checksHookContext()
		markElevatedFork(ctx)
		mp := ctx.Provider.(*mockProvider)
		p := &Plugin{}

		require.NoError(t, p.createCheckRun(ctx))
		require.NoError(t, p.updateCheckRun(ctx, nil))

		assert.Empty(t, mp.checkRunCalls)
		assert.Empty(t, mp.updateRunCalls)
	})

	t.Run("statuses are written for a same-repository pull request on an elevated event", func(t *testing.T) {
		ctx := checksHookContext()
		ctx.CICtx.ElevatedEvent = true
		mp := ctx.Provider.(*mockProvider)
		p := &Plugin{}

		require.NoError(t, p.createCheckRun(ctx))
		require.NoError(t, p.updateCheckRun(ctx, nil))

		require.Len(t, mp.checkRunCalls, 1)
		require.Len(t, mp.updateRunCalls, 1)
		assert.Equal(t, provider.CheckRunStateInProgress, mp.checkRunCalls[0].Status)
		assert.Equal(t, "https://ci.example/run/1", mp.checkRunCalls[0].DetailsURL)
		assert.Equal(t, "abc123", mp.checkRunCalls[0].SHA)
		assert.Equal(t, "owner", mp.checkRunCalls[0].Owner)
	})

	t.Run("statuses are released by unsafe fork execution", func(t *testing.T) {
		ctx := checksHookContext()
		markElevatedFork(ctx)
		ctx.Config.CI.AllowUnsafeForkExecution = true
		mp := ctx.Provider.(*mockProvider)

		require.NoError(t, (&Plugin{}).createCheckRun(ctx))

		assert.Len(t, mp.checkRunCalls, 1)
	})
}
