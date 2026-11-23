package okta

import (
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	u2fhost "github.com/marshallbrekka/go-u2fhost"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/cloudposse/atmos/pkg/auth/providers/aws/saml2aws/mocks"
)

// finderFor returns a DeviceFinder mock that finds the given device exactly once.
func finderFor(ctrl *gomock.Controller, device u2fhost.Device) *MockDeviceFinder {
	finder := NewMockDeviceFinder(ctrl)
	finder.EXPECT().findDevice().Return(device, nil).Times(1)
	return finder
}

// anyTimesFinderFor returns a DeviceFinder mock that finds the given device on every lookup. The flows under
// test may look the device up any number of times; the device mock's own expectations verify its use.
func anyTimesFinderFor(ctrl *gomock.Controller, device u2fhost.Device) *MockDeviceFinder {
	finder := NewMockDeviceFinder(ctrl)
	finder.EXPECT().findDevice().Return(device, nil).AnyTimes()
	return finder
}

// failingFinder returns a DeviceFinder mock whose findDevice fails with err exactly once.
func failingFinder(ctrl *gomock.Controller, err error) *MockDeviceFinder {
	finder := NewMockDeviceFinder(ctrl)
	finder.EXPECT().findDevice().Return(nil, err).Times(1)
	return finder
}

func TestNewFidoClient(t *testing.T) {
	ctrl := gomock.NewController(t)
	device := mocks.NewMockU2FDevice(ctrl)

	client, err := NewFidoClient("challengeNonce", "appID", "version", "keyHandle", "stateToken", finderFor(ctrl, device))
	require.NoError(t, err)
	assert.Same(t, device, client.Device)
	assert.Equal(t, "challengeNonce", client.ChallengeNonce)
	assert.Equal(t, "appID", client.AppID)
	assert.Equal(t, "version", client.Version)
	assert.Equal(t, "keyHandle", client.KeyHandle)
	assert.Equal(t, "stateToken", client.StateToken)
}

func TestChallengeWebAuthnU2F(t *testing.T) {
	request := &u2fhost.AuthenticateRequest{
		Challenge:          "challengeNonce",
		AppId:              "appID",
		Facet:              "https://appID",
		KeyHandle:          "keyHandle",
		ChannelIdPublicKey: nil,
		ChannelIdUnused:    false,
		CheckOnly:          false,
		WebAuthn:           true,
	}
	response := &u2fhost.AuthenticateResponse{
		ClientData:        "exampleClientData",
		SignatureData:     "exampleSignatureData",
		AuthenticatorData: "exampleAuthenticatorData",
	}

	ctrl := gomock.NewController(t)
	device := mocks.NewMockU2FDevice(ctrl)
	// The finder owns opening the device, so the client never calls Open; it is allowed but not required.
	device.EXPECT().Open().Return(nil).AnyTimes()
	device.EXPECT().Authenticate(request).Return(response, nil).Times(1)
	device.EXPECT().Close().Times(1)
	client, err := NewFidoClient("challengeNonce", "appID", "version", "keyHandle", "stateToken", finderFor(ctrl, device))
	require.NoError(t, err)

	got, err := client.ChallengeU2F()
	require.NoError(t, err)
	assert.Equal(t, &SignedAssertion{
		StateToken:        "stateToken", // gitleaks:allow -- Fake state token in a test fixture.
		ClientData:        "exampleClientData",
		SignatureData:     "exampleSignatureData",
		AuthenticatorData: "exampleAuthenticatorData",
	}, got)
}

func TestNewFidoClient_DeviceDiscovery(t *testing.T) {
	t.Run("no device plugged in fails immediately", func(t *testing.T) {
		// Exactly one discovery attempt; the controller verifies the count.
		finder := failingFinder(gomock.NewController(t), errNoDeviceFound)
		_, err := NewFidoClient("nonce", "app", "v", "kh", "st", finder)
		assert.ErrorIs(t, err, errNoDeviceFound)
	})

	t.Run("transient open error is retried", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		device := mocks.NewMockU2FDevice(ctrl)
		finder := NewMockDeviceFinder(ctrl)
		// The first call fails and the second finds the device: exactly two calls, in order.
		gomock.InOrder(
			finder.EXPECT().findDevice().Return(nil, fmt.Errorf("device busy")).Times(1),
			finder.EXPECT().findDevice().Return(device, nil).Times(1),
		)
		client, err := NewFidoClient("nonce", "app", "v", "kh", "st", finder)
		assert.NoError(t, err)
		assert.Same(t, device, client.Device)
		assert.Equal(t, "kh", client.KeyHandle)
		assert.Equal(t, "st", client.StateToken)
	})
}

func TestFidoClientChallengeU2F_Behavior(t *testing.T) {
	t.Run("missing device", func(t *testing.T) {
		client := &FidoClient{}
		_, err := client.ChallengeU2F()
		assert.EqualError(t, err, "no device found")
	})

	t.Run("waits for user presence and HID errors before succeeding", func(t *testing.T) {
		device := mocks.NewMockU2FDevice(gomock.NewController(t))
		device.EXPECT().Close().Times(1)
		device.EXPECT().Authenticate(gomock.Any()).Return(nil, &u2fhost.TestOfUserPresenceRequiredError{}).Times(1)
		device.EXPECT().Authenticate(gomock.Any()).Return(nil, fmt.Errorf("U2FHIDError: transient")).Times(1)
		device.EXPECT().Authenticate(gomock.Any()).Return(&u2fhost.AuthenticateResponse{
			ClientData: "cd", SignatureData: "sig", AuthenticatorData: "ad",
		}, nil).Times(1)

		client := &FidoClient{Device: device, StateToken: "st", AppID: "okta.example.com"}
		got, err := client.ChallengeU2F()
		assert.NoError(t, err)
		assert.Equal(t, &SignedAssertion{StateToken: "st", ClientData: "cd", SignatureData: "sig", AuthenticatorData: "ad"}, got)
	})

	t.Run("unexpected device error is returned", func(t *testing.T) {
		device := mocks.NewMockU2FDevice(gomock.NewController(t))
		device.EXPECT().Close().Times(1)
		device.EXPECT().Authenticate(gomock.Any()).Return(nil, fmt.Errorf("device unplugged")).Times(1)

		client := &FidoClient{Device: device}
		got, err := client.ChallengeU2F()
		assert.EqualError(t, err, "device unplugged")
		assert.Nil(t, got)
	})
}

