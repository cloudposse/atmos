package container

import (
	"context"
	"errors"
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	ctr "github.com/cloudposse/atmos/pkg/container"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestContainerRef(t *testing.T) {
	assert.Empty(t, containerRef(nil))
	assert.Equal(t, "cid", containerRef(&ctr.Info{ID: "cid", Name: "name"})) // ID precedence
	assert.Equal(t, "name", containerRef(&ctr.Info{Name: "name"}))           // Name fallback
}

func TestIsAbstractSection(t *testing.T) {
	assert.True(t, isAbstractSection(map[string]any{"metadata": map[string]any{"type": "abstract"}}))
	assert.False(t, isAbstractSection(map[string]any{"metadata": map[string]any{"type": "real"}}))
	assert.False(t, isAbstractSection(map[string]any{"metadata": "bad"}))
	assert.False(t, isAbstractSection(map[string]any{}))
}

func TestRequireImage(t *testing.T) {
	r := &resolved{spec: ContainerSpec{Image: "alpine"}, component: "api"}
	image, err := r.requireImage()
	require.NoError(t, err)
	assert.Equal(t, "alpine", image)

	_, err = (&resolved{spec: ContainerSpec{Image: "  "}, component: "api"}).requireImage()
	require.Error(t, err)
}

func TestRunUser(t *testing.T) {
	assert.Empty(t, (&resolved{spec: ContainerSpec{}}).runUser())
	assert.Equal(t, "app", (&resolved{spec: ContainerSpec{Run: &schema.ContainerRunStep{User: "app"}}}).runUser())
}

func TestMapToEnvList_RoundTrip(t *testing.T) {
	env := envListToMap([]string{"A=1", "B=2", "C=secret"})
	list := mapToEnvList(env)
	sort.Strings(list)
	assert.Equal(t, []string{"A=1", "B=2", "C=secret"}, list)
}

func TestEnsureImage_NoBuild(t *testing.T) {
	// No build configured: nothing to do, no runtime calls.
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	rt := NewMockRuntime(ctrl)

	r := &resolved{spec: ContainerSpec{Image: "alpine"}, component: "api"}
	require.NoError(t, r.ensureImage(context.Background(), rt, "alpine"))
}

func buildResolved() *resolved {
	return &resolved{
		spec: ContainerSpec{
			Image: "img:1",
			Build: &schema.ContainerBuildStep{Context: "app", Tags: []string{"img:1"}},
		},
		component: "api",
	}
}

func TestEnsureImage_AlreadyPresent(t *testing.T) {
	// Image already present locally: no build is triggered.
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	rt := NewMockRuntime(ctrl)

	rt.EXPECT().ImageInspect(gomock.Any(), "img:1").Return(&ctr.ImageInfo{}, nil)

	require.NoError(t, buildResolved().ensureImage(context.Background(), rt, "img:1"))
}

func TestEnsureImage_MissingThenBuilds(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	rt := NewMockRuntime(ctrl)

	gomock.InOrder(
		rt.EXPECT().ImageInspect(gomock.Any(), "img:1").Return(nil, errors.New("no such image: img:1")),
		rt.EXPECT().Build(gomock.Any(), gomock.Any()).Return(nil),
	)

	require.NoError(t, buildResolved().ensureImage(context.Background(), rt, "img:1"))
}

func TestEnsureImage_InspectNonMissingErrorSurfaces(t *testing.T) {
	// A non-missing inspect error must surface as-is — building would mask it.
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	rt := NewMockRuntime(ctrl)

	rt.EXPECT().ImageInspect(gomock.Any(), "img:1").Return(nil, assert.AnError)

	require.Error(t, buildResolved().ensureImage(context.Background(), rt, "img:1"))
}

func TestEnsureImage_BuildFails(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	rt := NewMockRuntime(ctrl)

	gomock.InOrder(
		rt.EXPECT().ImageInspect(gomock.Any(), "img:1").Return(nil, errors.New("image not found")),
		rt.EXPECT().Build(gomock.Any(), gomock.Any()).Return(assert.AnError),
	)

	require.Error(t, buildResolved().ensureImage(context.Background(), rt, "img:1"))
}

func TestExecuteLogs_DiscoverListError(t *testing.T) {
	// A List error during discovery is surfaced (covers the discover error path).
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	rt := NewMockRuntime(ctrl)
	withStubs(t, map[string]any{"image": "alpine"}, nil, rt)

	rt.EXPECT().List(gomock.Any(), gomock.Any()).Return(nil, assert.AnError)
	require.Error(t, ExecuteLogs(context.Background(), infoFor("api")))
}

func TestExecuteRun_RunsEphemeral(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	rt := NewMockRuntime(ctrl)
	section := map[string]any{"image": "alpine", "run": map[string]any{"command": "echo hi"}}
	withStubs(t, section, nil, rt)

	gomock.InOrder(
		rt.EXPECT().Create(gomock.Any(), gomock.Any()).Return("cid", nil),
		rt.EXPECT().Start(gomock.Any(), "cid").Return(nil),
		rt.EXPECT().Exec(gomock.Any(), "cid", []string{"/bin/sh", "-lc", "echo hi"}, gomock.Any()).Return(nil),
		rt.EXPECT().Remove(gomock.Any(), "cid", true).Return(nil), // CleanupAlways default
	)
	require.NoError(t, ExecuteRun(context.Background(), infoFor("api")))
}

func TestExecutePush_RuntimeError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	rt := NewMockRuntime(ctrl)
	withStubs(t, map[string]any{"image": "img:1"}, nil, rt)

	rt.EXPECT().Push(gomock.Any(), "img:1").Return(nil, assert.AnError)
	require.Error(t, ExecutePush(context.Background(), infoFor("api")))
}

func TestExecutePull_RuntimeError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	rt := NewMockRuntime(ctrl)
	withStubs(t, map[string]any{"image": "img:1"}, nil, rt)

	rt.EXPECT().Pull(gomock.Any(), "img:1").Return(assert.AnError)
	require.Error(t, ExecutePull(context.Background(), infoFor("api")))
}

func TestExecuteBuild_RuntimeError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	rt := NewMockRuntime(ctrl)
	section := map[string]any{"build": map[string]any{"context": "app", "tags": []any{"img:1"}}}
	withStubs(t, section, nil, rt)

	rt.EXPECT().Build(gomock.Any(), gomock.Any()).Return(assert.AnError)
	require.Error(t, ExecuteBuild(context.Background(), infoFor("api")))
}

func TestExecuteRestart_StartError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	rt := NewMockRuntime(ctrl)
	withStubs(t, map[string]any{"image": "alpine"}, nil, rt)

	gomock.InOrder(
		rt.EXPECT().List(gomock.Any(), ctr.DiscoveryFilter("dev", "container", "api")).Return(runningInfo(), nil),
		rt.EXPECT().Stop(gomock.Any(), "cid", defaultStopTimeout).Return(nil),
		rt.EXPECT().Start(gomock.Any(), "cid").Return(assert.AnError),
	)
	require.Error(t, ExecuteRestart(context.Background(), infoFor("api")))
}

func TestExecuteRm_RuntimeError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	rt := NewMockRuntime(ctrl)
	withStubs(t, map[string]any{"image": "alpine"}, nil, rt)

	gomock.InOrder(
		rt.EXPECT().List(gomock.Any(), ctr.DiscoveryFilter("dev", "container", "api")).Return(runningInfo(), nil),
		rt.EXPECT().Remove(gomock.Any(), "cid", true).Return(assert.AnError),
	)
	require.Error(t, ExecuteRm(context.Background(), infoFor("api")))
}
