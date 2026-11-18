package okta

import (
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/marshallbrekka/go-u2fhost"
	"github.com/trimble-oss/go-webauthn-client"
)

const (
	MaxOpenRetries = 10
	RetryDelay     = 200 * time.Millisecond

	// systemWebAuthnTimeoutMillis is the time the system WebAuthn prompt (Windows Hello) stays open.
	// Atmos fork: upstream passed 2000, which the library reads as milliseconds, so the user had two
	// seconds to complete a PIN or biometric prompt. 60 seconds covers an interactive prompt and is
	// longer than the 25 second USB touch timeout.
	systemWebAuthnTimeoutMillis = 60000
)

// ErrU2FAuthTimeout is returned when a U2F device does not complete authentication within u2fAuthTimeout.
// Atmos fork: both timeout paths wrap this sentinel so callers can match it with errors.Is.
var ErrU2FAuthTimeout = errors.New("timed out waiting for the U2F device")

// Variables so tests can shorten them.
var (
	// u2fAuthTimeout bounds how long ChallengeU2F waits for a touch.
	u2fAuthTimeout = 25 * time.Second
	// u2fPollInterval is the delay between Authenticate attempts.
	u2fPollInterval = 250 * time.Millisecond
)

var errNoDeviceFound = fmt.Errorf("no U2F devices found. device might not be plugged in")

// errU2FNoDevice is returned when a client has no device to challenge.
var errU2FNoDevice = errors.New("no device found")

// FidoClient represents a challenge and the device used to respond
type FidoClient struct {
	ChallengeNonce string
	AppID          string
	Version        string
	Device         u2fhost.Device
	KeyHandle      string
	StateToken     string
}

// SignedAssertion is passed back to Okta as response
type SignedAssertion struct {
	StateToken        string `json:"stateToken"`
	ClientData        string `json:"clientData"`
	SignatureData     string `json:"signatureData"`
	AuthenticatorData string `json:"authenticatorData"`
}

// DeviceFinder is used to mock out finding devices
type DeviceFinder interface {
	findDevice() (u2fhost.Device, error)
}

// U2FDevice is used to support mocking this device with mockery https://github.com/vektra/mockery/issues/210#issuecomment-485026348
type U2FDevice interface {
	u2fhost.Device
}

// NewFidoClient returns a new initialized FIDO1-based WebAuthnClient, representing a single device
func NewFidoClient(challengeNonce, appID, version, keyHandle, stateToken string, deviceFinder DeviceFinder) (FidoClient, error) {
	var device u2fhost.Device
	var err error

	retryCount := 0
	for retryCount < MaxOpenRetries {
		device, err = deviceFinder.findDevice()
		if err != nil {
			if errors.Is(err, errNoDeviceFound) {
				return FidoClient{}, err
			}

			retryCount++
			time.Sleep(RetryDelay)
			continue
		}

		return FidoClient{
			Device:         device,
			ChallengeNonce: challengeNonce,
			AppID:          appID,
			Version:        version,
			KeyHandle:      keyHandle,
			StateToken:     stateToken,
		}, nil
	}

	return FidoClient{}, fmt.Errorf("failed to create client: %w. exceeded max retries of %d", err, MaxOpenRetries)
}

// u2fAuthResult carries the outcome of one blocking Device.Authenticate call.
type u2fAuthResult struct {
	response *u2fhost.AuthenticateResponse
	err      error
}

// authenticateAsync runs one blocking Device.Authenticate call in its own goroutine.
// Atmos fork: go-u2fhost reads from the HID device without a deadline, so a device that stops answering
// blocked ChallengeU2F forever and the timeout below could never fire. The result channel is buffered
// and never closed, so the goroutine can always deliver its result and exit, even after the caller stopped waiting.
func (d *FidoClient) authenticateAsync(request *u2fhost.AuthenticateRequest) <-chan u2fAuthResult {
	return authenticateDeviceAsync(d.Device, request)
}

// authenticateDeviceAsync runs one blocking Authenticate call on the device in its own goroutine.
// Shared by the Okta FIDO and Duo U2F clients.
func authenticateDeviceAsync(device u2fhost.Device, request *u2fhost.AuthenticateRequest) <-chan u2fAuthResult {
	resultCh := make(chan u2fAuthResult, 1)
	go func() {
		response, err := device.Authenticate(request)
		resultCh <- u2fAuthResult{response: response, err: err}
	}()
	return resultCh
}

