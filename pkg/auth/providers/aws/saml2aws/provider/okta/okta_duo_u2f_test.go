package okta

import (
	"fmt"
	"testing"

	"github.com/marshallbrekka/go-u2fhost"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/auth/providers/aws/saml2aws/mocks"
)

func TestChallengeDuoU2F(t *testing.T) {
	challengeNonce := "challengeNonce"
	appID := "appID"
	version := "version"
	keyHandle := "keyHandle"
	stateToken := "stateToken"

	request := &u2fhost.AuthenticateRequest{
		Challenge: challengeNonce,
		Facet:     appID,
		AppId:     appID,
		KeyHandle: keyHandle,
		WebAuthn:  false,
	}

	clientData := "exampleClientDat"
	signatureData := "exampleSignatureData"

	response := &u2fhost.AuthenticateResponse{
		ClientData:    clientData,
		SignatureData: signatureData,
	}

	// Each challenge closes its device, so every subtest opens a fresh device and client.
	tests := []struct {
		name  string
		setup func(device *mocks.U2FDevice)
		check func(t *testing.T, resp *ResponseData, err error)
	}{
		{
			name: "error",
			setup: func(device *mocks.U2FDevice) {
				device.On("Authenticate", request).Return(nil, &u2fhost.BadKeyHandleError{}).Once()
			},
			check: func(t *testing.T, resp *ResponseData, err error) {
				assert.Nil(t, resp)
				var badKeyErr *u2fhost.BadKeyHandleError
				assert.ErrorAs(t, err, &badKeyErr)
			},
		},
		{
			name: "retry",
			setup: func(device *mocks.U2FDevice) {
				device.On("Authenticate", request).Return(nil, &u2fhost.TestOfUserPresenceRequiredError{}).Once()
				device.On("Authenticate", request).Return(response, nil).Once()
			},
			check: func(t *testing.T, resp *ResponseData, err error) {
				assert.NoError(t, err)
				assert.NotNil(t, resp)
			},
		},
		{
			name: "success",
			setup: func(device *mocks.U2FDevice) {
				device.On("Authenticate", request).Return(response, nil).Once()
			},
			check: func(t *testing.T, resp *ResponseData, err error) {
				require.NoError(t, err)
				assert.Equal(t, stateToken, resp.SessionId)
				assert.Equal(t, clientData, resp.ClientData)
				assert.Equal(t, signatureData, resp.SignatureData)
				assert.Equal(t, keyHandle, resp.KeyHandle)
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			device := &mocks.U2FDevice{}
			device.On("Close").Return().Once()
			tc.setup(device)

			client, err := NewDUOU2FClient(challengeNonce, appID, version, keyHandle, stateToken, &MockDeviceFinder{device})
			require.NoError(t, err)
			require.Same(t, device, client.Device)

			resp, err := client.ChallengeU2F()
			tc.check(t, resp, err)
			// The challenge closes its own device exactly once, and every scripted read was used.
			device.AssertExpectations(t)
			device.AssertNumberOfCalls(t, "Close", 1)
		})
	}
}

func TestNewDUOU2FClient_DeviceDiscovery(t *testing.T) {
	t.Run("no device plugged in fails immediately", func(t *testing.T) {
		finder := &scriptedDeviceFinder{errs: []error{errNoDeviceFound}}
		_, err := NewDUOU2FClient("nonce", "app", "v", "kh", "st", finder)
		assert.ErrorIs(t, err, errNoDeviceFound)
		assert.Equal(t, 1, finder.calls)
	})

	t.Run("transient open error is retried", func(t *testing.T) {
		device := &mocks.U2FDevice{}
		finder := &scriptedDeviceFinder{errs: []error{fmt.Errorf("device busy")}, device: device}
		client, err := NewDUOU2FClient("nonce", "app", "v", "kh", "st", finder)
		assert.NoError(t, err)
		assert.Equal(t, 2, finder.calls)
		assert.Same(t, device, client.Device)
		assert.Equal(t, "app", client.AppID)
	})

	t.Run("missing device", func(t *testing.T) {
		client := &DUOU2FClient{}
		_, err := client.ChallengeU2F()
		assert.EqualError(t, err, "no device found")
	})
}
