package okta

import (
	"fmt"
	"testing"

	"github.com/marshallbrekka/go-u2fhost"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

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
		setup func(device *mocks.MockU2FDevice)
		check func(t *testing.T, resp *ResponseData, err error)
	}{
		{
			name: "error",
			setup: func(device *mocks.MockU2FDevice) {
				device.EXPECT().Authenticate(request).Return(nil, &u2fhost.BadKeyHandleError{}).Times(1)
			},
			check: func(t *testing.T, resp *ResponseData, err error) {
				assert.Nil(t, resp)
				var badKeyErr *u2fhost.BadKeyHandleError
				assert.ErrorAs(t, err, &badKeyErr)
			},
		},
		{
			name: "retry",
			setup: func(device *mocks.MockU2FDevice) {
				device.EXPECT().Authenticate(request).Return(nil, &u2fhost.TestOfUserPresenceRequiredError{}).Times(1)
				device.EXPECT().Authenticate(request).Return(response, nil).Times(1)
			},
			check: func(t *testing.T, resp *ResponseData, err error) {
				assert.NoError(t, err)
				assert.NotNil(t, resp)
			},
		},
		{
			name: "success",
			setup: func(device *mocks.MockU2FDevice) {
				device.EXPECT().Authenticate(request).Return(response, nil).Times(1)
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
			ctrl := gomock.NewController(t)
			device := mocks.NewMockU2FDevice(ctrl)
			// The challenge closes its own device exactly once, and every scripted read is used: the
			// controller verifies both when the subtest ends.
			device.EXPECT().Close().Times(1)
			tc.setup(device)

			client, err := NewDUOU2FClient(challengeNonce, appID, version, keyHandle, stateToken, finderFor(ctrl, device))
			require.NoError(t, err)
			require.Same(t, device, client.Device)

			resp, err := client.ChallengeU2F()
			tc.check(t, resp, err)
		})
	}
}

func TestNewDUOU2FClient_DeviceDiscovery(t *testing.T) {
	t.Run("no device plugged in fails immediately", func(t *testing.T) {
		// Exactly one discovery attempt; the controller verifies the count.
		finder := failingFinder(gomock.NewController(t), errNoDeviceFound)
		_, err := NewDUOU2FClient("nonce", "app", "v", "kh", "st", finder)
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
		client, err := NewDUOU2FClient("nonce", "app", "v", "kh", "st", finder)
		assert.NoError(t, err)
		assert.Same(t, device, client.Device)
		assert.Equal(t, "app", client.AppID)
	})

	t.Run("missing device", func(t *testing.T) {
		client := &DUOU2FClient{}
		_, err := client.ChallengeU2F()
		assert.EqualError(t, err, "no device found")
	})
}
