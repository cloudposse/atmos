package container

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	execpkg "github.com/cloudposse/atmos/pkg/exec"
)

func TestAutoStartFromEnv(t *testing.T) {
	t.Setenv(envContainerRuntimeAutoStart, "true")
	assert.True(t, autoStartFromEnv())

	t.Setenv(envContainerRuntimeAutoStart, "1")
	assert.True(t, autoStartFromEnv())

	t.Setenv(envContainerRuntimeAutoStart, "")
	assert.False(t, autoStartFromEnv())
}

// TestDetectRuntimeWithPreferenceAndRecovery_AutoProviderRecoversPodman verifies
// provider: auto takes the same recovery path as an omitted provider. Docker is
// absent and the Podman machine is stopped, so detection starts it and succeeds.
func TestDetectRuntimeWithPreferenceAndRecovery_AutoProviderRecoversPodman(t *testing.T) {
	t.Setenv(envContainerRuntime, "")           // no explicit selector → auto-detect.
	t.Setenv(envContainerRuntimeAutoStart, "1") // the feature flag under test.

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	m := execpkg.NewMockCommandExecutor(ctrl)

	m.EXPECT().LookPath("docker").Return("", errors.New("not found")).AnyTimes()
	m.EXPECT().LookPath("podman").Return("/usr/bin/podman", nil).AnyTimes()

	// `podman info`: fails first (machine stopped), then succeeds after recovery.
	// The successful recovery result is reused; detection does not probe it again.
	gomock.InOrder(
		m.EXPECT().CommandContext(gomock.Any(), "podman", "info").Return(failCmd()),
		m.EXPECT().CommandContext(gomock.Any(), "podman", "info").Return(successCmd()),
	)
	// A machine exists → NeedsStart → recovery runs `podman machine start` (asserted).
	m.EXPECT().CommandContext(gomock.Any(), "podman", "machine", "list", "--format", "{{.Name}}").
		Return(echoCmd("podman-machine-default")).Times(1)
	m.EXPECT().CommandContext(gomock.Any(), "podman", "machine", "start").
		Return(successCmd()).Times(1)

	setExecutor(m)
	defer resetExecutor()

	rt, err := DetectRuntimeWithPreferenceAndRecovery(context.Background(), string(TypeAuto), false)
	require.NoError(t, err)
	assert.Equal(t, TypePodman, GetRuntimeType(rt))
}

// TestDetectRuntimeWithPreferenceAndRecovery_ExplicitPodmanRecovers verifies an
// explicit `provider: podman` preference (not auto/empty) still takes the
// recovery-and-reuse path: TryRecoverPodmanRuntime's own result is reused
// instead of a second `podman info` in DetectRuntimeWithPreference.
func TestDetectRuntimeWithPreferenceAndRecovery_ExplicitPodmanRecovers(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	m := execpkg.NewMockCommandExecutor(ctrl)

	// `podman info`: fails first (machine stopped), then succeeds after recovery.
	gomock.InOrder(
		m.EXPECT().CommandContext(gomock.Any(), "podman", "info").Return(failCmd()),
		m.EXPECT().CommandContext(gomock.Any(), "podman", "info").Return(successCmd()),
	)
	m.EXPECT().LookPath("podman").Return("/usr/bin/podman", nil).AnyTimes()
	m.EXPECT().CommandContext(gomock.Any(), "podman", "machine", "list", "--format", "{{.Name}}").
		Return(echoCmd("podman-machine-default")).Times(1)
	m.EXPECT().CommandContext(gomock.Any(), "podman", "machine", "start").
		Return(successCmd()).Times(1)

	setExecutor(m)
	defer resetExecutor()

	rt, err := DetectRuntimeWithPreferenceAndRecovery(context.Background(), string(TypePodman), true)
	require.NoError(t, err)
	assert.Equal(t, TypePodman, GetRuntimeType(rt))
}

