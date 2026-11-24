package signals

import (
	"context"
	"os"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// forwardRecorder records every signal a graceful participant receives.
type forwardRecorder struct {
	mu      sync.Mutex
	signals []os.Signal
}

func (r *forwardRecorder) forward(sig os.Signal) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.signals = append(r.signals, sig)
}

func (r *forwardRecorder) received() []os.Signal {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]os.Signal(nil), r.signals...)
}

// useDefaultGracefulStops installs a fresh production-shaped default registry for one test.
func useDefaultGracefulStops(t *testing.T) {
	t.Helper()
	previous := defaultGraceful
	defaultGraceful = NewGracefulStops(RequestExit, restartCleanupBudget)
	t.Cleanup(func() {
		defaultGraceful = previous
		pendingExitCode.Store(0)
	})
}

func TestGracefulStopForwardsTheFirstSignalOnceToEveryParticipant(t *testing.T) {
	var begun []os.Signal
	stops := NewGracefulStops(func(sig os.Signal) { begun = append(begun, sig) }, nil)
	var terraform, tofu forwardRecorder
	releaseTerraform := stops.Register("terraform", terraform.forward)
	defer releaseTerraform()
	releaseTofu := stops.Register("tofu", tofu.forward)
	defer releaseTofu()

	names, ok := stops.Begin(syscall.SIGTERM)
	require.True(t, ok, "running participants must defer the exit")
	assert.Equal(t, []string{"terraform", "tofu"}, names)
	assert.Equal(t, []os.Signal{syscall.SIGTERM}, begun, "the exit request is retained once")
	assert.Equal(t, []os.Signal{syscall.SIGTERM}, terraform.received())
	assert.Equal(t, []os.Signal{syscall.SIGTERM}, tofu.received())
	assert.True(t, stops.Stopping())

	// A second signal forces the exit and is never forwarded again.
	names, ok = stops.Begin(os.Interrupt)
	assert.False(t, ok, "the second signal must force the exit")
	assert.Nil(t, names)
	assert.Equal(t, []os.Signal{syscall.SIGTERM}, terraform.received(), "exactly one signal is forwarded")
	assert.Equal(t, []os.Signal{syscall.SIGTERM}, begun)
}

func TestGracefulStopWithoutParticipantsLeavesTheExitToTheCaller(t *testing.T) {
	begun := 0
	stops := NewGracefulStops(func(os.Signal) { begun++ }, nil)
	names, ok := stops.Begin(os.Interrupt)
	assert.False(t, ok)
	assert.Nil(t, names)
	assert.Zero(t, begun, "a stop without participants must not retain an exit request")
	assert.False(t, stops.Stopping())

	// The registry remains usable: a later run still gets the graceful treatment.
	var child forwardRecorder
	release := stops.Register("terraform", child.forward)
	defer release()
	_, ok = stops.Begin(os.Interrupt)
	assert.True(t, ok)
	assert.Equal(t, []os.Signal{os.Interrupt}, child.received())
}

func TestGracefulStopForwardsToParticipantsRegisteredAfterTheSignal(t *testing.T) {
	stops := NewGracefulStops(nil, nil)
	var first, late forwardRecorder
	releaseFirst := stops.Register("terraform", first.forward)
	defer releaseFirst()
	_, ok := stops.Begin(os.Interrupt)
	require.True(t, ok)

	// A child that started while the first signal was being handled must still be told to stop.
	releaseLate := stops.Register("terraform", late.forward)
	defer releaseLate()
	assert.Equal(t, []os.Signal{os.Interrupt}, late.received())
	assert.Equal(t, []os.Signal{os.Interrupt}, first.received(), "earlier participants are not signalled twice")
}

func TestGracefulStopWaitBlocksUntilEveryParticipantExits(t *testing.T) {
	drains := 0
	stops := NewGracefulStops(nil, func() { drains++ })
	var child forwardRecorder
	releaseFirst := stops.Register("terraform", child.forward)
	releaseSecond := stops.Register("terraform", child.forward)
	_, ok := stops.Begin(os.Interrupt)
	require.True(t, ok)

	waited := make(chan struct{})
	go func() {
		stops.Wait()
		close(waited)
	}()
	assertBlocked := func() {
		t.Helper()
		select {
		case <-waited:
			t.Fatal("Wait returned while a graceful participant was still running")
		case <-time.After(20 * time.Millisecond):
		}
	}
	assertBlocked()
	releaseFirst()
	releaseFirst()
	assertBlocked()
	releaseSecond()
	select {
	case <-waited:
	case <-time.After(time.Second):
		t.Fatal("Wait did not return after the last participant exited")
	}
	assert.Equal(t, 1, drains, "the drain callback runs once when the stop finishes")
	stops.Wait() // Returns at once when nothing is registered.
}

