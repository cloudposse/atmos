package container

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth"
	cfg "github.com/cloudposse/atmos/pkg/config"
	ctr "github.com/cloudposse/atmos/pkg/container"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/signals"
)

func TestLogsOptionsFrom(t *testing.T) {
	// Defaults: no follow, tail "all".
	def := logsOptionsFrom(nil)
	assert.False(t, def.follow)
	assert.Equal(t, "all", def.tail)

	// Explicit values are honored.
	got := logsOptionsFrom(map[string]any{"follow": true, "tail": "100"})
	assert.True(t, got.follow)
	assert.Equal(t, "100", got.tail)

	// Empty tail falls back to the default.
	assert.Equal(t, "all", logsOptionsFrom(map[string]any{"tail": ""}).tail)
}

// withLogsStubs stubs every seam the logs paths touch: config init, stack
// processing (a fixed image section per component), describe-stacks (the bulk
// target source), and runtime detection (the given mock).
func withLogsStubs(t *testing.T, stacksMap map[string]any, rt ctr.Runtime) {
	t.Helper()
	origInit, origProcess, origDescribe, origDetect := initCliConfig, processStacks, describeStacks, detectRuntime
	t.Cleanup(func() {
		initCliConfig, processStacks, describeStacks, detectRuntime = origInit, origProcess, origDescribe, origDetect
	})
	initCliConfig = func(_ schema.ConfigAndStacksInfo, _ bool) (schema.AtmosConfiguration, error) {
		return schema.AtmosConfiguration{}, nil
	}
	processStacks = func(_ *schema.AtmosConfiguration, info schema.ConfigAndStacksInfo, _, _, _ bool, _ []string, _ auth.AuthManager) (schema.ConfigAndStacksInfo, error) {
		info.ComponentSection = map[string]any{"image": "alpine"}
		return info, nil
	}
	describeStacks = func(_ *schema.AtmosConfiguration, _ string, _, _, _ []string, _, _, _, _ bool, _ []string, _ auth.AuthManager) (map[string]any, error) {
		return stacksMap, nil
	}
	detectRuntime = func(_ context.Context, _ string, _ bool) (ctr.Runtime, error) {
		return rt, nil
	}
}

// runningInfoFor builds a running container Info with the canonical instance
// label for the given component in stack "dev" (so FindInstance matches it).
func runningInfoFor(component string) []ctr.Info {
	return []ctr.Info{{
		ID:     component + "-cid",
		Status: "running",
		Labels: ctr.InstanceLabels("dev", cfg.ContainerComponentType, component),
	}}
}

func TestExecuteLogsWithOptions_SingleFollow(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	rt := NewMockRuntime(ctrl)
	withStubs(t, map[string]any{"image": "alpine"}, nil, rt)

	gomock.InOrder(
		rt.EXPECT().List(gomock.Any(), ctr.DiscoveryFilter("dev", "container", "api")).Return(runningInfo(), nil),
		rt.EXPECT().Logs(gomock.Any(), "cid", true, "100", nil, nil).Return(nil),
	)

	info := infoFor("api")
	require.NoError(t, ExecuteLogsWithOptions(context.Background(), info, logsOptions{follow: true, tail: "100"}))
}

func TestExecuteLogsWithOptions_ComponentWithAllRejected(t *testing.T) {
	err := ExecuteLogsWithOptions(context.Background(),
		&schema.ConfigAndStacksInfo{All: true, ComponentFromArg: "api"}, logsOptions{})
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrContainerComponentWithAll)
}

// oneStackTwoComps is a describe-stacks map with stack "dev" holding api+worker.
func oneStackTwoComps() map[string]any {
	return map[string]any{
		"dev": map[string]any{
			cfg.ComponentsSectionName: map[string]any{
				cfg.ContainerComponentType: map[string]any{
					"api":    map[string]any{"image": "api:dev"},
					"worker": map[string]any{"image": "worker:dev"},
				},
			},
		},
	}
}

