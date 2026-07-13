package emulator

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/container"
)

// runningEmulatorInfo builds a container.Info that FindInstance will match for
// the dev/emulator/aws instance, with a live host-port binding.
func runningEmulatorInfo(hostPort int) container.Info {
	return container.Info{
		ID:     "container-abc",
		Name:   "atmos-dev-emulator-aws",
		Image:  testDriverImage,
		Status: "running",
		Labels: map[string]string{
			container.LabelInstance:      container.InstanceAddress("dev", "emulator", "aws"),
			container.LabelStack:         "dev",
			container.LabelComponentType: "emulator",
			container.LabelComponent:     "aws",
		},
		Ports: []container.PortBinding{{ContainerPort: 4566, HostPort: hostPort, Protocol: "tcp"}},
	}
}

func TestManager_Resolve_Running(t *testing.T) {
	ctrl := gomock.NewController(t)
	runtime := NewMockRuntime(ctrl)
	runtime.EXPECT().List(gomock.Any(), gomock.Any()).Return([]container.Info{runningEmulatorInfo(54321)}, nil)
	runtime.EXPECT().Inspect(gomock.Any(), "container-abc").Return(&container.Info{}, nil)

	m := newManagerWithRuntime(runtime)
	endpoint, profile, err := m.Resolve(context.Background(), &Spec{Driver: testDriverName}, "dev", "aws")
	require.NoError(t, err)

	assert.Equal(t, TargetAWS, endpoint.Target)
	assert.Equal(t, 54321, endpoint.Ports[4566])
	assert.Equal(t, "http://127.0.0.1:54321", profile.Env["AWS_ENDPOINT_URL"])
	assert.Equal(t, "1", profile.Env["TEST_DRIVER"], "manager returns the resolved driver's profile env")
	assert.Equal(t, true, profile.Provider["test_flag"], "manager returns the resolved driver's provider fragment")
}

func TestManager_Resolve_GitHubJobContainerUsesNetworkAliasEndpoint(t *testing.T) {
	t.Setenv("GITHUB_ACTIONS", "true")
	t.Setenv(envEmulatorUseCurrentContainerNetwork, "")
	restore := stubEndpointHostDetection(t, true, "")
	defer restore()

	ctrl := gomock.NewController(t)
	runtime := NewMockRuntime(ctrl)
	gomock.InOrder(
		runtime.EXPECT().List(gomock.Any(), gomock.Any()).Return([]container.Info{runningEmulatorInfo(54321)}, nil),
		runtime.EXPECT().Inspect(gomock.Any(), "container-abc").Return(&container.Info{}, nil),
		runtime.EXPECT().Inspect(gomock.Any(), gomock.Any()).Return(&container.Info{Networks: []string{"github_network_123"}}, nil),
	)

	m := newManagerWithRuntime(runtime)
	endpoint, profile, err := m.Resolve(context.Background(), &Spec{Driver: testDriverName}, "dev", "aws")
	require.NoError(t, err)

	assert.Equal(t, "dev-aws", endpoint.Host)
	assert.Equal(t, 4566, endpoint.Ports[4566])
	assert.Equal(t, "http://dev-aws:4566", profile.Env["AWS_ENDPOINT_URL"])
}

func TestManager_Resolve_NotRunning(t *testing.T) {
	ctrl := gomock.NewController(t)
	runtime := NewMockRuntime(ctrl)
	runtime.EXPECT().List(gomock.Any(), gomock.Any()).Return([]container.Info{}, nil)

	m := newManagerWithRuntime(runtime)
	_, _, err := m.Resolve(context.Background(), &Spec{Driver: testDriverName}, "dev", "aws")
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrEmulatorNotRunning)
}

func TestManager_Ps_FiltersByStack(t *testing.T) {
	ctrl := gomock.NewController(t)
	runtime := NewMockRuntime(ctrl)
	other := runningEmulatorInfo(40000)
	other.Labels[container.LabelStack] = "prod" // different stack — excluded.
	runtime.EXPECT().List(gomock.Any(), gomock.Any()).Return([]container.Info{runningEmulatorInfo(54321), other}, nil)

	m := newManagerWithRuntime(runtime)
	statuses, err := m.Ps(context.Background(), "dev")
	require.NoError(t, err)
	require.Len(t, statuses, 1)
	assert.Equal(t, "aws", statuses[0].Name)
	assert.Equal(t, testDriverImage, statuses[0].Image)
	assert.Equal(t, "atmos-dev-emulator-aws", statuses[0].Container)
	assert.Equal(t, "container-abc", statuses[0].ID)
}