func TestGracefulStopDrainWithoutSignalDoesNotRunDrainCallback(t *testing.T) {
	drains := 0
	stops := NewGracefulStops(nil, func() { drains++ })
	release := stops.Register("terraform", func(os.Signal) {})
	release()
	assert.Zero(t, drains, "an ordinary exit is not the end of a graceful stop")
}

func TestGracefulStopWaitReturnsWhenTheSecondSignalForces(t *testing.T) {
	stops := NewGracefulStops(nil, nil)
	release := stops.Register("terraform", func(os.Signal) {})
	defer release()
	_, ok := stops.Begin(os.Interrupt)
	require.True(t, ok)

	waited := make(chan struct{})
	go func() {
		stops.Wait()
		close(waited)
	}()
	select {
	case <-waited:
		t.Fatal("Wait returned before the child exited or the exit was forced")
	case <-time.After(20 * time.Millisecond):
	}
	_, ok = stops.Begin(syscall.SIGTERM)
	require.False(t, ok)
	select {
	case <-waited:
	case <-time.After(time.Second):
		t.Fatal("a forced exit must stop waiting for the child")
	}
	stops.Wait() // A forced stop never blocks again.
}

func TestResolveExitCodeWaitsForGracefulStop(t *testing.T) {
	useDefaultGracefulStops(t)
	var child forwardRecorder
	release := DefaultGracefulStops().Register("terraform", child.forward)
	t.Cleanup(release)
	names, ok := BeginGracefulStop(os.Interrupt)
	require.True(t, ok)
	assert.Equal(t, []string{"terraform"}, names)
	require.True(t, ExitRequested(), "the first signal is retained as the exit status")

	exit := make(chan int, 1)
	go func() { exit <- ResolveExitCode(1) }()
	select {
	case code := <-exit:
		t.Fatalf("exited with %d while Terraform was still stopping", code)
	case <-time.After(20 * time.Millisecond):
	}
	release()
	select {
	case code := <-exit:
		assert.Equal(t, 130, code, "the interrupted exit code convention is preserved")
	case <-time.After(time.Second):
		t.Fatal("exit did not resume after the graceful stop finished")
	}
}

func TestGracefulStopRestartsTheCredentialCleanupBudget(t *testing.T) {
	useDefaultGracefulStops(t)
	release := DefaultGracefulStops().Register("terraform", func(os.Signal) {})
	t.Cleanup(release)
	_, ok := BeginGracefulStop(syscall.SIGTERM)
	require.True(t, ok)

	// Terraform may need longer than the cleanup budget to stop. The budget must
	// cover cleanup that runs after it exits instead of having expired meanwhile.
	cleanupScopes.Lock()
	cleanupScopes.deadline = time.Now().Add(-time.Minute)
	cleanupScopes.Unlock()
	release()
	cleanupScopes.Lock()
	deadline := cleanupScopes.deadline
	cleanupScopes.Unlock()
	assert.WithinDuration(t, time.Now().Add(maxCleanupShutdownWait), deadline, 5*time.Second)
	assert.Equal(t, 143, ResolveExitCode(0))
}

func TestRestartCleanupBudgetIgnoresUninterruptedRuns(t *testing.T) {
	cleanupScopes.Lock()
	before := cleanupScopes.deadline
	cleanupScopes.Unlock()
	restartCleanupBudget()
	cleanupScopes.Lock()
	after := cleanupScopes.deadline
	cleanupScopes.Unlock()
	assert.Equal(t, before, after)
}

func TestSignalContextRecordsTheSignalAsItsCause(t *testing.T) {
	received := make(chan os.Signal, 1)
	ctx, stop := cancelOnSignal(t.Context(), received)
	defer stop()
	derived, cancel := context.WithCancel(ctx)
	defer cancel()
	require.False(t, InterruptedBySignal(derived))

	received <- syscall.SIGTERM
	<-derived.Done()
	assert.True(t, InterruptedBySignal(ctx))
	assert.True(t, InterruptedBySignal(derived), "derived contexts inherit the signal cause")
	assert.ErrorIs(t, derived.Err(), context.Canceled)
}

func TestSignalContextIgnoresOtherCancellation(t *testing.T) {
	ctx, stop := cancelOnSignal(t.Context(), make(chan os.Signal))
	stop()
	<-ctx.Done()
	assert.False(t, InterruptedBySignal(ctx), "stopping the listener is not an interrupt")

	deadline, cancel := context.WithTimeout(t.Context(), time.Millisecond)
	defer cancel()
	<-deadline.Done()
	assert.False(t, InterruptedBySignal(deadline), "a timeout is not an interrupt")
	assert.False(t, InterruptedBySignal(t.Context()))
}

func TestInterruptContextStopsListening(t *testing.T) {
	ctx, stop := InterruptContext(t.Context())
	require.NoError(t, ctx.Err())
	stop()
	stop()
	<-ctx.Done()
	assert.False(t, InterruptedBySignal(ctx))
}
