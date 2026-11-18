package googleapps

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	u2fhost "github.com/marshallbrekka/go-u2fhost"
	"github.com/pkg/errors"
)

const maxOpenRetries = 10

// Atmos fork: the retry delay and the touch timeout are package variables so tests can shorten them.
// The production values are unchanged.
var (
	retryDelay      = 200 * time.Millisecond
	u2fTouchTimeout = 25 * time.Second

	// u2fOpenTimeout bounds how long opening a device may block on the HID initialization exchange.
	u2fOpenTimeout = 10 * time.Second

	// u2fPollInterval is how often the device is asked for a signature while waiting for a touch.
	u2fPollInterval = 250 * time.Millisecond // gitleaks:allow -- Poll interval, not a secret.
)

// u2fTouchDeadline starts the touch timeout. It returns a channel that fires when the timeout elapses and a
// function that releases the timer.
// Atmos fork: a package variable so tests can fire the timeout at a chosen moment. The default is a real timer.
var u2fTouchDeadline = func() (<-chan time.Time, func()) {
	timer := time.NewTimer(u2fTouchTimeout)
	return timer.C, func() { timer.Stop() }
}

// Atmos fork: HID enumeration goes through a package variable so tests can supply fake devices.
// The default is the real enumeration.
var listU2FDevices = func() []u2fhost.Device {
	hidDevices := u2fhost.Devices()
	devices := make([]u2fhost.Device, 0, len(hidDevices))
	for _, device := range hidDevices {
		devices = append(devices, device)
	}
	return devices
}

var errNoDeviceFound = fmt.Errorf("no U2F devices found. device might not be plugged in")

// errU2FOpenTimeout is returned when a device does not finish opening in time.
var errU2FOpenTimeout = errors.New("timed out opening the U2F device")

// errU2FTouchTimeout is returned when the user does not touch the device in time.
var errU2FTouchTimeout = errors.New("failed to get authentication response after 25 seconds")

// U2FClient represents a challenge and the device used to respond
type U2FClient struct {
	ChallengeNonce string
	AppID          string
	Facet          string
	Device         u2fhost.Device
	KeyHandle      string
}

// DeviceFinder is used to mock out finding devices
type DeviceFinder interface {
	findDevice() (u2fhost.Device, error)
}

// U2FDevice is used to support mocking this device with mockery https://github.com/vektra/mockery/issues/210#issuecomment-485026348
type U2FDevice interface {
	u2fhost.Device
}

// NewU2FClient returns a new initialized FIDO1-based U2F client, representing a single device
func NewU2FClient(challengeNonce, appID, facet, keyHandle string, deviceFinder DeviceFinder) (*U2FClient, error) {
	var device u2fhost.Device
	var err error

	retryCount := 0
	for retryCount < maxOpenRetries {
		device, err = deviceFinder.findDevice()
		if err != nil {
			if errors.Is(err, errNoDeviceFound) || errors.Is(err, errU2FOpenTimeout) {
				return nil, err
			}

			retryCount++
			time.Sleep(retryDelay)
			continue
		}

		return &U2FClient{
			Device:         device,
			ChallengeNonce: challengeNonce,
			AppID:          appID,
			KeyHandle:      keyHandle,
			Facet:          facet,
		}, nil
	}

	return nil, fmt.Errorf("failed to create client: %w. exceeded max retries of %d", err, maxOpenRetries)
}

// ChallengeU2F takes a U2FClient and returns a signed assertion to send to Google.
//
// Atmos fork: the device is owned by one worker goroutine that always closes it when its last read
// ends. The touch timeout is enforced by the caller, so a key that stops answering HID reads cannot block
// the caller (which can then offer another factor). The handle is never closed concurrently with a read.
func (d *U2FClient) ChallengeU2F() (string, error) {
	if d.Device == nil {
		return "", errors.New("no device found")
	}

	challenge, err := b64Safe(d.ChallengeNonce)
	if err != nil {
		d.Device.Close()
		return "", errors.Wrap(err, "invalid U2F challenge nonce")
	}
	keyHandle, err := b64Safe(d.KeyHandle)
	if err != nil {
		d.Device.Close()
		return "", errors.Wrap(err, "invalid U2F key handle")
	}
	request := &u2fhost.AuthenticateRequest{
		Challenge: challenge,
		Facet:     d.Facet,
		AppId:     d.AppID,
		KeyHandle: keyHandle,
	}

	type outcome struct {
		response string
		err      error
	}
	stop := make(chan struct{})
	done := make(chan outcome, 1)
	go func() {
		response, err := d.authenticateUntilStopped(request, stop)
		// The device is closed before the result is published, and only after its last read returned.
		d.Device.Close()
		done <- outcome{response: response, err: err}
	}()

	deadline, release := u2fTouchDeadline()
	defer release()
	select {
	case result := <-done:
		return result.response, result.err
	case <-deadline:
		// Tell the worker to stop; it closes the device when its current read returns.
		close(stop)
		return "", errU2FTouchTimeout
	}
}

