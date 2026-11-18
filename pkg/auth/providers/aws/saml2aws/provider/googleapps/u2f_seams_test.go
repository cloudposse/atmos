package googleapps

import (
	"errors"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	u2fhost "github.com/marshallbrekka/go-u2fhost"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/auth/providers/aws/saml2aws/mocks"
)

// useU2FDevices makes device enumeration return the given devices for the duration of a test.
func useU2FDevices(t *testing.T, devices ...u2fhost.Device) {
	t.Helper()

	orig := listU2FDevices
	listU2FDevices = func() []u2fhost.Device { return devices }
	t.Cleanup(func() { listU2FDevices = orig })
}

func TestFindDevice(t *testing.T) {
	errBusy := errors.New("device busy")

	t.Run("no devices attached", func(t *testing.T) {
		useU2FDevices(t)
		dev, err := (&U2FDeviceFinder{}).findDevice()
		require.ErrorIs(t, err, errNoDeviceFound)
		assert.Nil(t, dev)
	})

	t.Run("first device that opens is returned", func(t *testing.T) {
		first := &mocks.U2FDevice{}
		first.On("Open").Return(nil).Once()
		second := &mocks.U2FDevice{}
		useU2FDevices(t, first, second)

		dev, err := (&U2FDeviceFinder{}).findDevice()
		require.NoError(t, err)
		assert.Same(t, first, dev)
		// The device that was not needed is never opened.
		second.AssertNotCalled(t, "Open")
		first.AssertExpectations(t)
	})

	t.Run("a device that fails to open is closed and the next one is used", func(t *testing.T) {
		broken := &mocks.U2FDevice{}
		broken.On("Open").Return(errBusy).Once()
		broken.On("Close").Return().Once()
		working := &mocks.U2FDevice{}
		working.On("Open").Return(nil).Once()
		useU2FDevices(t, broken, working)

		dev, err := (&U2FDeviceFinder{}).findDevice()
		require.NoError(t, err)
		assert.Same(t, working, dev)
		broken.AssertExpectations(t)
		working.AssertExpectations(t)
	})

	t.Run("every device failing to open reports the last error", func(t *testing.T) {
		a := &mocks.U2FDevice{}
		a.On("Open").Return(errors.New("first failure")).Once()
		a.On("Close").Return().Once()
		b := &mocks.U2FDevice{}
		b.On("Open").Return(errBusy).Once()
		b.On("Close").Return().Once()
		useU2FDevices(t, a, b)

		dev, err := (&U2FDeviceFinder{}).findDevice()
		require.ErrorIs(t, err, errBusy)
		assert.Contains(t, err.Error(), "failed to open fido U2F device")
		assert.Nil(t, dev)
		a.AssertExpectations(t)
		b.AssertExpectations(t)
	})
}

func TestNewU2FClientGivesUpAfterMaxRetries(t *testing.T) {
	orig := retryDelay
	retryDelay = time.Microsecond
	t.Cleanup(func() { retryDelay = orig })

	errBusy := errors.New("device busy")
	finder := &scriptedDeviceFinder{err: errBusy}
	client, err := NewU2FClient("nonce", "app", "facet", "kh", finder)
	require.ErrorIs(t, err, errBusy)
	assert.Contains(t, err.Error(), "exceeded max retries")
	assert.Nil(t, client)
	assert.Equal(t, maxOpenRetries, finder.calls)
}

func TestChallengeU2FTimesOutWithoutTouch(t *testing.T) {
	orig := u2fTouchTimeout
	u2fTouchTimeout = time.Millisecond
	t.Cleanup(func() { u2fTouchTimeout = orig })

	closed := make(chan struct{})
	device := &mocks.U2FDevice{}
	device.On("Close").Run(func(mock.Arguments) { close(closed) }).Return().Once()
	device.On("Authenticate", mock.Anything).Return(nil, &u2fhost.TestOfUserPresenceRequiredError{}).Maybe()

	client := &U2FClient{Device: device, ChallengeNonce: "dGVzdAo=", KeyHandle: "dGVzdAo="}
	got, err := client.ChallengeU2F()
	require.ErrorIs(t, err, errU2FTouchTimeout)
	assert.Contains(t, err.Error(), "failed to get authentication response")
	assert.Empty(t, got)
	// The worker that owns the device closes it after the timeout was reported.
	select {
	case <-closed:
	case <-time.After(time.Second):
		t.Fatal("device was not closed after the timeout")
	}
}

