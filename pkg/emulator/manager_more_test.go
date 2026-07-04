package emulator

import (
	"context"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/container"
)

// errRuntimeBoom is a sentinel runtime error used to drive the manager's error paths.
var errRuntimeBoom = errors.New("boom")

// kubeTestDriverName is a distinct in-test driver whose Target is kubernetes, so
// the Resolve kubernetes branch (kubeconfig harvest) is exercised without
// clobbering the AWS-target testDriver.
const (
	kubeTestDriverName  = "test/k3s"
	kubeTestDriverImage = "test/k3s:latest"
	kubeTestDriverPort  = 6443
)

// kubeTestDriver is a kubernetes-target driver registered only for these tests.
type kubeTestDriver struct{}

func (kubeTestDriver) Name() string   { return kubeTestDriverName }
func (kubeTestDriver) Target() string { return TargetKubernetes }

func (kubeTestDriver) Defaults() ContainerDefaults {
	return ContainerDefaults{Image: kubeTestDriverImage, Ports: []int{kubeTestDriverPort}}
}

// Profile leaves Kubeconfig empty so the manager harvests it from the container.
func (kubeTestDriver) Profile(_ *Endpoint) Profile {
	return Profile{Env: map[string]string{"KUBE_DRIVER": "1"}}
}

func init() {
	RegisterDriver(kubeTestDriver{})
}

// kubeRunningInfo builds a running kubernetes-emulator container.Info that
// FindInstance matches for dev/emulator/k8s, with a live host-port binding.
func kubeRunningInfo(hostPort int) container.Info {
	return container.Info{
		ID:     "k3s-container-xyz",
		Image:  kubeTestDriverImage,
		Status: "running",
		Labels: map[string]string{
			container.LabelInstance:      container.InstanceAddress("dev", "emulator", "k8s"),
			container.LabelStack:         "dev",
			container.LabelComponentType: "emulator",
			container.LabelComponent:     "k8s",
		},
		Ports: []container.PortBinding{{ContainerPort: kubeTestDriverPort, HostPort: hostPort, Protocol: "tcp"}},
	}
}

func TestManager_Up_ReusesRunningContainer(t *testing.T) {
	ctrl := gomock.NewController(t)
	runtime := NewMockRuntime(ctrl)
	// Up calls UpWithRuntime (FindInstance->List) then endpoint (FindInstance->List).
	// A running instance is reused, so no Create/Start is invoked.
	runtime.EXPECT().List(gomock.Any(), gomock.Any()).
		Return([]container.Info{runningEmulatorInfo(54321)}, nil).Times(2)
	runtime.EXPECT().Inspect(gomock.Any(), "container-abc").Return(&container.Info{}, nil)

	m := newManagerWithRuntime(runtime)
	endpoint, err := m.Up(context.Background(), &Spec{Driver: testDriverName}, "dev", "aws",
		map[string]string{"EXTRA": "1"})
	require.NoError(t, err)

	assert.Equal(t, TargetAWS, endpoint.Target)
	assert.Equal(t, "localhost", endpoint.Host)
	assert.Equal(t, 54321, endpoint.Ports[4566])
}

func TestManager_Up_CreatesAndStartsContainer(t *testing.T) {
	ctrl := gomock.NewController(t)
	runtime := NewMockRuntime(ctrl)
	// First List (UpWithRuntime) finds nothing -> Create+Start. Second List
	// (endpoint) returns the now-running container so ports read back.
	gomock.InOrder(
		runtime.EXPECT().List(gomock.Any(), gomock.Any()).Return(nil, nil),
		runtime.EXPECT().Create(gomock.Any(), gomock.Any()).Return("new-id", nil),
		runtime.EXPECT().Start(gomock.Any(), "new-id").Return(nil),
		runtime.EXPECT().List(gomock.Any(), gomock.Any()).
			Return([]container.Info{runningEmulatorInfo(40001)}, nil),
		runtime.EXPECT().Inspect(gomock.Any(), "container-abc").Return(&container.Info{}, nil),
	)

	m := newManagerWithRuntime(runtime)
	endpoint, err := m.Up(context.Background(), &Spec{Driver: testDriverName}, "dev", "aws", nil)
	require.NoError(t, err)
	assert.Equal(t, 40001, endpoint.Ports[4566])
}

