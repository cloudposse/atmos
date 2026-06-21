package container

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/cloudposse/atmos/pkg/auth"
	"github.com/cloudposse/atmos/pkg/component"
	ctr "github.com/cloudposse/atmos/pkg/container"
	"github.com/cloudposse/atmos/pkg/schema"
)

// withStubs replaces the package seams so executor tests run without a real
// Atmos project or container runtime. The provided section becomes the resolved
// component section.
func withStubs(t *testing.T, section map[string]any, env []string, rt ctr.Runtime) {
	t.Helper()

	origInit, origProcess, origDetect := initCliConfig, processStacks, detectRuntime
	t.Cleanup(func() {
		initCliConfig, processStacks, detectRuntime = origInit, origProcess, origDetect
	})

	initCliConfig = func(_ schema.ConfigAndStacksInfo, _ bool) (schema.AtmosConfiguration, error) {
		return schema.AtmosConfiguration{}, nil
	}
	processStacks = func(_ *schema.AtmosConfiguration, info schema.ConfigAndStacksInfo, _, _, _ bool, _ []string, _ auth.AuthManager) (schema.ConfigAndStacksInfo, error) {
		info.ComponentSection = section
		info.ComponentEnvList = env
		return info, nil
	}
	detectRuntime = func(_ context.Context, _ string, _ bool) (ctr.Runtime, error) {
		return rt, nil
	}
}

func infoFor(component string) *schema.ConfigAndStacksInfo {
	return &schema.ConfigAndStacksInfo{ComponentFromArg: component, Stack: "dev"}
}

func TestExecuteUp_CreatesAndStarts(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	rt := NewMockRuntime(ctrl)

	section := map[string]any{
		"image": "localhost:5001/api:abc",
		"run":   map[string]any{"command": "./api", "ports": []any{map[string]any{"host": 8080, "container": 8080}}},
	}
	withStubs(t, section, []string{"PORT=8080"}, rt)

	gomock.InOrder(
		rt.EXPECT().List(gomock.Any(), ctr.DiscoveryFilter("dev", "container", "api")).Return([]ctr.Info{}, nil),
		rt.EXPECT().Create(gomock.Any(), gomock.Any()).
			DoAndReturn(func(_ context.Context, c *ctr.CreateConfig) (string, error) {
				assert.Equal(t, "atmos-dev-container-api", c.Name)
				assert.Equal(t, []string{"./api"}, c.Command)
				require.Len(t, c.Ports, 1)
				assert.Equal(t, "8080", c.Env["PORT"])
				assert.Equal(t, "dev/container/api", c.Labels[ctr.LabelInstance])
				return "cid", nil
			}),
		rt.EXPECT().Start(gomock.Any(), "cid").Return(nil),
	)

	require.NoError(t, ExecuteUp(context.Background(), infoFor("api")))
}

func TestExecuteUp_DryRun(t *testing.T) {
	section := map[string]any{"image": "alpine"}
	withStubs(t, section, nil, nil) // nil runtime: must not be used in dry-run.

	info := infoFor("api")
	info.DryRun = true
	require.NoError(t, ExecuteUp(context.Background(), info))
}

func TestExecuteUp_RequiresImage(t *testing.T) {
	withStubs(t, map[string]any{}, nil, nil)
	require.Error(t, ExecuteUp(context.Background(), infoFor("api")))
}

func TestExecuteDown_StopsAndRemoves(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	rt := NewMockRuntime(ctrl)
	withStubs(t, map[string]any{"image": "alpine"}, nil, rt)

	gomock.InOrder(
		rt.EXPECT().List(gomock.Any(), ctr.DiscoveryFilter("dev", "container", "api")).
			Return([]ctr.Info{{ID: "cid", Status: "running", Labels: ctr.InstanceLabels("dev", "container", "api")}}, nil),
		rt.EXPECT().Stop(gomock.Any(), "cid", defaultStopTimeout).Return(nil),
		rt.EXPECT().Remove(gomock.Any(), "cid", true).Return(nil),
	)

	require.NoError(t, ExecuteDown(context.Background(), infoFor("api")))
}

func TestExecuteBuild_NoBuildIsError(t *testing.T) {
	withStubs(t, map[string]any{"image": "alpine"}, nil, nil)
	require.Error(t, ExecuteBuild(context.Background(), infoFor("api")))
}

func TestExecuteBuild_DryRun(t *testing.T) {
	section := map[string]any{
		"build": map[string]any{"context": "app", "tags": []any{"img:1"}},
	}
	withStubs(t, section, nil, nil)

	info := infoFor("api")
	info.DryRun = true
	require.NoError(t, ExecuteBuild(context.Background(), info))
}

