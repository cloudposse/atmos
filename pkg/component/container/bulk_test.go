package container

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
)

// twoStacks is a describe-stacks map with two stacks and three (non-abstract)
// container components: dev/api, dev/worker, prod/api.
func twoStacks() map[string]any {
	return map[string]any{
		"dev": map[string]any{
			cfg.ComponentsSectionName: map[string]any{
				cfg.ContainerComponentType: map[string]any{
					"api":    map[string]any{"image": "api:dev"},
					"worker": map[string]any{"image": "worker:dev"},
				},
			},
		},
		"prod": containerStack(map[string]string{"api": "api:prod"}),
	}
}

func TestIsBulkVerb(t *testing.T) {
	for _, v := range []string{"build", "push", "pull", "up", "restart", "stop", "rm", "down"} {
		assert.True(t, isBulkVerb(v), "%q should be a bulk verb", v)
	}
	for _, v := range []string{"run", "exec", "attach", "logs", "ps", "list", "bogus"} {
		assert.False(t, isBulkVerb(v), "%q should not be a bulk verb", v)
	}
}

func TestShouldRunBulk(t *testing.T) {
	// Bulk verb + --all => bulk.
	assert.True(t, shouldRunBulk("up", &schema.ConfigAndStacksInfo{All: true, ComponentFromArg: "api"}))
	// Bulk verb + no component => bulk (interactive).
	assert.True(t, shouldRunBulk("down", &schema.ConfigAndStacksInfo{}))
	// Bulk verb + component, no --all => single.
	assert.False(t, shouldRunBulk("up", &schema.ConfigAndStacksInfo{ComponentFromArg: "api"}))
	// Non-bulk verb never goes bulk, even with no component.
	assert.False(t, shouldRunBulk("logs", &schema.ConfigAndStacksInfo{}))
	assert.False(t, shouldRunBulk("exec", &schema.ConfigAndStacksInfo{All: true}))
}

func TestOrderTargets(t *testing.T) {
	rows := []instanceRow{
		{stack: "dev", component: "api"},
		{stack: "dev", component: "worker"},
		{stack: "prod", component: "api"},
	}

	// Forward verbs preserve input (sorted) order.
	forward := orderTargets(rows, "up")
	require.Len(t, forward, 3)
	assert.Equal(t, "api", forward[0].component)
	assert.Equal(t, "prod", forward[2].stack)

	// Teardown verbs reverse the order.
	for _, verb := range []string{"down", "stop", "rm"} {
		rev := orderTargets(rows, verb)
		require.Len(t, rev, 3)
		assert.Equal(t, instanceRow{stack: "prod", component: "api"}, rev[0], "verb %q", verb)
		assert.Equal(t, instanceRow{stack: "dev", component: "api"}, rev[2], "verb %q", verb)
	}

	// Original slice is not mutated.
	assert.Equal(t, "api", rows[0].component)
	assert.Equal(t, "prod", rows[2].stack)
}

func TestDistinctStacks(t *testing.T) {
	rows := []instanceRow{
		{stack: "prod", component: "api"},
		{stack: "dev", component: "api"},
		{stack: "dev", component: "worker"},
	}
	assert.Equal(t, []string{"dev", "prod"}, distinctStacks(rows))
	assert.Empty(t, distinctStacks(nil))
}

func TestFilterByStackAndComponents(t *testing.T) {
	rows := []instanceRow{
		{stack: "dev", component: "api"},
		{stack: "dev", component: "worker"},
		{stack: "prod", component: "api"},
	}

	dev := filterByStack(rows, "dev")
	require.Len(t, dev, 2)
	assert.Equal(t, []string{"api", "worker"}, componentNames(dev))

	chosen := filterByComponents(dev, []string{"worker"})
	require.Len(t, chosen, 1)
	assert.Equal(t, "worker", chosen[0].component)

	assert.Empty(t, filterByStack(rows, "nope"))
	assert.Empty(t, filterByComponents(dev, []string{"nope"}))
}