func TestManager_Up_GitHubJobContainerAttachesCurrentNetworkAlias(t *testing.T) {
	t.Setenv("GITHUB_ACTIONS", "true")
	t.Setenv(envEmulatorUseCurrentContainerNetwork, "")
	restore := stubEndpointHostDetection(t, true, "")
	defer restore()

	ctrl := gomock.NewController(t)
	runtime := NewMockRuntime(ctrl)
	gomock.InOrder(
		runtime.EXPECT().Inspect(gomock.Any(), gomock.Any()).Return(&container.Info{Networks: []string{"github_network_123"}}, nil),
		runtime.EXPECT().List(gomock.Any(), gomock.Any()).Return(nil, nil),
		runtime.EXPECT().Create(gomock.Any(), gomock.Any()).
			DoAndReturn(func(_ context.Context, create *container.CreateConfig) (string, error) {
				require.Len(t, create.Networks, 1)
				assert.Equal(t, "github_network_123", create.Networks[0].Name)
				assert.Equal(t, []string{"dev-aws"}, create.Networks[0].Aliases)
				return "new-id", nil
			}),
		runtime.EXPECT().Start(gomock.Any(), "new-id").Return(nil),
		runtime.EXPECT().List(gomock.Any(), gomock.Any()).
			Return([]container.Info{runningEmulatorInfo(40001)}, nil),
		runtime.EXPECT().Inspect(gomock.Any(), "container-abc").Return(&container.Info{}, nil),
		runtime.EXPECT().Inspect(gomock.Any(), gomock.Any()).Return(&container.Info{Networks: []string{"github_network_123"}}, nil),
	)

	m := newManagerWithRuntime(runtime)
	endpoint, err := m.Up(context.Background(), &Spec{Driver: testDriverName}, "dev", "aws", nil)
	require.NoError(t, err)
	assert.Equal(t, "dev-aws", endpoint.Host)
	assert.Equal(t, 4566, endpoint.Ports[4566])
}

func TestManager_Up_UpWithRuntimeError(t *testing.T) {
	ctrl := gomock.NewController(t)
	runtime := NewMockRuntime(ctrl)
	runtime.EXPECT().List(gomock.Any(), gomock.Any()).Return(nil, errRuntimeBoom)

	m := newManagerWithRuntime(runtime)
	_, err := m.Up(context.Background(), &Spec{Driver: testDriverName}, "dev", "aws", nil)
	require.Error(t, err)
}

func TestManager_Up_UnknownDriver(t *testing.T) {
	ctrl := gomock.NewController(t)
	runtime := NewMockRuntime(ctrl)
	// namedConfig resolves spec.Image() first, which fails on an unknown driver
	// before any runtime call, so no List is expected.

	m := newManagerWithRuntime(runtime)
	_, err := m.Up(context.Background(), &Spec{Driver: "nope/missing"}, "dev", "aws", nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrUnknownEmulatorDriver)
}

func TestManager_Down_Delegates(t *testing.T) {
	ctrl := gomock.NewController(t)
	runtime := NewMockRuntime(ctrl)
	info := runningEmulatorInfo(54321)
	// Down -> FindInstance (List) -> Stop (running) -> Remove.
	gomock.InOrder(
		runtime.EXPECT().List(gomock.Any(), gomock.Any()).Return([]container.Info{info}, nil),
		runtime.EXPECT().Stop(gomock.Any(), info.ID, gomock.Any()).Return(nil),
		runtime.EXPECT().Remove(gomock.Any(), info.ID, true).Return(nil),
	)

	m := newManagerWithRuntime(runtime)
	require.NoError(t, m.Down(context.Background(), "dev", "aws"))
}

func TestManager_Down_NotFoundIsNoOp(t *testing.T) {
	ctrl := gomock.NewController(t)
	runtime := NewMockRuntime(ctrl)
	runtime.EXPECT().List(gomock.Any(), gomock.Any()).Return(nil, nil)

	m := newManagerWithRuntime(runtime)
	require.NoError(t, m.Down(context.Background(), "dev", "aws"))
}

func TestManager_Logs_InvokesRuntimeLogs(t *testing.T) {
	ctrl := gomock.NewController(t)
	runtime := NewMockRuntime(ctrl)
	info := runningEmulatorInfo(54321)
	runtime.EXPECT().List(gomock.Any(), gomock.Any()).Return([]container.Info{info}, nil)
	runtime.EXPECT().
		Logs(gomock.Any(), info.ID, true, "all", gomock.Nil(), gomock.Nil()).
		Return(nil)

	m := newManagerWithRuntime(runtime)
	require.NoError(t, m.Logs(context.Background(), "dev", "aws", true))
}

