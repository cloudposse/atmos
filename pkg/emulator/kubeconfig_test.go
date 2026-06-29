package emulator

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/container"
)

func k3sEmulatorInfo(hostPort int) container.Info {
	return container.Info{
		ID:     "k3s-abc",
		Status: "running",
		Labels: map[string]string{
			container.LabelInstance: container.InstanceAddress("dev", "emulator", "k3s"),
			container.LabelStack:    "dev",
		},
		Ports: []container.PortBinding{{ContainerPort: 6443, HostPort: hostPort, Protocol: "tcp"}},
	}
}

const rawK3sKubeconfig = `apiVersion: v1
clusters:
- cluster:
    certificate-authority-data: BASE64CA
    server: https://127.0.0.1:6443
  name: default
`

func TestManager_Kubeconfig_HarvestAndRewriteServer(t *testing.T) {
	ctrl := gomock.NewController(t)
	runtime := NewMockRuntime(ctrl)
	runtime.EXPECT().List(gomock.Any(), gomock.Any()).Return([]container.Info{k3sEmulatorInfo(36443)}, nil)
	runtime.EXPECT().
		Exec(gomock.Any(), "k3s-abc", []string{"cat", k3sKubeconfigPath}, gomock.Any()).
		DoAndReturn(func(_ context.Context, _ string, _ []string, opts *container.ExecOptions) error {
			_, err := opts.Stdout.Write([]byte(rawK3sKubeconfig))
			return err
		})

	m := newManagerWithRuntime(runtime)
	kubeconfig, err := m.Kubeconfig(context.Background(), "dev", "k3s")
	require.NoError(t, err)

	out := string(kubeconfig)
	// Rewritten to the live host port on the IPv4 loopback literal (not "localhost":
	// it can resolve to IPv6 ::1 and hang against an IPv4-only published port).
	assert.Contains(t, out, "server: https://127.0.0.1:36443", "server rewritten to the live host port on IPv4 loopback")
	assert.NotContains(t, out, ":6443", "original container port replaced by the live host port")
	assert.Contains(t, out, "certificate-authority-data: BASE64CA", "embedded CA preserved verbatim")
}

// shrinkKubeconfigTimers shrinks the readiness timers so retrying error paths
// fail fast instead of polling for 90s. Restored on cleanup.
func shrinkKubeconfigTimers(t *testing.T) {
	t.Helper()
	oldTimeout := kubeconfigReadyTimeout
	oldInterval := kubeconfigPollInterval
	kubeconfigReadyTimeout = time.Millisecond
	kubeconfigPollInterval = time.Millisecond
	t.Cleanup(func() {
		kubeconfigReadyTimeout = oldTimeout
		kubeconfigPollInterval = oldInterval
	})
}

// errExec is a sentinel used to assert the Exec-failure branch wraps ErrEmulatorConfigInvalid.
var errExec = errors.New("exec boom")

func TestManager_Kubeconfig_ExecError(t *testing.T) {
	ctrl := gomock.NewController(t)
	runtime := NewMockRuntime(ctrl)
	runtime.EXPECT().List(gomock.Any(), gomock.Any()).Return([]container.Info{k3sEmulatorInfo(36443)}, nil).AnyTimes()
	runtime.EXPECT().
		Exec(gomock.Any(), "k3s-abc", []string{"cat", k3sKubeconfigPath}, gomock.Any()).
		Return(errExec).
		AnyTimes()
	shrinkKubeconfigTimers(t)

	m := newManagerWithRuntime(runtime)
	_, err := m.Kubeconfig(context.Background(), "dev", "k3s")
	require.Error(t, err)
	// A failed `cat` inside the container means the emulator config is unreadable/invalid.
	assert.ErrorIs(t, err, errUtils.ErrEmulatorConfigInvalid)
	assert.ErrorIs(t, err, errExec)
}

func TestManager_Kubeconfig_NoBoundPort(t *testing.T) {
	ctrl := gomock.NewController(t)
	runtime := NewMockRuntime(ctrl)
	// hostPort==0 -> the container exists but exposes no bound port yet.
	runtime.EXPECT().List(gomock.Any(), gomock.Any()).Return([]container.Info{k3sEmulatorInfo(0)}, nil).AnyTimes()
	runtime.EXPECT().
		Exec(gomock.Any(), "k3s-abc", []string{"cat", k3sKubeconfigPath}, gomock.Any()).
		DoAndReturn(func(_ context.Context, _ string, _ []string, opts *container.ExecOptions) error {
			_, err := opts.Stdout.Write([]byte(rawK3sKubeconfig))
			return err
		}).
		AnyTimes()
	shrinkKubeconfigTimers(t)

	m := newManagerWithRuntime(runtime)
	_, err := m.Kubeconfig(context.Background(), "dev", "k3s")
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrEmulatorNotRunning)
	assert.Contains(t, err.Error(), "no bound port")
}

func TestFirstBoundPort(t *testing.T) {
	tests := []struct {
		name  string
		ports []container.PortBinding
		want  int
	}{
		{
			name:  "no ports",
			ports: nil,
			want:  0,
		},
		{
			name:  "single unbound port",
			ports: []container.PortBinding{{ContainerPort: 6443, HostPort: 0, Protocol: "tcp"}},
			want:  0,
		},
		{
			name: "skips unbound and returns first bound",
			ports: []container.PortBinding{
				{ContainerPort: 6443, HostPort: 0, Protocol: "tcp"},
				{ContainerPort: 8080, HostPort: 49153, Protocol: "tcp"},
			},
			want: 49153,
		},
		{
			name:  "single bound port",
			ports: []container.PortBinding{{ContainerPort: 6443, HostPort: 36443, Protocol: "tcp"}},
			want:  36443,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := firstBoundPort(&container.Info{Ports: tt.ports})
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestManager_Kubeconfig_NotRunning(t *testing.T) {
	ctrl := gomock.NewController(t)
	runtime := NewMockRuntime(ctrl)
	runtime.EXPECT().List(gomock.Any(), gomock.Any()).Return([]container.Info{}, nil).AnyTimes()
	oldTimeout := kubeconfigReadyTimeout
	oldInterval := kubeconfigPollInterval
	kubeconfigReadyTimeout = time.Millisecond
	kubeconfigPollInterval = time.Millisecond
	t.Cleanup(func() {
		kubeconfigReadyTimeout = oldTimeout
		kubeconfigPollInterval = oldInterval
	})

	m := newManagerWithRuntime(runtime)
	_, err := m.Kubeconfig(context.Background(), "dev", "k3s")
	require.Error(t, err)
	assert.True(t, strings.Contains(err.Error(), "not running"))
}
