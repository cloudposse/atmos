package workflow

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/background"
	"github.com/cloudposse/atmos/pkg/schema"
)

type fakeHandle struct {
	name       string
	readyErr   error
	stopErr    error
	readyCalls int
	stopCalls  int
}

func (h *fakeHandle) Name() string                      { return h.name }
func (h *fakeHandle) WaitReady(_ context.Context) error { h.readyCalls++; return h.readyErr }
func (h *fakeHandle) Stop(_ context.Context) error      { h.stopCalls++; return h.stopErr }

type fakeRunner struct {
	handles   map[string]*fakeHandle
	lastEnv   []string
	startErr  error
	readyErrs map[string]error
}

func (r *fakeRunner) Start(_ context.Context, step *schema.WorkflowStep, env []string) (background.Handle, error) {
	if r.startErr != nil {
		return nil, r.startErr
	}
	r.lastEnv = env
	h := &fakeHandle{name: step.Name, readyErr: r.readyErrs[step.Name]}
	if r.handles == nil {
		r.handles = map[string]*fakeHandle{}
	}
	r.handles[step.Name] = h
	return h, nil
}

func TestStartBackground_RegistersWithoutGating(t *testing.T) {
	reg := background.NewRegistry()
	runner := &fakeRunner{}
	step := &schema.WorkflowStep{Name: "emulator", Type: "container", BackgroundAsync: true}

	require.NoError(t, StartBackground(context.Background(), reg, runner, step, []string{"K=V"}))

	// Start is non-blocking: the handle is registered but readiness is NOT yet
	// checked (the implicit gate fires before the next foreground step).
	h, ok := reg.Get("emulator")
	require.True(t, ok)
	assert.Equal(t, 0, h.(*fakeHandle).readyCalls)
	assert.Equal(t, []string{"K=V"}, runner.lastEnv)
}

func TestStartBackground_PropagatesStartError(t *testing.T) {
	reg := background.NewRegistry()
	runner := &fakeRunner{startErr: errors.New("boom")}
	step := &schema.WorkflowStep{Name: "emulator", BackgroundAsync: true}

	err := StartBackground(context.Background(), reg, runner, step, nil)
	require.Error(t, err)
	assert.Empty(t, reg.Names(), "a failed start must not register a handle")
}

func TestWaitBackground_ReadiesNamedAndSurfacesError(t *testing.T) {
	// Register handles directly to isolate WaitBackground from the start-time gate.
	reg := background.NewRegistry()
	reg.Register(&fakeHandle{name: "bad", readyErr: errors.New("unhealthy")})

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
	reg := background.NewRegistry()
	runner := &fakeRunner{}
	require.NoError(t, StartBackground(context.Background(), reg, runner, &schema.WorkflowStep{Name: "emulator"}, nil))

	require.NoError(t, CancelBackground(context.Background(), reg, []string{"emulator"}))
	assert.Equal(t, 1, runner.handles["emulator"].stopCalls)
	// Removed from the registry, so the end-of-scope StopAll won't stop it again.
	assert.Empty(t, reg.Names())

	require.NoError(t, reg.StopAll(context.Background()))
	assert.Equal(t, 1, runner.handles["emulator"].stopCalls)
}

func TestWaitAllBackground_ReadiesEveryRegistered(t *testing.T) {
	reg := background.NewRegistry()
	runner := &fakeRunner{}
	for _, name := range []string{"a", "b", "c"} {
		require.NoError(t, StartBackground(context.Background(), reg, runner, &schema.WorkflowStep{Name: name}, nil))
	}
	// Start is non-blocking, so wait-all is what readies each (exactly once).
	require.NoError(t, WaitAllBackground(context.Background(), reg))
	for _, name := range []string{"a", "b", "c"} {
		assert.Equal(t, 1, runner.handles[name].readyCalls)
	}
}

func TestCancelBackground_KeepsHandleRegisteredOnFailedStop(t *testing.T) {
	// Negative path: a failed Stop must NOT remove the handle, so the deferred
	// StopAll at workflow exit can retry teardown.
	reg := background.NewRegistry()
	reg.Register(&fakeHandle{name: "emulator", stopErr: errors.New("down failed")})

	err := CancelBackground(context.Background(), reg, []string{"emulator"})
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrContainerRuntimeOperation)
	assert.Equal(t, []string{"emulator"}, reg.Names(), "failed stop keeps the handle for StopAll retry")
}

func TestGatePendingBackground_GatesOnceThenSkips(t *testing.T) {
	reg := background.NewRegistry()
	runner := &fakeRunner{}
	for _, name := range []string{"a", "b"} {
		require.NoError(t, StartBackground(context.Background(), reg, runner, &schema.WorkflowStep{Name: name}, nil))
	}

	gated := map[string]bool{}
	// First gate readies both and records them.
	require.NoError(t, GatePendingBackground(context.Background(), reg, gated))
	assert.Equal(t, 1, runner.handles["a"].readyCalls)
	assert.Equal(t, 1, runner.handles["b"].readyCalls)
	assert.True(t, gated["a"] && gated["b"])

	// Second gate is a no-op for already-gated services (no re-probe).
	require.NoError(t, GatePendingBackground(context.Background(), reg, gated))
	assert.Equal(t, 1, runner.handles["a"].readyCalls)
	assert.Equal(t, 1, runner.handles["b"].readyCalls)
}

func TestGatePendingBackground_NilRegistryIsNoOp(t *testing.T) {
	// The documented contract: a nil registry is a no-op (must not panic on reg.Names()).
	require.NoError(t, GatePendingBackground(context.Background(), nil, map[string]bool{}))
}

func TestGatePendingBackground_SurfacesUnhealthyAndLeavesUngated(t *testing.T) {
	reg := background.NewRegistry()
	reg.Register(&fakeHandle{name: "bad", readyErr: errors.New("unhealthy")})

	gated := map[string]bool{}
	err := GatePendingBackground(context.Background(), reg, gated)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unhealthy")
	assert.False(t, gated["bad"], "a failed gate must not mark the service ready")
}