func TestManager_Logs_NotFound(t *testing.T) {
	ctrl := gomock.NewController(t)
	runtime := NewMockRuntime(ctrl)
	runtime.EXPECT().List(gomock.Any(), gomock.Any()).Return(nil, nil)

	m := newManagerWithRuntime(runtime)
	err := m.Logs(context.Background(), "dev", "aws", false)
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrEmulatorNotRunning)
}

func TestManager_Exec_DefaultsToShell(t *testing.T) {
	ctrl := gomock.NewController(t)
	runtime := NewMockRuntime(ctrl)
	info := runningEmulatorInfo(54321)
	originalStdinIsTerminal := stdinIsTerminal
	stdinIsTerminal = func() bool { return true }
	t.Cleanup(func() { stdinIsTerminal = originalStdinIsTerminal })

	runtime.EXPECT().List(gomock.Any(), gomock.Any()).Return([]container.Info{info}, nil)
	runtime.EXPECT().
		Exec(gomock.Any(), info.ID, []string{"/bin/sh"}, gomock.AssignableToTypeOf(&container.ExecOptions{})).
		DoAndReturn(func(_ context.Context, _ string, _ []string, opts *container.ExecOptions) error {
			assert.True(t, opts.Tty, "interactive exec must allocate a TTY")
			assert.True(t, opts.AttachStdin)
			assert.True(t, opts.AttachStdout)
			assert.True(t, opts.AttachStderr)
			return nil
		})

	m := newManagerWithRuntime(runtime)
	require.NoError(t, m.Exec(context.Background(), "dev", "aws", nil))
}

func TestManager_Exec_NonInteractiveDisablesTTYAndStdin(t *testing.T) {
	ctrl := gomock.NewController(t)
	runtime := NewMockRuntime(ctrl)
	info := runningEmulatorInfo(54321)
	originalStdinIsTerminal := stdinIsTerminal
	stdinIsTerminal = func() bool { return false }
	t.Cleanup(func() { stdinIsTerminal = originalStdinIsTerminal })

	runtime.EXPECT().List(gomock.Any(), gomock.Any()).Return([]container.Info{info}, nil)
	runtime.EXPECT().
		Exec(gomock.Any(), info.ID, []string{"env"}, gomock.AssignableToTypeOf(&container.ExecOptions{})).
		DoAndReturn(func(_ context.Context, _ string, _ []string, opts *container.ExecOptions) error {
			assert.False(t, opts.Tty)
			assert.False(t, opts.AttachStdin)
			assert.True(t, opts.AttachStdout)
			assert.True(t, opts.AttachStderr)
			return nil
		})

	m := newManagerWithRuntime(runtime)
	require.NoError(t, m.Exec(context.Background(), "dev", "aws", []string{"env"}))
}

func TestManager_Exec_UsesProvidedCommand(t *testing.T) {
	ctrl := gomock.NewController(t)
	runtime := NewMockRuntime(ctrl)
	info := runningEmulatorInfo(54321)
	cmd := []string{"env"}
	runtime.EXPECT().List(gomock.Any(), gomock.Any()).Return([]container.Info{info}, nil)
	runtime.EXPECT().Exec(gomock.Any(), info.ID, cmd, gomock.Any()).Return(nil)

	m := newManagerWithRuntime(runtime)
	require.NoError(t, m.Exec(context.Background(), "dev", "aws", cmd))
}

func TestManager_Exec_NotFound(t *testing.T) {
	ctrl := gomock.NewController(t)
	runtime := NewMockRuntime(ctrl)
	runtime.EXPECT().List(gomock.Any(), gomock.Any()).Return(nil, nil)

	m := newManagerWithRuntime(runtime)
	err := m.Exec(context.Background(), "dev", "aws", nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrEmulatorNotRunning)
}

func TestManager_endpoint_NotRunning(t *testing.T) {
	ctrl := gomock.NewController(t)
	runtime := NewMockRuntime(ctrl)
	stopped := runningEmulatorInfo(54321)
	stopped.Status = "exited" // found but not running -> ErrEmulatorNotRunning.
	runtime.EXPECT().List(gomock.Any(), gomock.Any()).Return([]container.Info{stopped}, nil)

	m := newManagerWithRuntime(runtime)
	_, err := m.endpoint(context.Background(), runtime, &Spec{Driver: testDriverName}, "dev", "aws")
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrEmulatorNotRunning)
}