// useU2FTouchDeadline replaces the touch timeout with a channel the test closes to fire it.
func useU2FTouchDeadline(t *testing.T) chan time.Time {
	t.Helper()

	fire := make(chan time.Time)
	orig := u2fTouchDeadline
	u2fTouchDeadline = func() (<-chan time.Time, func()) { return fire, func() {} }
	t.Cleanup(func() { u2fTouchDeadline = orig })
	return fire
}

// Atmos fork: a key that stops answering mid-read must not block the caller past the touch timeout, and the
// handle must only be closed after that read returned.
func TestChallengeU2FTimeoutCoversBlockedRead(t *testing.T) {
	origPoll := u2fPollInterval                      // gitleaks:allow -- Test timing variables, not secrets.
	u2fPollInterval = time.Millisecond               // gitleaks:allow -- Test timing variables, not secrets.
	t.Cleanup(func() { u2fPollInterval = origPoll }) // gitleaks:allow -- Test timing variables, not secrets.
	fireTimeout := useU2FTouchDeadline(t)

	readStarted := make(chan struct{})
	releaseRead := make(chan struct{})
	var closedDuringRead atomic.Bool
	var readReturned atomic.Bool
	closed := make(chan struct{})

	device := &mocks.U2FDevice{}
	device.On("Authenticate", mock.Anything).Run(func(mock.Arguments) {
		close(readStarted)
		<-releaseRead
		readReturned.Store(true)
	}).Return(nil, errors.New("read aborted")).Once()
	device.On("Close").Run(func(mock.Arguments) {
		if !readReturned.Load() {
			closedDuringRead.Store(true)
		}
		close(closed)
	}).Return().Once()

	client := &U2FClient{Device: device, ChallengeNonce: "dGVzdAo=", KeyHandle: "dGVzdAo="}
	result := make(chan error, 1)
	go func() {
		_, err := client.ChallengeU2F()
		result <- err
	}()

	// The timeout fires only once the read is in flight.
	select {
	case <-readStarted:
	case <-time.After(5 * time.Second):
		t.Fatal("the device read did not start")
	}
	close(fireTimeout)
	select {
	case err := <-result:
		require.ErrorIs(t, err, errU2FTouchTimeout)
	case <-time.After(5 * time.Second):
		t.Fatal("ChallengeU2F did not return after the timeout while the read was blocked")
	}

	// The read is still blocked, so the handle must still be open.
	select {
	case <-closed:
		t.Fatal("device was closed while a read was in flight")
	case <-time.After(20 * time.Millisecond):
	}

	close(releaseRead)
	select {
	case <-closed:
	case <-time.After(time.Second):
		t.Fatal("device was not closed after the read returned")
	}
	assert.False(t, closedDuringRead.Load())
}

// Atmos fork: HID initialization is bounded separately; the worker that owns the pending Open closes the
// handle when it finally returns.
func TestFindDeviceBoundsBlockedOpen(t *testing.T) {
	orig := u2fOpenTimeout
	u2fOpenTimeout = 20 * time.Millisecond
	t.Cleanup(func() { u2fOpenTimeout = orig })

	releaseOpen := make(chan struct{})
	closed := make(chan struct{})
	hung := &mocks.U2FDevice{}
	hung.On("Open").Run(func(mock.Arguments) { <-releaseOpen }).Return(nil).Once()
	hung.On("Close").Run(func(mock.Arguments) { close(closed) }).Return().Once()
	useU2FDevices(t, hung)

	dev, err := (&U2FDeviceFinder{}).findDevice()
	require.ErrorIs(t, err, errU2FOpenTimeout)
	assert.Nil(t, dev)

	select {
	case <-closed:
		t.Fatal("device was closed before its Open returned")
	default:
	}

	close(releaseOpen)
	select {
	case <-closed:
	case <-time.After(time.Second):
		t.Fatal("device was not closed after Open returned")
	}
}

