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
	"github.com/cloudposse/atmos/pkg/data"
	iolib "github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/pkg/schema"
)

// initListTestIO initializes the data/UI writers so rendering functions
// (renderInstanceTable) do not panic on an uninitialized data package.
func initListTestIO(t *testing.T) {
	t.Helper()
	ioCtx, err := iolib.NewContext()
	require.NoError(t, err)
	data.InitWriter(ioCtx)
	t.Cleanup(data.Reset)
}

// withListStubs replaces the seams ExecuteList depends on so it runs without a
// real Atmos project or container runtime: initCliConfig returns an empty config,
// describeStacks returns the provided map/error, and detectRuntime returns rt/err.
func withListStubs(t *testing.T, stacksMap map[string]any, describeErr error, rt ctr.Runtime, detectErr error) {
	t.Helper()

	origInit, origDescribe, origDetect := initCliConfig, describeStacks, detectRuntime
	t.Cleanup(func() {
		initCliConfig, describeStacks, detectRuntime = origInit, origDescribe, origDetect
	})

	initCliConfig = func(_ schema.ConfigAndStacksInfo, _ bool) (schema.AtmosConfiguration, error) {
		return schema.AtmosConfiguration{}, nil
	}
	describeStacks = func(_ *schema.AtmosConfiguration, _ string, _, _, _ []string, _, _, _, _ bool, _ []string, _ auth.AuthManager) (map[string]any, error) {
		return stacksMap, describeErr
	}
	detectRuntime = func(_ context.Context, _ string, _ bool) (ctr.Runtime, error) {
		return rt, detectErr
	}
}

// containerStack builds a describe-stacks stack entry whose container components
// each carry the given image. Components prefixed with "abstract:" are marked
// abstract blueprints.
func containerStack(components map[string]string) map[string]any {
	typeMap := map[string]any{}
	for component, image := range components {
		comp := map[string]any{"image": image}
		typeMap[component] = comp
	}
	return map[string]any{
		cfg.ComponentsSectionName: map[string]any{
			cfg.ContainerComponentType: typeMap,
		},
	}
}

func TestCollectContainerInstances(t *testing.T) {
	// Two stacks, with an abstract blueprint that must be skipped, plus malformed
	// entries that must not panic or produce rows.
	devContainers := map[string]any{
		"api":    map[string]any{"image": "api:dev"},
		"worker": map[string]any{"image": "worker:dev"},
		"base":   map[string]any{"image": "base:dev", "metadata": map[string]any{"type": "abstract"}},
	}
	stacksMap := map[string]any{
		"dev": map[string]any{
			cfg.ComponentsSectionName: map[string]any{cfg.ContainerComponentType: devContainers},
		},
		"prod": containerStack(map[string]string{"api": "api:prod"}),
		// Malformed entries: ignored without panic.
		"bad-stack": "not-a-map",
		"no-comps":  map[string]any{"foo": "bar"},
		"comps-bad": map[string]any{cfg.ComponentsSectionName: "bad"},
		"no-cont":   map[string]any{cfg.ComponentsSectionName: map[string]any{"terraform": map[string]any{}}},
		"cont-bad":  map[string]any{cfg.ComponentsSectionName: map[string]any{cfg.ContainerComponentType: "bad"}},
	}

	rows := collectContainerInstances(stacksMap)

	// Abstract "base" is excluded; remaining sorted by stack then component.
	require.Len(t, rows, 3)
	assert.Equal(t, instanceRow{stack: "dev", component: "api", image: "api:dev", tags: []string{}, labels: map[string]string{}}, rows[0])
	assert.Equal(t, instanceRow{stack: "dev", component: "worker", image: "worker:dev", tags: []string{}, labels: map[string]string{}}, rows[1])
	assert.Equal(t, instanceRow{stack: "prod", component: "api", image: "api:prod", tags: []string{}, labels: map[string]string{}}, rows[2])
}

func TestCollectContainerInstances_Empty(t *testing.T) {
	assert.Nil(t, collectContainerInstances(map[string]any{}))
}

func TestIsAbstractComponent(t *testing.T) {
	cases := []struct {
		name     string
		compData any
		want     bool
	}{
		{"abstract", map[string]any{"metadata": map[string]any{"type": "abstract"}}, true},
		{"concrete type", map[string]any{"metadata": map[string]any{"type": "real"}}, false},
		{"no type", map[string]any{"metadata": map[string]any{}}, false},
		{"no metadata", map[string]any{"image": "alpine"}, false},
		{"metadata not a map", map[string]any{"metadata": "bad"}, false},
		{"not a map", "abstract", false},
		{"nil", nil, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, isAbstractComponent(tc.compData))
		})
	}
}

func TestImageFromComponent(t *testing.T) {
	assert.Equal(t, "alpine", imageFromComponent(map[string]any{"image": "alpine"}))
	assert.Empty(t, imageFromComponent(map[string]any{"other": "x"}))
	assert.Empty(t, imageFromComponent(map[string]any{"image": 42})) // non-string
	assert.Empty(t, imageFromComponent("not-a-map"))
	assert.Empty(t, imageFromComponent(nil))
}