func TestExecuteBuild_CallsRuntime(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	rt := NewMockRuntime(ctrl)
	section := map[string]any{
		"build": map[string]any{"context": "app", "dockerfile": "Dockerfile", "tags": []any{"img:1"}},
	}
	withStubs(t, section, nil, rt)

	rt.EXPECT().Build(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, b *ctr.BuildConfig) error {
			assert.Equal(t, "app", b.Context)
			assert.Equal(t, []string{"img:1"}, b.Tags)
			return nil
		})

	require.NoError(t, ExecuteBuild(context.Background(), infoFor("api")))
}

func TestExecutePush_CallsRuntime(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	rt := NewMockRuntime(ctrl)
	withStubs(t, map[string]any{"image": "img:1"}, nil, rt)

	rt.EXPECT().Push(gomock.Any(), "img:1").Return(&ctr.PushResult{Image: "img:1"}, nil)
	require.NoError(t, ExecutePush(context.Background(), infoFor("api")))
}

func TestExecutePull_RequiresImage(t *testing.T) {
	withStubs(t, map[string]any{}, nil, nil)
	require.Error(t, ExecutePull(context.Background(), infoFor("api")))
}

func TestExecuteRun_RequiresCommand(t *testing.T) {
	withStubs(t, map[string]any{"image": "alpine"}, nil, nil)
	require.Error(t, ExecuteRun(context.Background(), infoFor("api")))
}

func TestExecutePs_NotRunning(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	rt := NewMockRuntime(ctrl)
	withStubs(t, map[string]any{"image": "alpine"}, nil, rt)

	rt.EXPECT().List(gomock.Any(), ctr.DiscoveryFilter("dev", "container", "api")).Return([]ctr.Info{}, nil)
	require.NoError(t, ExecutePs(context.Background(), infoFor("api")))
}

func TestExecuteStop_NotFoundIsError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	rt := NewMockRuntime(ctrl)
	withStubs(t, map[string]any{"image": "alpine"}, nil, rt)

	rt.EXPECT().List(gomock.Any(), ctr.DiscoveryFilter("dev", "container", "api")).Return([]ctr.Info{}, nil)
	require.Error(t, ExecuteStop(context.Background(), infoFor("api")))
}

func TestExecuteExec_RunsCommand(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	rt := NewMockRuntime(ctrl)
	withStubs(t, map[string]any{"image": "alpine"}, nil, rt)

	gomock.InOrder(
		rt.EXPECT().List(gomock.Any(), ctr.DiscoveryFilter("dev", "container", "api")).
			Return([]ctr.Info{{ID: "cid", Status: "running", Labels: ctr.InstanceLabels("dev", "container", "api")}}, nil),
		rt.EXPECT().Exec(gomock.Any(), "cid", []string{"echo", "hi"}, gomock.Any()).Return(nil),
	)

	require.NoError(t, ExecuteExec(context.Background(), infoFor("api"), []string{"echo", "hi"}))
}

func runningInfo() []ctr.Info {
	return []ctr.Info{{ID: "cid", Status: "running", Labels: ctr.InstanceLabels("dev", "container", "api")}}
}

func TestExecuteLogs_Streams(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	rt := NewMockRuntime(ctrl)
	withStubs(t, map[string]any{"image": "alpine"}, nil, rt)

	gomock.InOrder(
		rt.EXPECT().List(gomock.Any(), ctr.DiscoveryFilter("dev", "container", "api")).Return(runningInfo(), nil),
		rt.EXPECT().Logs(gomock.Any(), "cid", false, "all", nil, nil).Return(nil),
	)
	require.NoError(t, ExecuteLogs(context.Background(), infoFor("api")))
}

func TestExecuteRestart_StopsThenStarts(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	rt := NewMockRuntime(ctrl)
	withStubs(t, map[string]any{"image": "alpine"}, nil, rt)

	gomock.InOrder(
		rt.EXPECT().List(gomock.Any(), ctr.DiscoveryFilter("dev", "container", "api")).Return(runningInfo(), nil),
		rt.EXPECT().Stop(gomock.Any(), "cid", defaultStopTimeout).Return(nil),
		rt.EXPECT().Start(gomock.Any(), "cid").Return(nil),
	)
	require.NoError(t, ExecuteRestart(context.Background(), infoFor("api")))
}

func TestExecuteRm_Removes(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	rt := NewMockRuntime(ctrl)
	withStubs(t, map[string]any{"image": "alpine"}, nil, rt)

	gomock.InOrder(
		rt.EXPECT().List(gomock.Any(), ctr.DiscoveryFilter("dev", "container", "api")).Return(runningInfo(), nil),
		rt.EXPECT().Remove(gomock.Any(), "cid", true).Return(nil),
	)
	require.NoError(t, ExecuteRm(context.Background(), infoFor("api")))
}

func TestExecuteStop_Stops(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	rt := NewMockRuntime(ctrl)
	withStubs(t, map[string]any{"image": "alpine"}, nil, rt)

	gomock.InOrder(
		rt.EXPECT().List(gomock.Any(), ctr.DiscoveryFilter("dev", "container", "api")).Return(runningInfo(), nil),
		rt.EXPECT().Stop(gomock.Any(), "cid", defaultStopTimeout).Return(nil),
	)
	require.NoError(t, ExecuteStop(context.Background(), infoFor("api")))
}

