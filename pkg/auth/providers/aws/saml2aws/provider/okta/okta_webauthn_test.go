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
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/auth/providers/aws/saml2aws/mocks"
)

type MockDeviceFinder struct {
	device *mocks.U2FDevice
}

func (m *MockDeviceFinder) findDevice() (u2fhost.Device, error) {
	return m.device, nil
}

func TestNewFidoClient(t *testing.T) {
	device := &mocks.U2FDevice{}

	client, err := NewFidoClient("challengeNonce", "appID", "version", "keyHandle", "stateToken", &MockDeviceFinder{device})
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

	device := &mocks.U2FDevice{}
	device.On("Open").Return(nil)
	device.On("Authenticate", request).Return(response, nil)
	device.On("Close").Return(nil)
	client, err := NewFidoClient("challengeNonce", "appID", "version", "keyHandle", "stateToken", &MockDeviceFinder{device})
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

// scriptedDeviceFinder returns the configured errors in order, then the device.
type scriptedDeviceFinder struct {
	errs   []error
	device *mocks.U2FDevice
	calls  int
}

func (s *scriptedDeviceFinder) findDevice() (u2fhost.Device, error) {
	s.calls++
	if len(s.errs) > 0 {
		err := s.errs[0]
		s.errs = s.errs[1:]
		return nil, err
	}
	return s.device, nil
}

func TestNewFidoClient_DeviceDiscovery(t *testing.T) {
	t.Run("no device plugged in fails immediately", func(t *testing.T) {
		finder := &scriptedDeviceFinder{errs: []error{errNoDeviceFound}}
		_, err := NewFidoClient("nonce", "app", "v", "kh", "st", finder)
		assert.ErrorIs(t, err, errNoDeviceFound)
		assert.Equal(t, 1, finder.calls)
	})

	t.Run("transient open error is retried", func(t *testing.T) {
		device := &mocks.U2FDevice{}
		finder := &scriptedDeviceFinder{errs: []error{fmt.Errorf("device busy")}, device: device}
		client, err := NewFidoClient("nonce", "app", "v", "kh", "st", finder)
		assert.NoError(t, err)
		assert.Equal(t, 2, finder.calls)
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
		device := &mocks.U2FDevice{}
		device.On("Close").Return().Once()
		device.On("Authenticate", mock.Anything).Return(nil, &u2fhost.TestOfUserPresenceRequiredError{}).Once()
		device.On("Authenticate", mock.Anything).Return(nil, fmt.Errorf("U2FHIDError: transient")).Once()
		device.On("Authenticate", mock.Anything).Return(&u2fhost.AuthenticateResponse{
			ClientData: "cd", SignatureData: "sig", AuthenticatorData: "ad",
		}, nil).Once()

		client := &FidoClient{Device: device, StateToken: "st", AppID: "okta.example.com"}
		got, err := client.ChallengeU2F()
		assert.NoError(t, err)
		assert.Equal(t, &SignedAssertion{StateToken: "st", ClientData: "cd", SignatureData: "sig", AuthenticatorData: "ad"}, got)
		device.AssertExpectations(t)
	})

	t.Run("unexpected device error is returned", func(t *testing.T) {
		device := &mocks.U2FDevice{}
		device.On("Close").Return().Once()
		device.On("Authenticate", mock.Anything).Return(nil, fmt.Errorf("device unplugged")).Once()

		client := &FidoClient{Device: device}
		got, err := client.ChallengeU2F()
		assert.EqualError(t, err, "device unplugged")
		assert.Nil(t, got)
		device.AssertExpectations(t)
	})
}

// A device that keeps asking for a touch times out between polls with the same sentinel as a hung read.
func TestFidoClientChallengeU2F_PresenceTimeoutBetweenPolls(t *testing.T) {
	savedTimeout, savedPoll := u2fAuthTimeout, u2fPollInterval                      // gitleaks:allow -- Test timing variables, not secrets.
	u2fAuthTimeout, u2fPollInterval = 50*time.Millisecond, 5*time.Millisecond       // gitleaks:allow -- Test timing variables, not secrets.
	t.Cleanup(func() { u2fAuthTimeout, u2fPollInterval = savedTimeout, savedPoll }) // gitleaks:allow -- Test timing variables, not secrets.

	closed := make(chan struct{})
	device := &mocks.U2FDevice{}
	device.On("Authenticate", mock.Anything).Return(nil, &u2fhost.TestOfUserPresenceRequiredError{})
	device.On("Close").Run(func(mock.Arguments) { close(closed) }).Return().Once()

	client := &FidoClient{Device: device}
	got, err := client.ChallengeU2F()
	require.ErrorIs(t, err, ErrU2FAuthTimeout)
	assert.Nil(t, got)
	device.AssertCalled(t, "Authenticate", mock.Anything)

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
	origTimeout, origInterval := u2fAuthTimeout, u2fPollInterval
	u2fAuthTimeout, u2fPollInterval = 100*time.Millisecond, 5*time.Millisecond // gitleaks:allow -- Test timing variables, not secrets.
	t.Cleanup(func() { u2fAuthTimeout, u2fPollInterval = origTimeout, origInterval })

	synctest.Test(t, func(t *testing.T) {
		release := make(chan struct{})
		var releaseOnce sync.Once
		releaseRead := func() { releaseOnce.Do(func() { close(release) }) }
		// Release the read exactly once, including when an assertion below fails.
		defer releaseRead()

		var closed atomic.Bool
		device := &mocks.U2FDevice{}
		device.On("Authenticate", mock.Anything).Run(func(mock.Arguments) { <-release }).Return(nil, fmt.Errorf("device unplugged")).Once()
		device.On("Close").Run(func(mock.Arguments) { closed.Store(true) }).Return().Once()

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
		device.AssertExpectations(t)
	})
}

// Negative path: a device that answers in time is closed by ChallengeU2F itself and the call does not time out.
func TestFidoClientChallengeU2F_ResponsiveDeviceDoesNotTimeOut(t *testing.T) {
	origInterval := u2fPollInterval
	u2fPollInterval = time.Millisecond
	t.Cleanup(func() { u2fPollInterval = origInterval })

	device := &mocks.U2FDevice{}
	device.On("Authenticate", mock.Anything).Return(&u2fhost.AuthenticateResponse{ClientData: "cd"}, nil).Once()
	device.On("Close").Return().Once()

	got, err := (&FidoClient{Device: device, StateToken: "st"}).ChallengeU2F()
	require.NoError(t, err)
	assert.Equal(t, "cd", got.ClientData)
	device.AssertExpectations(t)
}

// Regression: the system WebAuthn timeout is in milliseconds; two seconds was too short for a PIN or
// biometric prompt, so it must outlast the USB touch timeout.
func TestSystemWebAuthnTimeoutOutlastsUSBTimeout(t *testing.T) {
	assert.GreaterOrEqual(t, int64(systemWebAuthnTimeoutMillis), int64(25*time.Second/time.Millisecond))
}