func TestEmptyListOrError(t *testing.T) {
	// Project-shaped errors degrade to a clean empty listing (nil error).
	assert.NoError(t, emptyListOrError(errUtils.ErrFailedToFindImport))
	assert.NoError(t, emptyListOrError(errUtils.ErrNoStacksFound))

	// Any other error is propagated unchanged.
	err := emptyListOrError(assert.AnError)
	require.Error(t, err)
	assert.ErrorIs(t, err, assert.AnError)
}

func TestStatusDot(t *testing.T) {
	// Non-TTY renders a space so machine consumers read the STATUS column.
	assert.Equal(t, " ", statusDot(statusRunning, false))
	assert.Equal(t, " ", statusDot(statusStopped, false))

	// On a TTY both render the dot glyph (color codes are environment-dependent
	// and stripped in a no-color test environment, so only the glyph is asserted).
	assert.Contains(t, statusDot(statusRunning, true), runningDot)
	assert.Contains(t, statusDot(statusStopped, true), runningDot)
}

func TestRenderInstanceTable(t *testing.T) {
	initListTestIO(t)
	rows := []instanceRow{
		{stack: "dev", component: "api", image: "api:dev", status: statusRunning, running: true},
		{stack: "prod", component: "worker", image: "worker:prod", status: statusStopped},
	}
	assert.NoError(t, renderInstanceTable(rows))
	assert.NoError(t, renderInstanceTable(nil))
}

func TestAnnotateRunningState_NilRuntime(t *testing.T) {
	rows := []instanceRow{{stack: "dev", component: "api"}}
	annotateRunningState(context.Background(), nil, rows)
	assert.Equal(t, statusUnknown, rows[0].status)
	assert.False(t, rows[0].running)
}

func TestAnnotateRunningState_Running(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	rt := NewMockRuntime(ctrl)

	rt.EXPECT().List(gomock.Any(), ctr.DiscoveryFilter("dev", "container", "api")).
		Return([]ctr.Info{{ID: "cid", Status: "running", Labels: ctr.InstanceLabels("dev", "container", "api")}}, nil)

	rows := []instanceRow{{stack: "dev", component: "api"}}
	annotateRunningState(context.Background(), rt, rows)
	assert.Equal(t, statusRunning, rows[0].status)
	assert.True(t, rows[0].running)
}

func TestAnnotateRunningState_Stopped(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	rt := NewMockRuntime(ctrl)

	rt.EXPECT().List(gomock.Any(), ctr.DiscoveryFilter("dev", "container", "api")).
		Return([]ctr.Info{{ID: "cid", Status: "exited", Labels: ctr.InstanceLabels("dev", "container", "api")}}, nil)

	rows := []instanceRow{{stack: "dev", component: "api"}}
	annotateRunningState(context.Background(), rt, rows)
	assert.Equal(t, statusStopped, rows[0].status)
	assert.False(t, rows[0].running)
}

func TestAnnotateRunningState_ListError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	rt := NewMockRuntime(ctrl)

	rt.EXPECT().List(gomock.Any(), gomock.Any()).Return(nil, assert.AnError)

	rows := []instanceRow{{stack: "dev", component: "api"}}
	annotateRunningState(context.Background(), rt, rows)
	assert.Equal(t, statusUnknown, rows[0].status)
}

func TestExecuteList_NoComponents(t *testing.T) {
	withListStubs(t, map[string]any{}, nil, nil, nil)
	require.NoError(t, ExecuteList(context.Background(), &schema.ConfigAndStacksInfo{}))
}

func TestExecuteList_InitConfigError(t *testing.T) {
	withListStubs(t, nil, nil, nil, nil)
	// Override initCliConfig to fail with a generic error (propagated as-is).
	orig := initCliConfig
	t.Cleanup(func() { initCliConfig = orig })
	initCliConfig = func(_ schema.ConfigAndStacksInfo, _ bool) (schema.AtmosConfiguration, error) {
		return schema.AtmosConfiguration{}, assert.AnError
	}
	require.ErrorIs(t, ExecuteList(context.Background(), &schema.ConfigAndStacksInfo{}), assert.AnError)
}

func TestExecuteList_NoStacksDegradesToEmpty(t *testing.T) {
	withListStubs(t, nil, errUtils.ErrNoStacksFound, nil, nil)
	require.NoError(t, ExecuteList(context.Background(), &schema.ConfigAndStacksInfo{}))
}

func TestExecuteList_RendersWithRuntime(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	rt := NewMockRuntime(ctrl)

	initListTestIO(t)
	stacksMap := map[string]any{"dev": containerStack(map[string]string{"api": "api:dev"})}
	withListStubs(t, stacksMap, nil, rt, nil)

	rt.EXPECT().List(gomock.Any(), ctr.DiscoveryFilter("dev", "container", "api")).
		Return([]ctr.Info{{ID: "cid", Status: "running", Labels: ctr.InstanceLabels("dev", "container", "api")}}, nil)

	require.NoError(t, ExecuteList(context.Background(), &schema.ConfigAndStacksInfo{}))
}

func TestExecuteList_RuntimeDetectErrorRendersUnknown(t *testing.T) {
	// When runtime detection fails, rows still render (status unknown), no error.
	initListTestIO(t)
	stacksMap := map[string]any{"dev": containerStack(map[string]string{"api": "api:dev"})}
	withListStubs(t, stacksMap, nil, nil, assert.AnError)
	require.NoError(t, ExecuteList(context.Background(), &schema.ConfigAndStacksInfo{}))
}