// A device that keeps asking for a touch times out between polls with the same sentinel as a hung read.
func TestFidoClientChallengeU2F_PresenceTimeoutBetweenPolls(t *testing.T) {
	savedTimeout, savedPoll := u2fAuthTimeout, u2fPollInterval                      // gitleaks:allow -- Test timing variables, not secrets.
	u2fAuthTimeout, u2fPollInterval = 50*time.Millisecond, 5*time.Millisecond       // gitleaks:allow -- Test timing variables, not secrets.
	t.Cleanup(func() { u2fAuthTimeout, u2fPollInterval = savedTimeout, savedPoll }) // gitleaks:allow -- Test timing variables, not secrets.

	closed := make(chan struct{})
	device := mocks.NewMockU2FDevice(gomock.NewController(t))
	// Polled until the timeout, so at least once; the controller verifies the minimum when the test ends.
	device.EXPECT().Authenticate(gomock.Any()).Return(nil, &u2fhost.TestOfUserPresenceRequiredError{}).MinTimes(1)
	device.EXPECT().Close().Do(func() { close(closed) }).Times(1)

	client := &FidoClient{Device: device}
	got, err := client.ChallengeU2F()
	require.ErrorIs(t, err, ErrU2FAuthTimeout)
	assert.Nil(t, got)

	// The timeout can fire between polls (closed on return) or during a read (closed once the read
	// returns), so wait for the single close either way.
	select {
	case <-closed:
	case <-time.After(5 * time.Second):
		t.Fatal("device was never closed")
	}
}

// Regression: a device whose Authenticate read never returns must not block ChallengeU2F past its timeout,
// and the device must not be closed while that read is still in flight.
// The test runs in a synctest bubble: time is virtual, a regression that waits on the blocked read fails as
// a deadlock instead of hanging, and synctest.Wait lets every runnable goroutine settle before the checks.
func TestFidoClientChallengeU2F_HungDeviceTimesOut(t *testing.T) {
	origTimeout, origInterval := u2fAuthTimeout, u2fPollInterval                      // gitleaks:allow -- Test timing variables, not secrets.
	u2fAuthTimeout, u2fPollInterval = 100*time.Millisecond, 5*time.Millisecond        // gitleaks:allow -- Test timing variables, not secrets.
	t.Cleanup(func() { u2fAuthTimeout, u2fPollInterval = origTimeout, origInterval }) // gitleaks:allow -- Restore test timing variables.

	synctest.Test(t, func(t *testing.T) {
		release := make(chan struct{})
		var releaseOnce sync.Once
		releaseRead := func() { releaseOnce.Do(func() { close(release) }) }
		// Release the read exactly once, including when an assertion below fails.
		defer releaseRead()

		var closed atomic.Bool
		// The controller belongs to the bubble's t, so it verifies the expectations inside the bubble.
		device := mocks.NewMockU2FDevice(gomock.NewController(t))
		device.EXPECT().Authenticate(gomock.Any()).Do(func(*u2fhost.AuthenticateRequest) { <-release }).Return(nil, fmt.Errorf("device unplugged")).Times(1)
		device.EXPECT().Close().Do(func() { closed.Store(true) }).Times(1)

		got, err := (&FidoClient{Device: device}).ChallengeU2F()
		require.ErrorIs(t, err, ErrU2FAuthTimeout)
		assert.Contains(t, err.Error(), "failed to get authentication response")
		assert.Nil(t, got)

		// The read is still blocked: let all runnable goroutines settle, then the device must still be open.
		synctest.Wait()
		assert.False(t, closed.Load(), "device was closed while its Authenticate read was still in flight")

		// Once the blocked read returns, the abandoned call closes the device exactly once.
		releaseRead()
		synctest.Wait()
		assert.True(t, closed.Load(), "device was never closed after the blocked read returned")
	})
}

// Negative path: a device that answers in time is closed by ChallengeU2F itself and the call does not time out.
func TestFidoClientChallengeU2F_ResponsiveDeviceDoesNotTimeOut(t *testing.T) {
	origInterval := u2fPollInterval
	u2fPollInterval = time.Millisecond
	t.Cleanup(func() { u2fPollInterval = origInterval })

	device := mocks.NewMockU2FDevice(gomock.NewController(t))
	device.EXPECT().Authenticate(gomock.Any()).Return(&u2fhost.AuthenticateResponse{ClientData: "cd"}, nil).Times(1)
	device.EXPECT().Close().Times(1)

	got, err := (&FidoClient{Device: device, StateToken: "st"}).ChallengeU2F()
	require.NoError(t, err)
	assert.Equal(t, "cd", got.ClientData)
}

// Regression: the system WebAuthn timeout is in milliseconds; two seconds was too short for a PIN or
// biometric prompt, so it must outlast the USB touch timeout.
func TestSystemWebAuthnTimeoutOutlastsUSBTimeout(t *testing.T) {
	assert.GreaterOrEqual(t, int64(systemWebAuthnTimeoutMillis), int64(25*time.Second/time.Millisecond))
}
