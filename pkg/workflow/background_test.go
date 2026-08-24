package workflow

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/background"
	"github.com/cloudposse/atmos/pkg/schema"
)

// newMockHandle returns a MockHandle whose Name() always reports the given name
// (the registry queries it on register/lookup). Callers add WaitReady/Stop
// expectations per test.
func newMockHandle(ctrl *gomock.Controller, name string) *MockHandle {
	h := NewMockHandle(ctrl)
	h.EXPECT().Name().Return(name).AnyTimes()
	return h
}

func TestStartBackground_RegistersWithoutGating(t *testing.T) {
	ctrl := gomock.NewController(t)
	reg := background.NewRegistry()
	handle := newMockHandle(ctrl, "emulator")
	// WaitReady is intentionally NOT expected: Start is non-blocking, so readiness is
	// only checked later by the implicit gate. gomock fails if WaitReady is called.
	step := &schema.WorkflowStep{Name: "emulator", Type: "container", BackgroundAsync: true}

	var gotEnv []string
	runner := NewMockRunner(ctrl)
	runner.EXPECT().Start(gomock.Any(), step, gomock.Any()).
		DoAndReturn(func(_ context.Context, _ *schema.WorkflowStep, env []string) (background.Handle, error) {
			gotEnv = env
			return handle, nil
		})

	require.NoError(t, StartBackground(context.Background(), reg, runner, step, []string{"K=V"}))

	_, ok := reg.Get("emulator")
	require.True(t, ok, "the handle must be registered")
	assert.Equal(t, []string{"K=V"}, gotEnv)
}

func TestStartBackground_PropagatesStartError(t *testing.T) {
	ctrl := gomock.NewController(t)
	reg := background.NewRegistry()
	runner := NewMockRunner(ctrl)
	runner.EXPECT().Start(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, errors.New("boom"))

	err := StartBackground(context.Background(), reg, runner, &schema.WorkflowStep{Name: "emulator", BackgroundAsync: true}, nil)
	require.Error(t, err)
	assert.Empty(t, reg.Names(), "a failed start must not register a handle")
}

func TestWaitBackground_ReadiesNamedAndSurfacesError(t *testing.T) {
	ctrl := gomock.NewController(t)
	reg := background.NewRegistry()
	bad := newMockHandle(ctrl, "bad")
	bad.EXPECT().WaitReady(gomock.Any()).Return(errors.New("unhealthy"))
	reg.Register(bad)

	// Explicit wait surfaces a failed readiness.
	err := WaitBackground(context.Background(), reg, []string{"bad"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unhealthy")

	// Unknown target is reported, not silently ignored.
	err = WaitBackground(context.Background(), reg, []string{"ghost"})
	require.Error(t, err)
	assert.ErrorIs(t, err, schema.ErrWorkflowControlStepInvalid)
}

func TestCancelBackground_StopsAndRemoves(t *testing.T) {
	ctrl := gomock.NewController(t)
	reg := background.NewRegistry()
	emu := newMockHandle(ctrl, "emulator")
	// Stop exactly once: Cancel stops+removes it, so the later StopAll must not stop again.
	emu.EXPECT().Stop(gomock.Any()).Return(nil).Times(1)
	reg.Register(emu)

	require.NoError(t, CancelBackground(context.Background(), reg, []string{"emulator"}))
	assert.Empty(t, reg.Names(), "removed from the registry, so end-of-scope StopAll won't stop it again")

	require.NoError(t, reg.StopAll(context.Background()))
}

func TestWaitAllBackground_ReadiesEveryRegistered(t *testing.T) {
	ctrl := gomock.NewController(t)
	reg := background.NewRegistry()
	for _, name := range []string{"a", "b", "c"} {
		h := newMockHandle(ctrl, name)
		// Start is non-blocking, so wait-all is what readies each (exactly once).
		h.EXPECT().WaitReady(gomock.Any()).Return(nil).Times(1)
		reg.Register(h)
	}

	require.NoError(t, WaitAllBackground(context.Background(), reg))
}

func TestCancelBackground_KeepsHandleRegisteredOnFailedStop(t *testing.T) {
	ctrl := gomock.NewController(t)
	// Negative path: a failed Stop must NOT remove the handle, so the deferred
	// StopAll at workflow exit can retry teardown.
	reg := background.NewRegistry()
	emu := newMockHandle(ctrl, "emulator")
	emu.EXPECT().Stop(gomock.Any()).Return(errors.New("down failed"))
	reg.Register(emu)

	err := CancelBackground(context.Background(), reg, []string{"emulator"})
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrContainerRuntimeOperation)
	assert.Equal(t, []string{"emulator"}, reg.Names(), "failed stop keeps the handle for StopAll retry")
}

func TestGatePendingBackground_GatesOnceThenSkips(t *testing.T) {
	ctrl := gomock.NewController(t)
	reg := background.NewRegistry()
	for _, name := range []string{"a", "b"} {
		h := newMockHandle(ctrl, name)
		// Times(1) enforces that the second gate does NOT re-probe an already-gated service.
		h.EXPECT().WaitReady(gomock.Any()).Return(nil).Times(1)
		reg.Register(h)
	}

	gated := map[string]bool{}
	require.NoError(t, GatePendingBackground(context.Background(), reg, gated))
	assert.True(t, gated["a"] && gated["b"], "gated services are recorded")

	// Second gate is a no-op for already-gated services (no re-probe).
	require.NoError(t, GatePendingBackground(context.Background(), reg, gated))
}

func TestGatePendingBackground_NilRegistryIsNoOp(t *testing.T) {
	// The documented contract: a nil registry is a no-op (must not panic on reg.Names()).
	require.NoError(t, GatePendingBackground(context.Background(), nil, map[string]bool{}))
}

func TestGatePendingBackground_SurfacesUnhealthyAndLeavesUngated(t *testing.T) {
	ctrl := gomock.NewController(t)
	reg := background.NewRegistry()
	bad := newMockHandle(ctrl, "bad")
	bad.EXPECT().WaitReady(gomock.Any()).Return(errors.New("unhealthy"))
	reg.Register(bad)

	gated := map[string]bool{}
	err := GatePendingBackground(context.Background(), reg, gated)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unhealthy")
	assert.False(t, gated["bad"], "a failed gate must not mark the service ready")
}