// u2fTimeoutError builds the error returned when a U2F device does not answer in time.
func u2fTimeoutError() error {
	return fmt.Errorf("%w: failed to get authentication response after %d seconds", ErrU2FAuthTimeout, int(u2fAuthTimeout/time.Second))
}

// ChallengeU2F takes a FidoClient and returns a signed assertion to send to Okta
func (d *FidoClient) ChallengeU2F() (*SignedAssertion, error) {
	if d.Device == nil {
		return nil, errU2FNoDevice
	}
	request := &u2fhost.AuthenticateRequest{
		Challenge: d.ChallengeNonce,
		Facet:     "https://" + d.AppID,
		AppId:     d.AppID,
		KeyHandle: d.KeyHandle,
		WebAuthn:  true,
	}
	// do the change
	prompted := false
	// Atmos fork: the absolute deadline is also checked before another read is dispatched, because
	// select picks randomly when the timeout and the ticker are both ready.
	deadline := time.Now().Add(u2fAuthTimeout)
	timeout := time.After(u2fAuthTimeout)
	interval := time.NewTicker(u2fPollInterval)
	var responsePayload *SignedAssertion

	// The device is closed on return, except when a blocked Authenticate call is abandoned on timeout:
	// closing a HID handle while another goroutine is reading from it is unsafe, so the abandoned call
	// closes the device itself once its read returns.
	closeDevice := true
	defer func() {
		if closeDevice {
			d.Device.Close()
		}
	}()
	defer interval.Stop()
	// Atmos fork: each attempt is dispatched before waiting, so the first device read always starts
	// before the timeout can win (upstream waited one poll interval first, racing the timeout).
	for {
		pending := d.authenticateAsync(request)
		var result u2fAuthResult
		select {
		case result = <-pending:
		case <-timeout:
			closeDevice = false
			go func() {
				<-pending
				d.Device.Close()
			}()
			return nil, u2fTimeoutError()
		}
		response, err := result.response, result.err
		if err == nil {
			responsePayload = &SignedAssertion{
				StateToken:        d.StateToken,
				ClientData:        response.ClientData,
				SignatureData:     response.SignatureData,
				AuthenticatorData: response.AuthenticatorData,
			}
			log.Println("  ==> Touch accepted. Proceeding with authentication")
			return responsePayload, nil
		}

		var presenceErr *u2fhost.TestOfUserPresenceRequiredError
		switch {
		case errors.As(err, &presenceErr):
			if !prompted {
				log.Println("Touch the flashing U2F device to authenticate...")
				prompted = true
			}
		default:
			errString := fmt.Sprintf("%s", err)
			if !strings.Contains(errString, "U2FHIDError") &&
				!strings.Contains(errString, "hidapi: hid_error is not implemented yet") {
				return responsePayload, err
			}
		}

		select {
		case <-timeout:
			return nil, u2fTimeoutError()
		case <-interval.C:
			if !time.Now().Before(deadline) {
				return nil, u2fTimeoutError()
			}
		}
	}
}

// U2FDeviceFinder returns a U2F device
type U2FDeviceFinder struct{}

func (*U2FDeviceFinder) findDevice() (u2fhost.Device, error) {
	var err error

	allDevices := u2fhost.Devices()
	if len(allDevices) == 0 {
		return nil, errNoDeviceFound
	}

	for i, device := range allDevices {
		err = device.Open()
		if err != nil {
			device.Close()

			continue
		}

		return allDevices[i], nil
	}

	return nil, fmt.Errorf("failed to open fido U2F device: %w", err)
}

// ChallengeSystemWebAuthn challenges the system level WebAuthn API (e.g. Windows Hello) and
// returns a SignedAssertion
func ChallengeSystemWebAuthn(challengeNonce, appID, stateToken string) (*SignedAssertion, error) {
	var responsePayload *SignedAssertion

	response, err := webauthn.Authenticate(challengeNonce, appID, systemWebAuthnTimeoutMillis)
	if err != nil {
		return nil, err
	}

	responsePayload = &SignedAssertion{
		StateToken:        stateToken,
		ClientData:        response.ClientData,
		SignatureData:     response.SignatureData,
		AuthenticatorData: response.AuthenticatorData,
	}

	return responsePayload, nil
}