func TestRunBulk_ContinueOnErrorAndAggregate(t *testing.T) {
	var calls []string
	exec := func(_ context.Context, info *schema.ConfigAndStacksInfo) error {
		calls = append(calls, info.Stack+"/"+info.ComponentFromArg)
		// Per-target info is isolated and never bulk.
		assert.False(t, info.All)
		assert.Equal(t, info.ComponentFromArg, info.Component)
		if info.ComponentFromArg == "worker" {
			return assert.AnError
		}
		return nil
	}

	targets := []instanceRow{
		{stack: "dev", component: "api"},
		{stack: "dev", component: "worker"},
		{stack: "prod", component: "api"},
	}
	err := runBulk(context.Background(), &schema.ConfigAndStacksInfo{All: true}, "up", exec, targets)

	// All three ran despite the middle failure, and the failure is aggregated.
	require.Error(t, err)
	assert.ErrorIs(t, err, assert.AnError)
	assert.Equal(t, []string{"dev/api", "dev/worker", "prod/api"}, calls)
}

func TestRunBulk_AllSucceed(t *testing.T) {
	exec := func(_ context.Context, _ *schema.ConfigAndStacksInfo) error { return nil }
	targets := []instanceRow{{stack: "dev", component: "api"}}
	require.NoError(t, runBulk(context.Background(), &schema.ConfigAndStacksInfo{}, "up", exec, targets))
}

func TestExecuteBulk_ComponentWithAllRejected(t *testing.T) {
	err := ExecuteBulk(context.Background(), &schema.ConfigAndStacksInfo{All: true, ComponentFromArg: "api"}, "up")
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrContainerComponentWithAll)
}

func TestExecuteBulk_UnknownVerb(t *testing.T) {
	err := ExecuteBulk(context.Background(), &schema.ConfigAndStacksInfo{All: true}, "bogus")
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrComponentExecutionFailed)
}

// withBulkExecutor temporarily replaces the executor for a verb so ExecuteBulk
// can be exercised end-to-end without a real runtime, recording the calls.
func withBulkExecutor(t *testing.T, verb string, fn singleExecutor) {
	t.Helper()
	orig := bulkExecutors[verb]
	t.Cleanup(func() { bulkExecutors[verb] = orig })
	bulkExecutors[verb] = fn
}

func TestExecuteBulk_AllRunsEveryComponent(t *testing.T) {
	withListStubs(t, twoStacks(), nil, nil, nil)

	var calls []string
	withBulkExecutor(t, "up", func(_ context.Context, info *schema.ConfigAndStacksInfo) error {
		calls = append(calls, info.Stack+"/"+info.ComponentFromArg)
		return nil
	})

	require.NoError(t, ExecuteBulk(context.Background(), &schema.ConfigAndStacksInfo{All: true}, "up"))
	// Sorted order for a forward verb.
	assert.Equal(t, []string{"dev/api", "dev/worker", "prod/api"}, calls)
}

func TestExecuteBulk_AllReverseForTeardown(t *testing.T) {
	withListStubs(t, twoStacks(), nil, nil, nil)

	var calls []string
	withBulkExecutor(t, "down", func(_ context.Context, info *schema.ConfigAndStacksInfo) error {
		calls = append(calls, info.Stack+"/"+info.ComponentFromArg)
		return nil
	})

	require.NoError(t, ExecuteBulk(context.Background(), &schema.ConfigAndStacksInfo{All: true}, "down"))
	// Reverse-sorted order for a teardown verb.
	assert.Equal(t, []string{"prod/api", "dev/worker", "dev/api"}, calls)
}

func TestExecuteBulk_NoComponentsIsNoop(t *testing.T) {
	withListStubs(t, map[string]any{}, nil, nil, nil)
	require.NoError(t, ExecuteBulk(context.Background(), &schema.ConfigAndStacksInfo{All: true}, "up"))
}

// withPromptStubs replaces the interactive picker seams so selection logic can be
// tested without a TTY.
func withPromptStubs(t *testing.T, value string, valueErr error, values []string, valuesErr error) {
	t.Helper()
	origV, origM := promptForValue, promptForMultipleValues
	t.Cleanup(func() { promptForValue, promptForMultipleValues = origV, origM })
	promptForValue = func(_, _ string, _ []string) (string, error) { return value, valueErr }
	promptForMultipleValues = func(_, _ string, _ []string) ([]string, error) { return values, valuesErr }
}