func TestManager_endpoint_FindError(t *testing.T) {
	ctrl := gomock.NewController(t)
	runtime := NewMockRuntime(ctrl)
	runtime.EXPECT().List(gomock.Any(), gomock.Any()).Return(nil, errRuntimeBoom)

	m := newManagerWithRuntime(runtime)
	_, err := m.endpoint(context.Background(), runtime, &Spec{Driver: testDriverName}, "dev", "aws")
	require.Error(t, err)
}

func TestManager_Resolve_KubernetesHarvestsKubeconfig(t *testing.T) {
	ctrl := gomock.NewController(t)
	runtime := NewMockRuntime(ctrl)
	info := kubeRunningInfo(16443)
	// Resolve: endpoint (List) -> kubernetes branch -> Kubeconfig: find (List) +
	// Exec(cat kubeconfig) writing the raw kubeconfig into the provided buffer.
	rawKubeconfig := "apiVersion: v1\nclusters:\n- cluster:\n    server: https://127.0.0.1:6443\n"
	runtime.EXPECT().List(gomock.Any(), gomock.Any()).
		Return([]container.Info{info}, nil).Times(2)
	runtime.EXPECT().Inspect(gomock.Any(), info.ID).Return(&container.Info{}, nil)
	runtime.EXPECT().
		Exec(gomock.Any(), info.ID, []string{"cat", k3sKubeconfigPath}, gomock.Any()).
		DoAndReturn(func(_ context.Context, _ string, _ []string, opts *container.ExecOptions) error {
			_, _ = io.WriteString(opts.Stdout, rawKubeconfig)
			return nil
		})

	m := newManagerWithRuntime(runtime)
	endpoint, profile, err := m.Resolve(context.Background(), &Spec{Driver: kubeTestDriverName}, "dev", "k8s")
	require.NoError(t, err)

	assert.Equal(t, TargetKubernetes, endpoint.Target)
	assert.Equal(t, 16443, endpoint.Ports[kubeTestDriverPort])
	assert.Equal(t, "1", profile.Env["KUBE_DRIVER"])
	require.NotEmpty(t, profile.Kubeconfig)
	// The harvested kubeconfig's server URL is rewritten to the live host port on
	// the IPv4 loopback literal (not "localhost"; see loopbackHostToIPv4).
	assert.Contains(t, string(profile.Kubeconfig), "server: https://127.0.0.1:16443")
}

func TestManager_Resolve_KubernetesKubeconfigError(t *testing.T) {
	// The kubeconfig harvest retries the readiness race; pin the timeout to 0 so a
	// persistent exec error fails after a single attempt (matching the mock counts).
	origTimeout := kubeconfigReadyTimeout
	defer func() { kubeconfigReadyTimeout = origTimeout }()
	kubeconfigReadyTimeout = 0

	ctrl := gomock.NewController(t)
	runtime := NewMockRuntime(ctrl)
	info := kubeRunningInfo(16443)
	runtime.EXPECT().List(gomock.Any(), gomock.Any()).
		Return([]container.Info{info}, nil).AnyTimes()
	runtime.EXPECT().Inspect(gomock.Any(), info.ID).Return(&container.Info{}, nil).AnyTimes()
	runtime.EXPECT().
		Exec(gomock.Any(), info.ID, []string{"cat", k3sKubeconfigPath}, gomock.Any()).
		Return(errRuntimeBoom).AnyTimes()
	oldTimeout := kubeconfigReadyTimeout
	oldInterval := kubeconfigPollInterval
	kubeconfigReadyTimeout = time.Millisecond
	kubeconfigPollInterval = time.Millisecond
	t.Cleanup(func() {
		kubeconfigReadyTimeout = oldTimeout
		kubeconfigPollInterval = oldInterval
	})

	m := newManagerWithRuntime(runtime)
	_, _, err := m.Resolve(context.Background(), &Spec{Driver: kubeTestDriverName}, "dev", "k8s")
	require.Error(t, err)
}

func TestManager_find_ListError(t *testing.T) {
	ctrl := gomock.NewController(t)
	runtime := NewMockRuntime(ctrl)
	runtime.EXPECT().List(gomock.Any(), gomock.Any()).Return(nil, errRuntimeBoom)

	m := newManagerWithRuntime(runtime)
	_, _, err := m.find(context.Background(), "dev", "aws")
	require.Error(t, err)
}