// TestDetectRuntimeWithPreferenceAndRecovery_ExplicitPodmanRecoveryFails is the
// negative counterpart: when recovery cannot bring Podman up, the explicit
// `provider: podman` branch returns an error without falling back to Docker.
func TestDetectRuntimeWithPreferenceAndRecovery_ExplicitPodmanRecoveryFails(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	m := execpkg.NewMockCommandExecutor(ctrl)

	// Podman binary is entirely missing: TryRecoverPodmanRuntime's status check
	// short-circuits to RuntimeUnavailable via the `default` case (no recovery attempted).
	m.EXPECT().LookPath("podman").Return("", errors.New("not found")).AnyTimes()

	setExecutor(m)
	defer resetExecutor()

	rt, err := DetectRuntimeWithPreferenceAndRecovery(context.Background(), string(TypePodman), true)
	require.Error(t, err)
	require.ErrorIs(t, err, errUtils.ErrRuntimeNotAvailable)
	assert.Nil(t, rt)
}

// TestDetectRuntimeWithPreferenceAndRecovery_AutoDockerAvailableSkipsPodman
// verifies the auto/empty-selection branch returns Docker directly, without
// probing or attempting to recover Podman at all, when Docker is available.
func TestDetectRuntimeWithPreferenceAndRecovery_AutoDockerAvailableSkipsPodman(t *testing.T) {
	t.Setenv(envContainerRuntime, "")
	t.Setenv(envContainerRuntimeAutoStart, "1")

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	m := execpkg.NewMockCommandExecutor(ctrl)

	m.EXPECT().LookPath("docker").Return("/usr/bin/docker", nil).Times(1)
	m.EXPECT().CommandContext(gomock.Any(), "docker", "info").Return(successCmd()).Times(1)
	// No podman expectations at all: gomock fails the test if Podman is probed.

	setExecutor(m)
	defer resetExecutor()

	rt, err := DetectRuntimeWithPreferenceAndRecovery(context.Background(), "", true)
	require.NoError(t, err)
	assert.Equal(t, TypeDocker, GetRuntimeType(rt))
}

// TestDetectRuntimeWithPreferenceAndRecovery_AutoNeitherAvailable verifies the
// auto/empty-selection branch's final error path: neither Docker nor a
// recovered Podman is available.
func TestDetectRuntimeWithPreferenceAndRecovery_AutoNeitherAvailable(t *testing.T) {
	t.Setenv(envContainerRuntime, "")
	t.Setenv(envContainerRuntimeAutoStart, "1")

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	m := execpkg.NewMockCommandExecutor(ctrl)

	m.EXPECT().LookPath("docker").Return("", errors.New("not found")).Times(1)
	m.EXPECT().LookPath("podman").Return("", errors.New("not found")).AnyTimes()

	setExecutor(m)
	defer resetExecutor()

	rt, err := DetectRuntimeWithPreferenceAndRecovery(context.Background(), "", true)
	require.Error(t, err)
	require.ErrorIs(t, err, errUtils.ErrRuntimeNotAvailable)
	assert.Nil(t, rt)
}

// TestDetectRuntimeWithPreferenceAndRecovery_NoFlagSkipsAutoStart is the negative
// counterpart: with the flag off and autoStart arg false, a stopped Podman machine
// must NOT be started (no `podman machine start` expectation), and detection fails.
func TestDetectRuntimeWithPreferenceAndRecovery_NoFlagSkipsAutoStart(t *testing.T) {
	t.Setenv(envContainerRuntime, "")
	t.Setenv(envContainerRuntimeAutoStart, "")

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	m := execpkg.NewMockCommandExecutor(ctrl)

	m.EXPECT().LookPath("docker").Return("", errors.New("not found")).AnyTimes()
	m.EXPECT().LookPath("podman").Return("/usr/bin/podman", nil).AnyTimes()
	m.EXPECT().CommandContext(gomock.Any(), "podman", "info").Return(failCmd()).AnyTimes()
	m.EXPECT().CommandContext(gomock.Any(), "podman", "machine", "list", "--format", "{{.Name}}").
		Return(echoCmd("podman-machine-default")).AnyTimes()
	// No `podman machine start` expectation: gomock fails the test if recovery runs it.

	setExecutor(m)
	defer resetExecutor()

	_, err := DetectRuntimeWithPreferenceAndRecovery(context.Background(), "", false)
	require.Error(t, err)
}