// authenticateUntilStopped polls the device for a signed response until it succeeds, fails or stop is closed.
func (d *U2FClient) authenticateUntilStopped(request *u2fhost.AuthenticateRequest, stop <-chan struct{}) (string, error) {
	prompted := false
	interval := time.NewTicker(u2fPollInterval)
	defer interval.Stop()
	for {
		select {
		case <-stop:
			return "", errU2FTouchTimeout
		case <-interval.C:
			// Atmos fork: select picks randomly when stop and the ticker are both ready, so stop is checked
			// again before starting a read that could block and keep the device open after the timeout.
			select {
			case <-stop:
				return "", errU2FTouchTimeout
			default:
			}
			response, err := d.Device.Authenticate(request)
			if err == nil {
				responseJSON, err := json.Marshal(response)
				if err != nil {
					return "", err
				}
				log.Println("  ==> Touch accepted. Proceeding with authentication")
				return string(responseJSON), nil
			}

			var presenceErr *u2fhost.TestOfUserPresenceRequiredError
			switch {
			case errors.As(err, &presenceErr):
				if !prompted {
					log.Println("Touch the flashing U2F device to authenticate...")
					prompted = true
				}
			default:
				return "", err
			}
		}
	}
}

// U2FDeviceFinder returns a U2F device
type U2FDeviceFinder struct{}

func (*U2FDeviceFinder) findDevice() (u2fhost.Device, error) {
	var err error
	timedOut := false
	allDevices := listU2FDevices()
	if len(allDevices) == 0 {
		return nil, errNoDeviceFound
	}

	for i, device := range allDevices {
		err = openDevice(device, u2fOpenTimeout)
		if err != nil {
			timedOut = timedOut || errors.Is(err, errU2FOpenTimeout)
			continue
		}

		return allDevices[i], nil
	}

	// Atmos fork: when any device timed out, the error keeps errU2FOpenTimeout even if a later device failed
	// differently, so NewU2FClient does not retry discovery and leave more blocked Open calls behind.
	if timedOut && !errors.Is(err, errU2FOpenTimeout) {
		return nil, fmt.Errorf("failed to open fido U2F device: %w; %w", errU2FOpenTimeout, err)
	}
	return nil, fmt.Errorf("failed to open fido U2F device: %w", err)
}

// b64Safe re-encodes standard base64 data as unpadded URL-safe base64.
// Atmos fork: upstream panicked on invalid input; it now returns the decode error.
func b64Safe(data string) (string, error) {
	val, err := base64.StdEncoding.DecodeString(data)
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(val), nil
}

// openDevice opens the device, giving up after timeout.
//
// Atmos fork: a key can open at the OS level but never answer the HID initialization request, which
// blocked Open forever. The Open call runs in a goroutine that owns the handle: it closes the device when Open
// fails, and also when Open succeeds after the caller gave up. The caller never closes the device itself.
func openDevice(device u2fhost.Device, timeout time.Duration) error {
	var (
		mu        sync.Mutex
		abandoned bool
	)
	result := make(chan error, 1)

	go func() {
		err := device.Open()
		if err != nil {
			device.Close()
		}
		mu.Lock()
		defer mu.Unlock()
		if abandoned {
			if err == nil {
				device.Close()
			}
			return
		}
		result <- err
	}()

	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case err := <-result:
		return err
	case <-timer.C:
		mu.Lock()
		defer mu.Unlock()
		// The worker may have finished between the timer firing and the lock being taken.
		select {
		case err := <-result:
			return err
		default:
		}
		abandoned = true
		return errU2FOpenTimeout
	}
}