func TestExecutePs_Running(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	rt := NewMockRuntime(ctrl)
	withStubs(t, map[string]any{"image": "alpine"}, nil, rt)

	rt.EXPECT().List(gomock.Any(), ctr.DiscoveryFilter("dev", "container", "api")).Return(runningInfo(), nil)
	require.NoError(t, ExecutePs(context.Background(), infoFor("api")))
}

func TestExecutePull_CallsRuntime(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	rt := NewMockRuntime(ctrl)
	withStubs(t, map[string]any{"image": "img:1"}, nil, rt)

	rt.EXPECT().Pull(gomock.Any(), "img:1").Return(nil)
	require.NoError(t, ExecutePull(context.Background(), infoFor("api")))
}

func TestExecutePush_DryRun(t *testing.T) {
	withStubs(t, map[string]any{"image": "img:1"}, nil, nil)
	info := infoFor("api")
	info.DryRun = true
	require.NoError(t, ExecutePush(context.Background(), info))
}

func TestExecutePull_DryRun(t *testing.T) {
	withStubs(t, map[string]any{"image": "img:1"}, nil, nil)
	info := infoFor("api")
	info.DryRun = true
	require.NoError(t, ExecutePull(context.Background(), info))
}

func TestExecuteRun_DryRun(t *testing.T) {
	section := map[string]any{"image": "alpine", "run": map[string]any{"command": "echo hi"}}
	withStubs(t, section, nil, nil)
	info := infoFor("api")
	info.DryRun = true
	require.NoError(t, ExecuteRun(context.Background(), info))
}

func TestProviderExecute_DispatchesToVerbs(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	rt := NewMockRuntime(ctrl)
	withStubs(t, map[string]any{"image": "alpine"}, nil, rt)

	rt.EXPECT().List(gomock.Any(), ctr.DiscoveryFilter("dev", "container", "api")).Return([]ctr.Info{}, nil)

	p := &ContainerComponentProvider{}
	err := p.Execute(&component.ExecutionContext{
		SubCommand:          "ps",
		ConfigAndStacksInfo: *infoFor("api"),
	})
	require.NoError(t, err)
}

func TestPrepare_InitConfigError(t *testing.T) {
	orig := initCliConfig
	t.Cleanup(func() { initCliConfig = orig })
	initCliConfig = func(_ schema.ConfigAndStacksInfo, _ bool) (schema.AtmosConfiguration, error) {
		return schema.AtmosConfiguration{}, assert.AnError
	}
	_, err := prepare(infoFor("api"))
	require.Error(t, err)
}

func TestPrepare_ProcessStacksError(t *testing.T) {
	origInit, origProcess := initCliConfig, processStacks
	t.Cleanup(func() { initCliConfig, processStacks = origInit, origProcess })
	initCliConfig = func(_ schema.ConfigAndStacksInfo, _ bool) (schema.AtmosConfiguration, error) {
		return schema.AtmosConfiguration{}, nil
	}
	processStacks = func(_ *schema.AtmosConfiguration, info schema.ConfigAndStacksInfo, _, _, _ bool, _ []string, _ auth.AuthManager) (schema.ConfigAndStacksInfo, error) {
		return info, assert.AnError
	}
	_, err := prepare(infoFor("api"))
	require.Error(t, err)
}

func TestPrepare_RejectsUnknownComposition(t *testing.T) {
	origInit, origProcess := initCliConfig, processStacks
	t.Cleanup(func() { initCliConfig, processStacks = origInit, origProcess })

	// Config declares composition "storefront" with services [api]; the component
	// "worker" claims membership but is not declared — must hard-error.
	initCliConfig = func(_ schema.ConfigAndStacksInfo, _ bool) (schema.AtmosConfiguration, error) {
		return schema.AtmosConfiguration{
			Compositions: map[string]schema.Composition{
				"storefront": {Services: []string{"api"}},
			},
		}, nil
	}
	processStacks = func(_ *schema.AtmosConfiguration, info schema.ConfigAndStacksInfo, _, _, _ bool, _ []string, _ auth.AuthManager) (schema.ConfigAndStacksInfo, error) {
		info.ComponentSection = map[string]any{
			"composition": "storefront",
			"image":       "alpine",
		}
		return info, nil
	}

	_, err := prepare(infoFor("worker"))
	require.Error(t, err)
}

func TestEnvListToMap(t *testing.T) {
	// Container application env comes from the component `env:` section (resolved
	// with secrets) — there is no run-level env.
	env := envListToMap([]string{"A=1", "B=secret", "C=3"})
	assert.Equal(t, "1", env["A"])
	assert.Equal(t, "secret", env["B"])
	assert.Equal(t, "3", env["C"])
}

func TestDefaultStopTimeoutValue(t *testing.T) {
	assert.Equal(t, 10*time.Second, defaultStopTimeout)
}
