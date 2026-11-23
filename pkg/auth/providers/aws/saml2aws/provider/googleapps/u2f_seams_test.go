package googleapps

import (
	"errors"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	u2fhost "github.com/marshallbrekka/go-u2fhost"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

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
		ctrl := gomock.NewController(t)
		first := mocks.NewMockU2FDevice(ctrl)
		first.EXPECT().Open().Return(nil).Times(1)
		// The device that was not needed is never opened: it has no expectations, so any call fails the test.
		second := mocks.NewMockU2FDevice(ctrl)
		useU2FDevices(t, first, second)

		dev, err := (&U2FDeviceFinder{}).findDevice()
		require.NoError(t, err)
		assert.Same(t, first, dev)
	})

	t.Run("a device that fails to open is closed and the next one is used", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		broken := mocks.NewMockU2FDevice(ctrl)
		broken.EXPECT().Open().Return(errBusy).Times(1)
		broken.EXPECT().Close().Times(1)
		working := mocks.NewMockU2FDevice(ctrl)
		working.EXPECT().Open().Return(nil).Times(1)
		useU2FDevices(t, broken, working)

		dev, err := (&U2FDeviceFinder{}).findDevice()
		require.NoError(t, err)
		assert.Same(t, working, dev)
	})

	t.Run("every device failing to open reports the last error", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		a := mocks.NewMockU2FDevice(ctrl)
		a.EXPECT().Open().Return(errors.New("first failure")).Times(1)
		a.EXPECT().Close().Times(1)
		b := mocks.NewMockU2FDevice(ctrl)
		b.EXPECT().Open().Return(errBusy).Times(1)
		b.EXPECT().Close().Times(1)
		useU2FDevices(t, a, b)

		dev, err := (&U2FDeviceFinder{}).findDevice()
		require.ErrorIs(t, err, errBusy)
		assert.Contains(t, err.Error(), "failed to open fido U2F device")
		assert.Nil(t, dev)
	})
}

func TestNewU2FClientGivesUpAfterMaxRetries(t *testing.T) {
	orig := retryDelay
	retryDelay = time.Microsecond
	t.Cleanup(func() { retryDelay = orig })

	errBusy := errors.New("device busy")
	// Discovery is attempted exactly maxOpenRetries times; the controller verifies the count.
	finder := failingFinder(gomock.NewController(t), errBusy, maxOpenRetries)
	client, err := NewU2FClient("nonce", "app", "facet", "kh", finder)
	require.ErrorIs(t, err, errBusy)
	assert.Contains(t, err.Error(), "exceeded max retries")
	assert.Nil(t, client)
}

func TestChallengeU2FTimesOutWithoutTouch(t *testing.T) {
	orig := u2fTouchTimeout
	u2fTouchTimeout = time.Millisecond
	t.Cleanup(func() { u2fTouchTimeout = orig })

	closed := make(chan struct{})
	device := mocks.NewMockU2FDevice(gomock.NewController(t))
	device.EXPECT().Close().Do(func() { close(closed) }).Times(1)
	device.EXPECT().Authenticate(gomock.Any()).Return(nil, &u2fhost.TestOfUserPresenceRequiredError{}).AnyTimes()

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

	device := mocks.NewMockU2FDevice(gomock.NewController(t))
	device.EXPECT().Authenticate(gomock.Any()).Do(func(*u2fhost.AuthenticateRequest) {
		close(readStarted)
		<-releaseRead
		readReturned.Store(true)
	}).Return(nil, errors.New("read aborted")).Times(1)
	device.EXPECT().Close().Do(func() {
		if !readReturned.Load() {
			closedDuringRead.Store(true)
		}
		close(closed)
	}).Times(1)

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
	hung := mocks.NewMockU2FDevice(gomock.NewController(t))
	hung.EXPECT().Open().Do(func() { <-releaseOpen }).Return(nil).Times(1)
	hung.EXPECT().Close().Do(func() { close(closed) }).Times(1)
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
	busy := mocks.NewMockU2FDevice(gomock.NewController(t))
	busy.EXPECT().Open().Return(errBusy).Times(1)
	busy.EXPECT().Close().Times(1)
	useU2FDevices(t, hung, busy)
	t.Cleanup(release)

	dev, err := (&U2FDeviceFinder{}).findDevice()
	assert.Nil(t, dev)
	require.ErrorIs(t, err, errU2FOpenTimeout)
	require.ErrorIs(t, err, errBusy)

	// NewU2FClient takes its non-retryable path for the combined error: exactly one discovery attempt.
	finder := failingFinder(gomock.NewController(t), err, 1)
	_, err = NewU2FClient("nonce", "app", "facet", "kh", finder)
	require.ErrorIs(t, err, errU2FOpenTimeout)
}

