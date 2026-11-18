package okta

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

func TestNewFidoClient(t *testing.T) {
	challengeNonce := "challengeNonce"
	appID := "appID"
	version := "version"
	keyHandle := "keyHandle"
	stateToken := "stateToken"
	tests := []fidoClientTests{
		{
			title: "Returns new client successfully if device found",
			err:   nil,
		},
	}

	for _, test := range tests {
		t.Run(test.title, func(t *testing.T) {
			_, err := NewFidoClient(challengeNonce, appID, version, keyHandle, stateToken, &MockDeviceFinder{&mocks.U2FDevice{}})
			if test.err != nil {
				assert.NotNil(t, err)
				assert.Equal(t, test.err.Error(), err.Error())
			}
		})
	}
}

func TestChallengeWebAuthnU2F(t *testing.T) {
	challengeNonce := "challengeNonce"
	appID := "appID"
	version := "version"
	keyHandle := "keyHandle"
	stateToken := "stateToken"
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
				Challenge:          challengeNonce,
				AppId:              appID,
				Facet:              "https://" + appID,
				KeyHandle:          keyHandle,
				ChannelIdPublicKey: nil,
				ChannelIdUnused:    false,
				CheckOnly:          false,
				WebAuthn:           true,
			}
			response := &u2fhost.AuthenticateResponse{}
			device.On("Authenticate", request).Return(response, nil)
			device.On("Close").Return(nil)
			client, _ := NewFidoClient(challengeNonce, appID, version, keyHandle, stateToken, mockDeviceFinder)
			_, err := client.ChallengeU2F()
			if test.err != nil {
				assert.NotNil(t, err)
				assert.Equal(t, test.err.Error(), err.Error())
			}
		})
	}
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
