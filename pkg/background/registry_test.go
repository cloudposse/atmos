package background

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeHandle is a test double recording WaitReady/Stop calls.
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

func TestRegistry_RegisterGetNames(t *testing.T) {
	reg := NewRegistry()
	a := &fakeHandle{name: "a"}
	b := &fakeHandle{name: "b"}
	reg.Register(a)
	reg.Register(b)

	got, ok := reg.Get("a")
	require.True(t, ok)
	assert.Same(t, a, got)

	_, ok = reg.Get("missing")
	assert.False(t, ok)

	// Names preserve registration order.
	assert.Equal(t, []string{"a", "b"}, reg.Names())
}

func TestRegistry_RemoveDropsHandle(t *testing.T) {
	reg := NewRegistry()
	reg.Register(&fakeHandle{name: "a"})
	reg.Register(&fakeHandle{name: "b"})

	reg.Remove("a")
	_, ok := reg.Get("a")
	assert.False(t, ok)
	assert.Equal(t, []string{"b"}, reg.Names())
}

func TestRegistry_StopAllStopsEveryHandleAndClears(t *testing.T) {
	reg := NewRegistry()
	a := &fakeHandle{name: "a"}
	b := &fakeHandle{name: "b"}
	reg.Register(a)
	reg.Register(b)

	require.NoError(t, reg.StopAll(context.Background()))
	assert.Equal(t, 1, a.stopCalls)
	assert.Equal(t, 1, b.stopCalls)
	// Registry is drained after StopAll so a later auto-teardown is a no-op.
	assert.Empty(t, reg.Names())
}

func TestRegistry_StopAllJoinsErrorsAndStillStopsRest(t *testing.T) {
	reg := NewRegistry()
	boom := errors.New("boom")
	a := &fakeHandle{name: "a", stopErr: boom}
	b := &fakeHandle{name: "b"}
	reg.Register(a)
	reg.Register(b)

	err := reg.StopAll(context.Background())
	require.Error(t, err)
	assert.ErrorIs(t, err, boom)
	// b is still stopped despite a failing (no early return).
	assert.Equal(t, 1, b.stopCalls)
	assert.Empty(t, reg.Names())
}