// A device that opens after an earlier device timed out is still used.
func TestFindDeviceUsesALaterDeviceAfterOpenTimeout(t *testing.T) {
	orig := u2fOpenTimeout
	u2fOpenTimeout = 20 * time.Millisecond
	t.Cleanup(func() { u2fOpenTimeout = orig })

	hung, release := newHungU2FDevice(t)
	working := mocks.NewMockU2FDevice(gomock.NewController(t))
	working.EXPECT().Open().Return(nil).Times(1)
	useU2FDevices(t, hung, working)
	t.Cleanup(release)

	dev, err := (&U2FDeviceFinder{}).findDevice()
	require.NoError(t, err)
	assert.Same(t, working, dev)
}

// newHungU2FDevice returns a device whose Open blocks until release is called. Release unblocks Open and
// waits, bounded, for the worker that owns the pending Open to close the device.
// The device's controller is created here, so a release registered with t.Cleanup afterwards runs before
// the controller verifies the expectations.
func newHungU2FDevice(t *testing.T) (*mocks.MockU2FDevice, func()) {
	t.Helper()

	releaseOpen := make(chan struct{})
	closed := make(chan struct{})
	hung := mocks.NewMockU2FDevice(gomock.NewController(t))
	hung.EXPECT().Open().Do(func() { <-releaseOpen }).Return(nil).Times(1)
	hung.EXPECT().Close().Do(func() { close(closed) }).Times(1)
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
		device := mocks.NewMockU2FDevice(gomock.NewController(t))
		device.EXPECT().Authenticate(gomock.Any()).Do(func(*u2fhost.AuthenticateRequest) {
			if reads.Add(1) == 1 {
				close(stop)
				// Let several poll intervals elapse so a tick is pending next to stop when the loop selects
				// again. This only lets the ticker advance; it does not synchronize goroutines.
				<-time.After(10 * u2fPollInterval)
			}
		}).Return(nil, &u2fhost.TestOfUserPresenceRequiredError{}).AnyTimes() // The read count is asserted below.

		client := &U2FClient{Device: device}
		_, err := client.authenticateUntilStopped(&u2fhost.AuthenticateRequest{}, stop)
		require.ErrorIs(t, err, errU2FTouchTimeout)
		require.Equal(t, int32(1), reads.Load(), "a read started after stop was closed")
	}
}

func TestNewU2FClientDoesNotRetryOpenTimeout(t *testing.T) {
	// Exactly one discovery attempt; the controller verifies the count.
	finder := failingFinder(gomock.NewController(t), fmt.Errorf("wrapped: %w", errU2FOpenTimeout), 1)
	_, err := NewU2FClient("nonce", "app", "facet", "kh", finder)
	require.ErrorIs(t, err, errU2FOpenTimeout)
}

func TestChallengeU2FInvalidKeyHandle(t *testing.T) {
	device := mocks.NewMockU2FDevice(gomock.NewController(t))
	device.EXPECT().Close().Times(1)

	client := &U2FClient{Device: device, ChallengeNonce: "dGVzdAo=", KeyHandle: "not base64!"}
	_, err := client.ChallengeU2F()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid U2F key handle")
}
