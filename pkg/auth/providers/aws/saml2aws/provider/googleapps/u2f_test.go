package googleapps

import (
	"fmt"
	"testing"

	u2fhost "github.com/marshallbrekka/go-u2fhost"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/cloudposse/atmos/pkg/auth/providers/aws/saml2aws/mocks"
)

type fidoClientTests struct {
	title string
	err   error
}

type MockDeviceFinder struct {
	device *mocks.U2FDevice
}

func (m *MockDeviceFinder) findDevice() (u2fhost.Device, error) {
	return m.device, nil
}

func TestNewU2FClient(t *testing.T) {
	challengeNonce := "challengeNonce"
	appID := "appID"
	version := "version"
	keyHandle := "keyHandle"
	tests := []fidoClientTests{
		{
			title: "Returns new client successfully if device found",
			err:   nil,
		},
	}

	for _, test := range tests {
		t.Run(test.title, func(t *testing.T) {
			_, err := NewU2FClient(challengeNonce, appID, version, keyHandle, &MockDeviceFinder{&mocks.U2FDevice{}})
			if test.err != nil {
				assert.NotNil(t, err)
				assert.Equal(t, test.err.Error(), err.Error())
			}
		})
	}
}

func TestChallengeU2F(t *testing.T) {
	challengeNonce := "dGVzdAo="
	appID := "appID"
	facet := "facet"
	keyHandle := "dGVzdAo="
	tests := []fidoClientTests{
		{
			title: "Returns signed assertion from device",
			err:   nil,
		},
	}

	for _, test := range tests {
		t.Run(test.title, func(t *testing.T) {
			device := &mocks.U2FDevice{}
			mockDeviceFinder := &MockDeviceFinder{device}
			device.On("Open").Return(nil)
			request := &u2fhost.AuthenticateRequest{
				Challenge:          "dGVzdAo",
				AppId:              appID,
				Facet:              facet,
				KeyHandle:          "dGVzdAo",
				ChannelIdPublicKey: nil,
				ChannelIdUnused:    false,
				CheckOnly:          false,
				WebAuthn:           false,
			}
			response := &u2fhost.AuthenticateResponse{}
			device.On("Authenticate", request).Return(response, nil)
			device.On("Close").Return(nil)
			client, _ := NewU2FClient(challengeNonce, appID, facet, keyHandle, mockDeviceFinder)
			_, err := client.ChallengeU2F()
			if test.err != nil {
				assert.NotNil(t, err)
				assert.Equal(t, test.err.Error(), err.Error())
			}
		})
	}
}

// scriptedDeviceFinder returns err for the first errCount calls (all calls when errCount is 0 and
// err is set), then the device.
type scriptedDeviceFinder struct {
	err      error
	errCount int
	device   *mocks.U2FDevice
	calls    int
}

func (s *scriptedDeviceFinder) findDevice() (u2fhost.Device, error) {
	s.calls++
	if s.err != nil && (s.errCount == 0 || s.calls <= s.errCount) {
		return nil, s.err
	}
	return s.device, nil
}

// withDeviceFinder makes the U2F challenge use the given finder instead of real hardware.
func withDeviceFinder(t *testing.T, finder DeviceFinder) {
	t.Helper()

	orig := newU2FDeviceFinder
	newU2FDeviceFinder = func() DeviceFinder { return finder }
	t.Cleanup(func() { newU2FDeviceFinder = orig })
}

func TestNewU2FClient_DeviceDiscovery(t *testing.T) {
	t.Run("no device plugged in fails immediately", func(t *testing.T) {
		finder := &scriptedDeviceFinder{err: errNoDeviceFound}
		_, err := NewU2FClient("nonce", "app", "facet", "kh", finder)
		assert.ErrorIs(t, err, errNoDeviceFound)
		assert.Equal(t, 1, finder.calls)
	})

	t.Run("transient open error is retried", func(t *testing.T) {
		device := &mocks.U2FDevice{}
		finder := &scriptedDeviceFinder{err: fmt.Errorf("device busy"), errCount: 1, device: device}
		client, err := NewU2FClient("nonce", "app", "facet", "kh", finder)
		assert.NoError(t, err)
		assert.Equal(t, 2, finder.calls)
		assert.Same(t, device, client.Device)
		assert.Equal(t, "facet", client.Facet)
	})
}

func TestU2FClientChallengeU2F_Behavior(t *testing.T) {
	t.Run("missing device", func(t *testing.T) {
		_, err := (&U2FClient{}).ChallengeU2F()
		assert.EqualError(t, err, "No Device Found")
	})

	t.Run("waits for user presence then returns the JSON response", func(t *testing.T) {
		device := &mocks.U2FDevice{}
		device.On("Close").Return().Once()
		device.On("Authenticate", mock.Anything).Return(nil, &u2fhost.TestOfUserPresenceRequiredError{}).Once()
		device.On("Authenticate", mock.Anything).Return(&u2fhost.AuthenticateResponse{ClientData: "cd", SignatureData: "sig"}, nil).Once()

		client := &U2FClient{Device: device, ChallengeNonce: "dGVzdAo=", KeyHandle: "dGVzdAo="}
		got, err := client.ChallengeU2F()
		assert.NoError(t, err)
		assert.JSONEq(t, `{"clientData":"cd","signatureData":"sig","keyHandle":""}`, got)
		device.AssertExpectations(t)
	})

	t.Run("device error is returned", func(t *testing.T) {
		device := &mocks.U2FDevice{}
		device.On("Close").Return().Once()
		device.On("Authenticate", mock.Anything).Return(nil, fmt.Errorf("device unplugged")).Once()

		client := &U2FClient{Device: device, ChallengeNonce: "dGVzdAo=", KeyHandle: "dGVzdAo="}
		_, err := client.ChallengeU2F()
		assert.EqualError(t, err, "device unplugged")
	})

	// Atmos fork: upstream panicked on an invalid challenge; it now returns an error.
	t.Run("invalid base64 challenge returns an error", func(t *testing.T) {
		device := &mocks.U2FDevice{}
		device.On("Close").Return().Once()
		client := &U2FClient{Device: device, ChallengeNonce: "not base64!", KeyHandle: "dGVzdAo="}
		_, err := client.ChallengeU2F()
		assert.ErrorContains(t, err, "invalid U2F challenge nonce")
		device.AssertExpectations(t)
	})
}
