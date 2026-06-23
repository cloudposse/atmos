package container

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
)

// errMissingImage is recognized by IsImageMissingError (substring "no such image").
var errMissingImage = errors.New("no such image: img:1")

func namedConfig(pullPolicy string) *NamedConfig {
	return &NamedConfig{
		Stack: "dev", ComponentType: "container", Component: "api",
		Image: "img:1", PullPolicy: pullPolicy,
	}
}

func TestCreateNamedContainer_PullAlwaysThenCreate(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	rt := NewMockRuntime(ctrl)

	gomock.InOrder(
		rt.EXPECT().Pull(gomock.Any(), "img:1").Return(nil),
		rt.EXPECT().Create(gomock.Any(), gomock.Any()).Return("cid", nil),
	)
	id, err := createNamedContainer(context.Background(), rt, namedConfig(PullAlways), "name")
	require.NoError(t, err)
	assert.Equal(t, "cid", id)
}

func TestCreateNamedContainer_PullAlwaysFails(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	rt := NewMockRuntime(ctrl)

	// PullAlways pull fails up front; Create is never attempted.
	rt.EXPECT().Pull(gomock.Any(), "img:1").Return(assert.AnError)
	_, err := createNamedContainer(context.Background(), rt, namedConfig(PullAlways), "name")
	require.Error(t, err)
}

func TestCreateNamedContainer_NonImageErrorSurfacesWithoutPull(t *testing.T) {
	// Negative path: a non-missing-image create failure must surface as-is and must
	// NOT trigger a recovery pull.
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	rt := NewMockRuntime(ctrl)

	rt.EXPECT().Create(gomock.Any(), gomock.Any()).Return("", assert.AnError)
	_, err := createNamedContainer(context.Background(), rt, namedConfig(PullMissing), "name")
	require.Error(t, err)
}

func TestCreateNamedContainer_MissingImageRecoversByPull(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	rt := NewMockRuntime(ctrl)

	gomock.InOrder(
		rt.EXPECT().Create(gomock.Any(), gomock.Any()).Return("", errMissingImage),
		rt.EXPECT().Pull(gomock.Any(), "img:1").Return(nil),
		rt.EXPECT().Create(gomock.Any(), gomock.Any()).Return("cid", nil),
	)
	id, err := createNamedContainer(context.Background(), rt, namedConfig(PullMissing), "name")
	require.NoError(t, err)
	assert.Equal(t, "cid", id)
}

func TestCreateNamedContainer_MissingImagePullFails(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	rt := NewMockRuntime(ctrl)

	gomock.InOrder(
		rt.EXPECT().Create(gomock.Any(), gomock.Any()).Return("", errMissingImage),
		rt.EXPECT().Pull(gomock.Any(), "img:1").Return(assert.AnError),
	)
	_, err := createNamedContainer(context.Background(), rt, namedConfig(PullMissing), "name")
	require.Error(t, err)
}

func TestCreateNamedContainer_MissingImageRetryCreateFails(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	rt := NewMockRuntime(ctrl)

	gomock.InOrder(
		rt.EXPECT().Create(gomock.Any(), gomock.Any()).Return("", errMissingImage),
		rt.EXPECT().Pull(gomock.Any(), "img:1").Return(nil),
		rt.EXPECT().Create(gomock.Any(), gomock.Any()).Return("", assert.AnError),
	)
	_, err := createNamedContainer(context.Background(), rt, namedConfig(PullMissing), "name")
	require.Error(t, err)
}

func TestCreateNamedContainer_PullNeverDoesNotRecover(t *testing.T) {
	// Negative path: with PullNever, a missing image is not recovered by pulling.
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	rt := NewMockRuntime(ctrl)

	rt.EXPECT().Create(gomock.Any(), gomock.Any()).Return("", errMissingImage)
	_, err := createNamedContainer(context.Background(), rt, namedConfig(PullNever), "name")
	require.Error(t, err)
}

func TestUpWithRuntime_NilParams(t *testing.T) {
	cfg := &NamedConfig{Stack: "dev", ComponentType: "container", Component: "api", Image: "img:1"}
	_, err := UpWithRuntime(context.Background(), nil, cfg)
	require.ErrorIs(t, err, errUtils.ErrNilParam)

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	_, err = UpWithRuntime(context.Background(), NewMockRuntime(ctrl), nil)
	require.ErrorIs(t, err, errUtils.ErrNilParam)
}

func TestUpWithRuntime_RequiresImage(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	_, err := UpWithRuntime(context.Background(), NewMockRuntime(ctrl),
		&NamedConfig{Stack: "dev", ComponentType: "container", Component: "api"})
	require.Error(t, err)
}

