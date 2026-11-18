package googleapps

import (
	"fmt"
	"testing"

	u2fhost "github.com/marshallbrekka/go-u2fhost"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/cloudposse/atmos/pkg/auth/providers/aws/saml2aws/mocks"
)

type fidoClientTests struct {
	title string
	err   error
}

// finderFor returns a DeviceFinder mock that finds the given device exactly once.
func finderFor(ctrl *gomock.Controller, device u2fhost.Device) *MockDeviceFinder {
	finder := NewMockDeviceFinder(ctrl)
	finder.EXPECT().findDevice().Return(device, nil).Times(1)
	return finder
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
			ctrl := gomock.NewController(t)
			device := mocks.NewMockU2FDevice(ctrl)
			client, err := NewU2FClient(challengeNonce, appID, version, keyHandle, finderFor(ctrl, device))
			if test.err != nil {
				require.Error(t, err)
				assert.Equal(t, test.err.Error(), err.Error())
				return
			}
			require.NoError(t, err)
			require.NotNil(t, client)
			assert.Same(t, device, client.Device)
			assert.Equal(t, challengeNonce, client.ChallengeNonce)
			assert.Equal(t, appID, client.AppID)
			assert.Equal(t, version, client.Facet)
			assert.Equal(t, keyHandle, client.KeyHandle)
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
			ctrl := gomock.NewController(t)
			device := mocks.NewMockU2FDevice(ctrl)
			mockDeviceFinder := finderFor(ctrl, device)
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
			response := &u2fhost.AuthenticateResponse{ClientData: "cd", SignatureData: "sig"}
			device.EXPECT().Authenticate(request).Return(response, nil).Times(1)
			device.EXPECT().Close().Times(1)
			client, err := NewU2FClient(challengeNonce, appID, facet, keyHandle, mockDeviceFinder)
			require.NoError(t, err)
			got, err := client.ChallengeU2F()
			if test.err != nil {
				require.Error(t, err)
				assert.Equal(t, test.err.Error(), err.Error())
				return
			}
			require.NoError(t, err)
			assert.JSONEq(t, `{"clientData":"cd","signatureData":"sig","keyHandle":""}`, got)
		})
	}
}

// failingFinder returns a DeviceFinder mock whose findDevice fails with err exactly times times.
func failingFinder(ctrl *gomock.Controller, err error, times int) *MockDeviceFinder {
	finder := NewMockDeviceFinder(ctrl)
	finder.EXPECT().findDevice().Return(nil, err).Times(times)
	return finder
}

// noDeviceFinder returns a DeviceFinder mock that reports that no device is plugged in, any number of times.
func noDeviceFinder(ctrl *gomock.Controller) *MockDeviceFinder {
	finder := NewMockDeviceFinder(ctrl)
	finder.EXPECT().findDevice().Return(nil, errNoDeviceFound).AnyTimes()
	return finder
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
		// The finder is called exactly once; the controller verifies the count when the test ends.
		finder := failingFinder(gomock.NewController(t), errNoDeviceFound, 1)
		_, err := NewU2FClient("nonce", "app", "facet", "kh", finder)
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
		client, err := NewU2FClient("nonce", "app", "facet", "kh", finder)
		assert.NoError(t, err)
		assert.Same(t, device, client.Device)
		assert.Equal(t, "facet", client.Facet)
	})
}

func TestU2FClientChallengeU2F_Behavior(t *testing.T) {
	t.Run("missing device", func(t *testing.T) {
		_, err := (&U2FClient{}).ChallengeU2F()
		assert.EqualError(t, err, "no device found")
	})

	t.Run("waits for user presence then returns the JSON response", func(t *testing.T) {
		device := mocks.NewMockU2FDevice(gomock.NewController(t))
		device.EXPECT().Close().Times(1)
		device.EXPECT().Authenticate(gomock.Any()).Return(nil, &u2fhost.TestOfUserPresenceRequiredError{}).Times(1)
		device.EXPECT().Authenticate(gomock.Any()).Return(&u2fhost.AuthenticateResponse{ClientData: "cd", SignatureData: "sig"}, nil).Times(1)

		client := &U2FClient{Device: device, ChallengeNonce: "dGVzdAo=", KeyHandle: "dGVzdAo="}
		got, err := client.ChallengeU2F()
		assert.NoError(t, err)
		assert.JSONEq(t, `{"clientData":"cd","signatureData":"sig","keyHandle":""}`, got)
	})

	t.Run("device error is returned", func(t *testing.T) {
		device := mocks.NewMockU2FDevice(gomock.NewController(t))
		device.EXPECT().Close().Times(1)
		device.EXPECT().Authenticate(gomock.Any()).Return(nil, fmt.Errorf("device unplugged")).Times(1)

		client := &U2FClient{Device: device, ChallengeNonce: "dGVzdAo=", KeyHandle: "dGVzdAo="}
		_, err := client.ChallengeU2F()
		assert.EqualError(t, err, "device unplugged")
	})

	// Atmos fork: upstream panicked on an invalid challenge; it now returns an error.
	t.Run("invalid base64 challenge returns an error", func(t *testing.T) {
		device := mocks.NewMockU2FDevice(gomock.NewController(t))
		device.EXPECT().Close().Times(1)
		client := &U2FClient{Device: device, ChallengeNonce: "not base64!", KeyHandle: "dGVzdAo="}
		_, err := client.ChallengeU2F()
		assert.ErrorContains(t, err, "invalid U2F challenge nonce")
	})
}