func TestExecuteLogsWithOptions_AllSequential(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	rt := NewMockRuntime(ctrl)
	withLogsStubs(t, oneStackTwoComps(), rt)

	// Non-follow: each component is streamed in turn with nil (unprefixed) writers.
	rt.EXPECT().List(gomock.Any(), ctr.DiscoveryFilter("dev", "container", "api")).Return(runningInfoFor("api"), nil)
	rt.EXPECT().Logs(gomock.Any(), "api-cid", false, "all", nil, nil).Return(nil)
	rt.EXPECT().List(gomock.Any(), ctr.DiscoveryFilter("dev", "container", "worker")).Return(runningInfoFor("worker"), nil)
	rt.EXPECT().Logs(gomock.Any(), "worker-cid", false, "all", nil, nil).Return(nil)

	err := ExecuteLogsWithOptions(context.Background(),
		&schema.ConfigAndStacksInfo{All: true, Stack: "dev"}, logsOptions{tail: "all"})
	require.NoError(t, err)
}

func TestExecuteLogsWithOptions_AllFollowConcurrent(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	rt := NewMockRuntime(ctrl)
	withLogsStubs(t, oneStackTwoComps(), rt)

	// Follow: each component is discovered, then followed with a (non-nil)
	// per-component prefixed writer. Order across goroutines is not guaranteed.
	rt.EXPECT().List(gomock.Any(), ctr.DiscoveryFilter("dev", "container", "api")).Return(runningInfoFor("api"), nil)
	rt.EXPECT().List(gomock.Any(), ctr.DiscoveryFilter("dev", "container", "worker")).Return(runningInfoFor("worker"), nil)
	rt.EXPECT().Logs(gomock.Any(), "api-cid", true, "all", gomock.Not(gomock.Nil()), gomock.Not(gomock.Nil())).Return(nil)
	rt.EXPECT().Logs(gomock.Any(), "worker-cid", true, "all", gomock.Not(gomock.Nil()), gomock.Not(gomock.Nil())).Return(nil)

	err := ExecuteLogsWithOptions(context.Background(),
		&schema.ConfigAndStacksInfo{All: true, Stack: "dev"}, logsOptions{follow: true, tail: "all"})
	require.NoError(t, err)
}

func TestMaxComponentNameLenAndCenterLabel(t *testing.T) {
	targets := []instanceRow{
		{stack: "dev", component: "api"},
		{stack: "dev", component: "worker"},
		{stack: "dev", component: "db"},
	}
	width := maxComponentNameLen(targets)
	assert.Equal(t, 6, width) // "worker"

	// Labels are uppercased and centered within the common width.
	assert.Equal(t, " API  ", centerLabel("api", width)) // pad 3 → left 1, right 2.
	assert.Equal(t, "  DB  ", centerLabel("db", width))  // pad 4 → left 2, right 2.
	assert.Equal(t, "WORKER", centerLabel("worker", width))
	assert.Equal(t, "TOOLONG", centerLabel("toolong", width)) // already wider: uppercased, unchanged width.

	assert.Equal(t, 0, maxComponentNameLen(nil))
}

func TestFollowContextSuspendsInterruptExit(t *testing.T) {
	// Regression: while following, Atmos's global interrupt-exit must be
	// suspended so Ctrl-C is handled here (graceful stop) instead of the main
	// signal handler exiting 130. The suspension must be released by stop().
	require.False(t, signals.InterruptExitSuspended())

	ctx, stop := followContext(context.Background())
	require.NotNil(t, ctx)
	assert.True(t, signals.InterruptExitSuspended(), "interrupt-exit should be suspended while following")

	stop()
	assert.False(t, signals.InterruptExitSuspended(), "interrupt-exit suspension must be released on stop")
}

func TestExecuteLogsWithOptions_NoComponentsIsNoop(t *testing.T) {
	rt := NewMockRuntime(gomock.NewController(t))
	withLogsStubs(t, map[string]any{}, rt)
	require.NoError(t, ExecuteLogsWithOptions(context.Background(),
		&schema.ConfigAndStacksInfo{All: true}, logsOptions{}))
}
