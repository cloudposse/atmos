package okta

import (
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/marshallbrekka/go-u2fhost"
)

// DUOU2fClient represents a challenge and the device used to respond
type DUOU2FClient struct {
	ChallengeNonce string
	AppID          string
	Version        string
	Device         u2fhost.Device
	KeyHandle      string
	StateToken     string
}

// ResponseData is passed back to DUO as a response
type ResponseData struct {
	SessionId     string `json:"sessionId"`
	ClientData    string `json:"clientData"`
	SignatureData string `json:"signatureData"`
	KeyHandle     string `json:"keyHandle"`
}

// NewDUOU2FClient returns a new initialized DUOU2F-based WebAuthnClient, representing a single device
func NewDUOU2FClient(challengeNonce, appID, version, keyHandle, stateToken string, deviceFinder DeviceFinder) (*DUOU2FClient, error) {
	var device u2fhost.Device
	var err error

	retryCount := 0
	for retryCount < MaxOpenRetries {
		device, err = deviceFinder.findDevice()
		if err != nil {
			if errors.Is(err, errNoDeviceFound) {
				return nil, err
			}

			retryCount++
			time.Sleep(RetryDelay)
			continue
		}

		return &DUOU2FClient{
			Device:         device,
			ChallengeNonce: challengeNonce,
			AppID:          appID,
			Version:        version,
			KeyHandle:      keyHandle,
			StateToken:     stateToken,
		}, nil
	}

	return nil, fmt.Errorf("failed to create client: %w. exceeded max retries of %d", err, MaxOpenRetries)
}

// ChallengeU2F takes a FidoClient and returns a signed assertion to send to Okta
func (d *DUOU2FClient) ChallengeU2F() (*ResponseData, error) {
	if d.Device == nil {
		return nil, errU2FNoDevice
	}
	request := &u2fhost.AuthenticateRequest{
		Challenge: d.ChallengeNonce,
		Facet:     d.AppID,
		AppId:     d.AppID,
		KeyHandle: d.KeyHandle,
		WebAuthn:  false,
	}
	// do the change
	prompted := false
	// Atmos fork: the wait is bounded by u2fAuthTimeout (25 seconds, as before) even while a device read
	// is blocked, and no new read starts after the deadline. See FidoClient.ChallengeU2F.
	deadline := time.Now().Add(u2fAuthTimeout)
	timeout := time.After(u2fAuthTimeout)
	interval := time.NewTicker(u2fPollInterval)

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
	for {
		pending := authenticateDeviceAsync(d.Device, request)
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
			// Atmos fork: upstream printed the touch prompts to stdout, the caller's data
			// channel; they are written to stderr with the other interactive prompts.
			fmt.Fprintf(os.Stderr, "  ==> Touch accepted. Proceeding with authentication\n")
			return &ResponseData{
				SessionId:     d.StateToken,
				ClientData:    response.ClientData,
				SignatureData: response.SignatureData,
				KeyHandle:     d.KeyHandle,
			}, nil
		}

		var presenceErr *u2fhost.TestOfUserPresenceRequiredError
		if !errors.As(err, &presenceErr) {
			return nil, err
		}
		if !prompted {
			fmt.Fprintf(os.Stderr, "\nTouch the flashing U2F device to authenticate...\n")
			prompted = true
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