// Regression: an open timeout on one device followed by a different failure on the next one reported only the
// later failure, so NewU2FClient retried discovery and left more blocked Open calls behind.
func TestFindDeviceKeepsOpenTimeoutWhenALaterDeviceFails(t *testing.T) {
	orig := u2fOpenTimeout
	u2fOpenTimeout = 20 * time.Millisecond
	t.Cleanup(func() { u2fOpenTimeout = orig })

	errBusy := errors.New("device busy")
	hung, release := newHungU2FDevice(t)
	busy := &mocks.U2FDevice{}
	busy.On("Open").Return(errBusy).Once()
	busy.On("Close").Return().Once()
	useU2FDevices(t, hung, busy)
	t.Cleanup(release)

	dev, err := (&U2FDeviceFinder{}).findDevice()
	assert.Nil(t, dev)
	require.ErrorIs(t, err, errU2FOpenTimeout)
	require.ErrorIs(t, err, errBusy)
	busy.AssertExpectations(t)

	// NewU2FClient takes its non-retryable path for the combined error.
	finder := &scriptedDeviceFinder{err: err}
	_, err = NewU2FClient("nonce", "app", "facet", "kh", finder)
	require.ErrorIs(t, err, errU2FOpenTimeout)
	assert.Equal(t, 1, finder.calls)
}

// A device that opens after an earlier device timed out is still used.
func TestFindDeviceUsesALaterDeviceAfterOpenTimeout(t *testing.T) {
	orig := u2fOpenTimeout
	u2fOpenTimeout = 20 * time.Millisecond
	t.Cleanup(func() { u2fOpenTimeout = orig })

	hung, release := newHungU2FDevice(t)
	working := &mocks.U2FDevice{}
	working.On("Open").Return(nil).Once()
	useU2FDevices(t, hung, working)
	t.Cleanup(release)

	dev, err := (&U2FDeviceFinder{}).findDevice()
	require.NoError(t, err)
	assert.Same(t, working, dev)
}

// newHungU2FDevice returns a device whose Open blocks until release is called. Release unblocks Open and
// waits, bounded, for the worker that owns the pending Open to close the device.
func newHungU2FDevice(t *testing.T) (*mocks.U2FDevice, func()) {
	t.Helper()

	releaseOpen := make(chan struct{})
	closed := make(chan struct{})
	hung := &mocks.U2FDevice{}
	hung.On("Open").Run(func(mock.Arguments) { <-releaseOpen }).Return(nil).Once()
	hung.On("Close").Run(func(mock.Arguments) { close(closed) }).Return().Once()
	release := func() {
		close(releaseOpen)
		select {
		case <-closed:
		case <-time.After(5 * time.Second):
			t.Error("the timed-out device was not closed after its Open returned")
		}
	}
	return hung, release
}

// Regression: when stop and the poll ticker were both ready, the worker could pick the ticker and start
// another blocking read after the timeout had already been reported.
func TestAuthenticateUntilStoppedDoesNotReadAfterStop(t *testing.T) {
	origPoll := u2fPollInterval                      // gitleaks:allow -- Test timing variables, not secrets.
	u2fPollInterval = time.Millisecond               // gitleaks:allow -- Test timing variables, not secrets.
	t.Cleanup(func() { u2fPollInterval = origPoll }) // gitleaks:allow -- Test timing variables, not secrets.

	// select chooses at random between ready cases, so repeat to make a missing recheck show up.
	for range 20 {
		var reads atomic.Int32
		stop := make(chan struct{})
		device := &mocks.U2FDevice{}
		device.On("Authenticate", mock.Anything).Run(func(mock.Arguments) {
			if reads.Add(1) == 1 {
				close(stop)
				// Let several poll intervals elapse so a tick is pending next to stop when the loop selects
				// again. This only lets the ticker advance; it does not synchronize goroutines.
				<-time.After(10 * u2fPollInterval)
			}
		}).Return(nil, &u2fhost.TestOfUserPresenceRequiredError{})

		client := &U2FClient{Device: device}
		_, err := client.authenticateUntilStopped(&u2fhost.AuthenticateRequest{}, stop)
		require.ErrorIs(t, err, errU2FTouchTimeout)
		require.Equal(t, int32(1), reads.Load(), "a read started after stop was closed")
	}
}

func TestNewU2FClientDoesNotRetryOpenTimeout(t *testing.T) {
	finder := &scriptedDeviceFinder{err: fmt.Errorf("wrapped: %w", errU2FOpenTimeout)}
	_, err := NewU2FClient("nonce", "app", "facet", "kh", finder)
	require.ErrorIs(t, err, errU2FOpenTimeout)
	assert.Equal(t, 1, finder.calls)
}

func TestChallengeU2FInvalidKeyHandle(t *testing.T) {
	device := &mocks.U2FDevice{}
	device.On("Close").Return().Once()

	client := &U2FClient{Device: device, ChallengeNonce: "dGVzdAo=", KeyHandle: "not base64!"}
	_, err := client.ChallengeU2F()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid U2F key handle")
	device.AssertExpectations(t)
}
