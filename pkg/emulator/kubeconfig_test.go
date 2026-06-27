package emulator

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

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
	assert.Contains(t, out, "server: https://localhost:36443", "server rewritten to the live host port")
	assert.NotContains(t, out, "127.0.0.1:6443")
	assert.Contains(t, out, "certificate-authority-data: BASE64CA", "embedded CA preserved verbatim")
}

func TestManager_Kubeconfig_NotRunning(t *testing.T) {
	ctrl := gomock.NewController(t)
	runtime := NewMockRuntime(ctrl)
	runtime.EXPECT().List(gomock.Any(), gomock.Any()).Return([]container.Info{}, nil)

	m := newManagerWithRuntime(runtime)
	_, err := m.Kubeconfig(context.Background(), "dev", "k3s")
	require.Error(t, err)
	assert.True(t, strings.Contains(err.Error(), "not running"))
}
