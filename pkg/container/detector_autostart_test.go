package container

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

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
	gomock.InOrder(
		m.EXPECT().CommandContext(gomock.Any(), "podman", "info").Return(failCmd()),
		m.EXPECT().CommandContext(gomock.Any(), "podman", "info").Return(successCmd()),
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
