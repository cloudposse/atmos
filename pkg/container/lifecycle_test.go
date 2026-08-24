package container

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func newContainerInfo(id, status, comp string) Info {
	return Info{
		ID:     id,
		Status: status,
		Labels: InstanceLabels("dev", "container", comp),
	}
}

func TestUp_DryRun_DoesNotTouchRuntime(t *testing.T) {
	named, err := Up(context.Background(), &NamedConfig{
		Stack:         "dev",
		ComponentType: "container",
		Component:     "api",
		Image:         "alpine:latest",
		DryRun:        true,
	})
	require.NoError(t, err)
	assert.Equal(t, "atmos-dev-container-api", named.Name())
	assert.Equal(t, "atmos-dev-container-api", named.ID())
}

func TestUp_RequiresImage(t *testing.T) {
	_, err := Up(context.Background(), &NamedConfig{Stack: "dev", ComponentType: "container", Component: "api"})
	require.Error(t, err)
}

func TestUpWithRuntime_CreatesWhenAbsent(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ctx := context.Background()
	rt := NewMockRuntime(ctrl)
	cfg := &NamedConfig{
		Stack: "dev", ComponentType: "container", Component: "api",
		Image:   "localhost:5001/api:abc",
		Command: []string{"./api"},
		Ports:   []PortBinding{{ContainerPort: 8080, HostPort: 8080, Protocol: "tcp"}},
		Mounts:  []Mount{{Type: "bind", Source: "/repo", Target: "/workspace"}},
		Env:     map[string]string{"PORT": "8080"},
	}

	gomock.InOrder(
		rt.EXPECT().
			List(ctx, DiscoveryFilter("dev", "container", "api")).
			Return([]Info{}, nil),
		rt.EXPECT().Create(ctx, gomock.Any()).
			DoAndReturn(func(_ context.Context, create *CreateConfig) (string, error) {
				assert.Equal(t, "atmos-dev-container-api", create.Name)
				assert.Equal(t, "localhost:5001/api:abc", create.Image)
				assert.Equal(t, []string{"./api"}, create.Command)
				assert.Equal(t, "dev/container/api", create.Labels[LabelInstance])
				assert.Equal(t, "container", create.Labels[LabelComponentType])
				// Mounts and ports are threaded through unchanged.
				require.Len(t, create.Ports, 1)
				assert.Equal(t, 8080, create.Ports[0].HostPort)
				require.Len(t, create.Mounts, 1)
				assert.Equal(t, "/workspace", create.Mounts[0].Target)
				assert.Equal(t, "8080", create.Env["PORT"])
				return "new-id", nil
			}),
		rt.EXPECT().Start(ctx, "new-id").Return(nil),
	)

	named, err := upWithRuntime(ctx, rt, cfg, RuntimeName("dev", "container", "api"))
	require.NoError(t, err)
	assert.Equal(t, "new-id", named.ID())
	assert.False(t, named.AlreadyRunning)
}

func TestUpWithRuntime_StartsWhenStopped(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ctx := context.Background()
	rt := NewMockRuntime(ctrl)
	cfg := &NamedConfig{Stack: "dev", ComponentType: "container", Component: "api", Image: "alpine"}

	gomock.InOrder(
		rt.EXPECT().
			List(ctx, DiscoveryFilter("dev", "container", "api")).
			Return([]Info{newContainerInfo("existing", "exited", "api")}, nil),
		rt.EXPECT().Start(ctx, "existing").Return(nil),
	)

	named, err := upWithRuntime(ctx, rt, cfg, RuntimeName("dev", "container", "api"))
	require.NoError(t, err)
	assert.Equal(t, "existing", named.ID())
	assert.False(t, named.AlreadyRunning)
}

func TestUpWithRuntime_NoOpWhenRunning(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ctx := context.Background()
	rt := NewMockRuntime(ctrl)
	cfg := &NamedConfig{Stack: "dev", ComponentType: "container", Component: "api", Image: "alpine"}

	rt.EXPECT().
		List(ctx, DiscoveryFilter("dev", "container", "api")).
		Return([]Info{newContainerInfo("existing", "running", "api")}, nil)

	named, err := upWithRuntime(ctx, rt, cfg, RuntimeName("dev", "container", "api"))
	require.NoError(t, err)
	assert.Equal(t, "existing", named.ID())
	assert.True(t, named.AlreadyRunning)
}

func TestFindInstance_IgnoresForeignLabels(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ctx := context.Background()
	rt := NewMockRuntime(ctrl)

	// Runtime returns a container whose instance label does NOT match (loose
	// filter defense). FindInstance must reject it.
	rt.EXPECT().
		List(ctx, DiscoveryFilter("dev", "container", "api")).
		Return([]Info{newContainerInfo("other", "running", "worker")}, nil)

	info, found, err := FindInstance(ctx, rt, "dev", "container", "api")
	require.NoError(t, err)
	assert.False(t, found)
	assert.Nil(t, info)
}

func TestFindInstance_MatchesByInstanceLabel(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ctx := context.Background()
	rt := NewMockRuntime(ctrl)
	rt.EXPECT().
		List(ctx, DiscoveryFilter("dev", "container", "api")).
		Return([]Info{newContainerInfo("match", "running", "api")}, nil)

	info, found, err := FindInstance(ctx, rt, "dev", "container", "api")
	require.NoError(t, err)
	require.True(t, found)
	assert.Equal(t, "match", info.ID)
}

func TestDown_StopsAndRemovesRunning(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ctx := context.Background()
	rt := NewMockRuntime(ctrl)

	gomock.InOrder(
		rt.EXPECT().
			List(ctx, DiscoveryFilter("dev", "container", "api")).
			Return([]Info{newContainerInfo("id1", "running", "api")}, nil),
		rt.EXPECT().Stop(ctx, "id1", defaultStopTimeout).Return(nil),
		rt.EXPECT().Remove(ctx, "id1", true).Return(nil),
	)

	require.NoError(t, Down(ctx, rt, "dev", "container", "api"))
}

func TestDown_RemovesStoppedWithoutStop(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ctx := context.Background()
	rt := NewMockRuntime(ctrl)

	gomock.InOrder(
		rt.EXPECT().
			List(ctx, DiscoveryFilter("dev", "container", "api")).
			Return([]Info{newContainerInfo("id1", "exited", "api")}, nil),
		rt.EXPECT().Remove(ctx, "id1", true).Return(nil),
	)

	require.NoError(t, Down(ctx, rt, "dev", "container", "api"))
}

func TestDown_NoOpWhenAbsent(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ctx := context.Background()
	rt := NewMockRuntime(ctrl)
	rt.EXPECT().
		List(ctx, DiscoveryFilter("dev", "container", "api")).
		Return([]Info{}, nil)

	require.NoError(t, Down(ctx, rt, "dev", "container", "api"))
}