func TestExecuteBulk_NonInteractiveNoSelectionErrors(t *testing.T) {
	withListStubs(t, twoStacks(), nil, nil, nil)
	// Multiple stacks => the stack picker is consulted first; simulate "no TTY".
	withPromptStubs(t, "", errUtils.ErrInteractiveModeNotAvailable, nil, nil)

	err := ExecuteBulk(context.Background(), &schema.ConfigAndStacksInfo{}, "up")
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrNoContainerComponentSelected)
}

func TestExecuteBulk_InteractiveSelectsSubset(t *testing.T) {
	withListStubs(t, twoStacks(), nil, nil, nil)
	// Stack "dev" is given, so only the component multi-select runs; pick "worker".
	withPromptStubs(t, "", nil, []string{"worker"}, nil)

	var calls []string
	withBulkExecutor(t, "up", func(_ context.Context, info *schema.ConfigAndStacksInfo) error {
		calls = append(calls, info.Stack+"/"+info.ComponentFromArg)
		return nil
	})

	require.NoError(t, ExecuteBulk(context.Background(), &schema.ConfigAndStacksInfo{Stack: "dev"}, "up"))
	assert.Equal(t, []string{"dev/worker"}, calls)
}

func TestExecuteBulk_InteractivePicksStackThenComponents(t *testing.T) {
	withListStubs(t, twoStacks(), nil, nil, nil)
	// No stack given and two stacks exist => stack picker returns "prod";
	// then the component multi-select returns "api".
	withPromptStubs(t, "prod", nil, []string{"api"}, nil)

	var calls []string
	withBulkExecutor(t, "up", func(_ context.Context, info *schema.ConfigAndStacksInfo) error {
		calls = append(calls, info.Stack+"/"+info.ComponentFromArg)
		return nil
	})

	require.NoError(t, ExecuteBulk(context.Background(), &schema.ConfigAndStacksInfo{}, "up"))
	assert.Equal(t, []string{"prod/api"}, calls)
}

func TestExecuteBulk_UserAbortPropagates(t *testing.T) {
	withListStubs(t, twoStacks(), nil, nil, nil)
	withPromptStubs(t, "", nil, nil, errUtils.ErrUserAborted)

	err := ExecuteBulk(context.Background(), &schema.ConfigAndStacksInfo{Stack: "dev"}, "up")
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrUserAborted)
}

func TestSelectTargetsInteractively_SingleStackSkipsStackPrompt(t *testing.T) {
	// Only one stack present => no stack prompt; components pre-selected (all).
	withPromptStubs(t, "", errUtils.ErrInteractiveModeNotAvailable /* would fail if called */, []string{"api"}, nil)
	rows := []instanceRow{{stack: "dev", component: "api"}}
	targets, err := selectTargetsInteractively(rows, &schema.ConfigAndStacksInfo{}, "up")
	require.NoError(t, err)
	require.Len(t, targets, 1)
	assert.Equal(t, instanceRow{stack: "dev", component: "api"}, targets[0])
}

func TestSelectTargetsInteractively_EmptyRows(t *testing.T) {
	targets, err := selectTargetsInteractively(nil, &schema.ConfigAndStacksInfo{}, "up")
	require.NoError(t, err)
	assert.Empty(t, targets)
}

func TestResolveBulkTargets_AllForwardsStack(t *testing.T) {
	var gotStack string
	origInit, origDescribe := initCliConfig, describeStacks
	t.Cleanup(func() { initCliConfig, describeStacks = origInit, origDescribe })
	initCliConfig = func(_ schema.ConfigAndStacksInfo, _ bool) (schema.AtmosConfiguration, error) {
		return schema.AtmosConfiguration{}, nil
	}
	describeStacks = func(_ *schema.AtmosConfiguration, stack string, _, _, _ []string, _, _, _, _ bool, _ []string, _ auth.AuthManager) (map[string]any, error) {
		gotStack = stack
		return twoStacks(), nil
	}

	targets, err := resolveBulkTargets(&schema.ConfigAndStacksInfo{All: true, Stack: "dev"}, "up")
	require.NoError(t, err)
	assert.Equal(t, "dev", gotStack)
	// --all returns every component from the described map (stub ignores the filter).
	require.Len(t, targets, 3)
	assert.Equal(t, "api", targets[0].component)
	assert.Equal(t, instanceRow{stack: "prod", component: "api", image: "api:prod"}, targets[2])
}