func TestManager_Ps_ListError(t *testing.T) {
	ctrl := gomock.NewController(t)
	runtime := NewMockRuntime(ctrl)
	runtime.EXPECT().List(gomock.Any(), gomock.Any()).Return(nil, errRuntimeBoom)

	m := newManagerWithRuntime(runtime)
	_, err := m.Ps(context.Background(), "dev")
	require.Error(t, err)
}

// rootlessTestDriverName is a rootless-capable driver used to exercise the
// rootless override branch of namedConfig (which RuntimeIsRootless never enters
// for a mock runtime, so it is driven by calling namedConfig directly).
const rootlessTestDriverName = "test/rootless"

type rootlessTestDriver struct{}

func (rootlessTestDriver) Name() string   { return rootlessTestDriverName }
func (rootlessTestDriver) Target() string { return TargetKubernetes }

func (rootlessTestDriver) Defaults() ContainerDefaults {
	return ContainerDefaults{
		Image:      "test/rootless:latest",
		Ports:      []int{6443},
		Env:        map[string]string{"K3S_TOKEN": "secret"},
		Privileged: true,
		Command:    []string{"server"},
	}
}

func (rootlessTestDriver) Profile(_ *Endpoint) Profile { return Profile{} }

// RootlessOverride makes rootlessTestDriver implement RootlessOverrider.
func (rootlessTestDriver) RootlessOverride() (runArgs, command []string, ok bool) {
	return []string{"--entrypoint", "/rootless.sh"}, []string{"server", "--rootless"}, true
}

func init() {
	RegisterDriver(rootlessTestDriver{})
}

func TestManager_namedConfig_RootlessOverrideAndMerge(t *testing.T) {
	m := newManagerWithRuntime(nil)
	spec := &Spec{Driver: rootlessTestDriverName}

	cfgRootless, err := m.namedConfig(spec, "dev", "k8s",
		map[string]string{"K3S_TOKEN": "override", "EXTRA": "1"}, true)
	require.NoError(t, err)

	// Rootless branch swaps in the driver's rootless run-args and command.
	assert.Equal(t, []string{"--entrypoint", "/rootless.sh"}, cfgRootless.RunArgs)
	assert.Equal(t, []string{"server", "--rootless"}, cfgRootless.Command)
	assert.True(t, cfgRootless.Privileged)
	// Component env overrides the driver default; other driver defaults survive.
	assert.Equal(t, "override", cfgRootless.Env["K3S_TOKEN"])
	assert.Equal(t, "1", cfgRootless.Env["EXTRA"])
	assert.Len(t, cfgRootless.Ports, 1)
	assert.Equal(t, 6443, cfgRootless.Ports[0].ContainerPort)
	assert.Equal(t, defaultProtocol, cfgRootless.Ports[0].Protocol)

	// Rootful keeps the driver's default command and adds no rootless run-args.
	cfgRootful, err := m.namedConfig(spec, "dev", "k8s", nil, false)
	require.NoError(t, err)
	assert.Nil(t, cfgRootful.RunArgs)
	assert.Equal(t, []string{"server"}, cfgRootful.Command)
	assert.Equal(t, "secret", cfgRootful.Env["K3S_TOKEN"])
}

func TestManager_namedConfig_UnknownDriverErrors(t *testing.T) {
	m := newManagerWithRuntime(nil)
	_, err := m.namedConfig(&Spec{Driver: "nope/missing"}, "dev", "k8s", nil, false)
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrUnknownEmulatorDriver)
}

func TestManager_namedConfig_RootlessOverrideUnknownDriver(t *testing.T) {
	m := newManagerWithRuntime(nil)
	// rootless=true forces the RootlessOverride() path, which resolves the driver
	// again and surfaces the unknown-driver error.
	_, err := m.namedConfig(&Spec{Driver: "nope/missing"}, "dev", "k8s", nil, true)
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrUnknownEmulatorDriver)
}

func TestNewManager_DetectsRuntime(t *testing.T) {
	// NewManager has no injected runtime, so runtimeFor falls through to detection.
	// With an unsatisfiable preference and no auto-start, detection fails cleanly,
	// which exercises the NewManager constructor and the detection branch of
	// runtimeFor without requiring a real container runtime.
	m := NewManager("definitely-not-a-runtime", false)
	require.NotNil(t, m)
	_, err := m.Ps(context.Background(), "dev")
	require.Error(t, err)
}