func TestUpWithRuntime_DelegatesToReconcile(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	rt := NewMockRuntime(ctrl)
	cfg := &NamedConfig{Stack: "dev", ComponentType: "container", Component: "api", Image: "img:1"}

	// Already running: UpWithRuntime returns AlreadyRunning without mutating.
	rt.EXPECT().List(gomock.Any(), DiscoveryFilter("dev", "container", "api")).
		Return([]Info{newContainerInfo("cid", "running", "api")}, nil)

	named, err := UpWithRuntime(context.Background(), rt, cfg)
	require.NoError(t, err)
	assert.True(t, named.AlreadyRunning)
	assert.Equal(t, "cid", named.ID())
}

func TestUpWithRuntime_StartFailureCleansUp(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	rt := NewMockRuntime(ctrl)
	cfg := &NamedConfig{Stack: "dev", ComponentType: "container", Component: "api", Image: "img:1"}

	// Created but unstartable: best-effort Remove is attempted.
	gomock.InOrder(
		rt.EXPECT().List(gomock.Any(), gomock.Any()).Return([]Info{}, nil),
		rt.EXPECT().Create(gomock.Any(), gomock.Any()).Return("cid", nil),
		rt.EXPECT().Start(gomock.Any(), "cid").Return(assert.AnError),
		rt.EXPECT().Remove(gomock.Any(), "cid", true).Return(nil),
	)
	_, err := upWithRuntime(context.Background(), rt, cfg, "name")
	require.Error(t, err)
}

func TestUpWithRuntime_StartFailureAndCleanupFailure(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	rt := NewMockRuntime(ctrl)
	cfg := &NamedConfig{Stack: "dev", ComponentType: "container", Component: "api", Image: "img:1"}

	// Both Start and the cleanup Remove fail: both errors are surfaced.
	gomock.InOrder(
		rt.EXPECT().List(gomock.Any(), gomock.Any()).Return([]Info{}, nil),
		rt.EXPECT().Create(gomock.Any(), gomock.Any()).Return("cid", nil),
		rt.EXPECT().Start(gomock.Any(), "cid").Return(assert.AnError),
		rt.EXPECT().Remove(gomock.Any(), "cid", true).Return(errors.New("remove failed")),
	)
	err := func() error {
		_, e := upWithRuntime(context.Background(), rt, cfg, "name")
		return e
	}()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cleanup failed")
}

func TestBuildNamedCreateConfig(t *testing.T) {
	cfg := &NamedConfig{
		Stack: "dev", ComponentType: "container", Component: "api",
		Image:   "img:1",
		Command: []string{"./api"},
		Ports:   []PortBinding{{HostPort: 8080, ContainerPort: 80}},
		Mounts:  []Mount{{Source: "/repo", Target: "/workspace"}},
		Env:     map[string]string{"PORT": "8080"},
		User:    "app",
		RunArgs: []string{"--rm"},
		Labels:  map[string]string{"custom": "x"},
	}
	got := buildNamedCreateConfig(cfg, "name")

	assert.Equal(t, "name", got.Name)
	assert.Equal(t, "img:1", got.Image)
	assert.Equal(t, []string{"./api"}, got.Command)
	assert.Equal(t, "app", got.User)
	assert.Equal(t, []string{"--rm"}, got.RunArgs)
	require.Len(t, got.Ports, 1)
	assert.Equal(t, 8080, got.Ports[0].HostPort)
	require.Len(t, got.Mounts, 1)
	assert.Equal(t, "/workspace", got.Mounts[0].Target)

	// Canonical instance labels are present, and caller labels are merged in.
	assert.Equal(t, "dev/container/api", got.Labels[LabelInstance])
	assert.Equal(t, "x", got.Labels["custom"])

	// Isolation (result→source): mutating the result labels must not mutate the
	// source config.
	got.Labels["mutated"] = "y"
	_, exists := cfg.Labels["mutated"]
	assert.False(t, exists)

	// Isolation (source→result): mutating the source config after the build must
	// not leak into the already-built result (no shared backing map).
	cfg.Labels["added-after"] = "z"
	_, leaked := got.Labels["added-after"]
	assert.False(t, leaked)
}

func TestBuildNamedCreateConfig_IdentityLabelsAuthoritative(t *testing.T) {
	// Reserved identity labels are authoritative: a caller attempting to override
	// one must not win, otherwise label-based discovery (FindInstance/Up/Down)
	// would no longer match this instance. Non-reserved caller labels still merge.
	cfg := &NamedConfig{
		Stack: "dev", ComponentType: "container", Component: "api", Image: "img:1",
		Labels: map[string]string{LabelInstance: "overridden", "custom": "x"},
	}
	got := buildNamedCreateConfig(cfg, "name")
	assert.Equal(t, "dev/container/api", got.Labels[LabelInstance])
	assert.Equal(t, "x", got.Labels["custom"])
}

func TestContainerRef(t *testing.T) {
	assert.Empty(t, containerRef(nil))
	assert.Equal(t, "cid", containerRef(&Info{ID: "cid", Name: "name"})) // ID precedence
	assert.Equal(t, "name", containerRef(&Info{Name: "name"}))           // Name fallback
}
