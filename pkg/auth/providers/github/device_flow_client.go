package github

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
)

const (
	defaultInterval = 5   // Default polling interval in seconds.
	maxPollAttempts = 120 // Maximum polling attempts (10 minutes at 5s intervals).
)

var (
	// GitHub OAuth endpoints (variables for testing).
	deviceCodeURL  = "https://github.com/login/device/code"
	accessTokenURL = "https://github.com/login/oauth/access_token"
)

// realDeviceFlowClient implements the DeviceFlowClient interface for actual GitHub API calls.
// This implementation is inspired by ghtkn: https://github.com/suzuki-shunsuke/ghtkn
type realDeviceFlowClient struct {
	clientID      string
	scopes        []string
	keychainSvc   string
	httpClient    *http.Client
	keychainStore KeychainStore
}

// KeychainStore defines the interface for OS keychain operations.
// This allows us to mock keychain operations for testing.
type KeychainStore interface {
	// Get retrieves a token from the keychain.
	Get(service string, account string) (string, error)
	// Set stores a token in the keychain.
	Set(service string, account string, token string) error
	// Delete removes a token from the keychain.
	Delete(service string, account string) error
}

// NewDeviceFlowClient creates a new real Device Flow client.
func NewDeviceFlowClient(clientID string, scopes []string, keychainSvc string) DeviceFlowClient {
	return &realDeviceFlowClient{
		clientID:    clientID,
		scopes:      scopes,
		keychainSvc: keychainSvc,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		keychainStore: newOSKeychainStore(),
	}
}

// SetKeychainStore allows injection of a mock keychain for testing.
func (c *realDeviceFlowClient) SetKeychainStore(store KeychainStore) {
	c.keychainStore = store
}

// StartDeviceFlow initiates the Device Flow and returns device/user codes.
func (c *realDeviceFlowClient) StartDeviceFlow(ctx context.Context) (*DeviceFlowResponse, error) {
	defer perf.Track(nil, "github.realDeviceFlowClient.StartDeviceFlow")()

	// Prepare request body.
	data := url.Values{}
	data.Set("client_id", c.clientID)
	if len(c.scopes) > 0 {
		data.Set("scope", strings.Join(c.scopes, " "))
	}

	// Create request.
	req, err := http.NewRequestWithContext(ctx, "POST", deviceCodeURL, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, fmt.Errorf("%w: failed to create device code request: %v", errUtils.ErrAuthenticationFailed, err)
	}

	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	// Send request.
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: failed to request device code: %v", errUtils.ErrAuthenticationFailed, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("%w: device code request failed with status %d: %s", errUtils.ErrAuthenticationFailed, resp.StatusCode, string(body))
	}

	// Parse response.
	var result DeviceFlowResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("%w: failed to decode device code response: %v", errUtils.ErrAuthenticationFailed, err)
	}

	// Set default interval if not provided.
	if result.Interval == 0 {
		result.Interval = defaultInterval
	}

	return &result, nil
}

// PollForToken polls GitHub for the access token after user authorization.
func (c *realDeviceFlowClient) PollForToken(ctx context.Context, deviceCode string) (string, error) {
	defer perf.Track(nil, "github.realDeviceFlowClient.PollForToken")()

	ticker := time.NewTicker(time.Duration(defaultInterval) * time.Second)
	defer ticker.Stop()

	attempts := 0

	for {
		select {
		case <-ctx.Done():
			return "", fmt.Errorf("%w: context cancelled while polling for token", errUtils.ErrAuthenticationFailed)
		case <-ticker.C:
			attempts++
			if attempts > maxPollAttempts {
				return "", fmt.Errorf("%w: token polling timeout after %d attempts", errUtils.ErrAuthenticationFailed, maxPollAttempts)
			}

			token, shouldContinue, err := c.pollOnce(ctx, deviceCode)
			if err != nil {
				return "", err
			}
			if !shouldContinue {
				return token, nil
			}
		}
	}
}

// pollOnce performs a single poll attempt.
func (c *realDeviceFlowClient) pollOnce(ctx context.Context, deviceCode string) (token string, shouldContinue bool, err error) {
	data := url.Values{}
	data.Set("client_id", c.clientID)
	data.Set("device_code", deviceCode)
	data.Set("grant_type", "urn:ietf:params:oauth:grant-type:device_code")

	req, err := http.NewRequestWithContext(ctx, "POST", accessTokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		return "", false, fmt.Errorf("%w: failed to create access token request: %v", errUtils.ErrAuthenticationFailed, err)
	}

	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", false, fmt.Errorf("%w: failed to request access token: %v", errUtils.ErrAuthenticationFailed, err)
	}
	defer resp.Body.Close()

	var result struct {
		AccessToken string `json:"access_token"`
		Error       string `json:"error"`
		ErrorDesc   string `json:"error_description"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", false, fmt.Errorf("%w: failed to decode access token response: %v", errUtils.ErrAuthenticationFailed, err)
	}

	// Check for errors.
	switch result.Error {
	case "":
		// Success!
		if result.AccessToken == "" {
			return "", false, fmt.Errorf("%w: received empty access token", errUtils.ErrAuthenticationFailed)
		}
		return result.AccessToken, false, nil
	case "authorization_pending":
		// User hasn't authorized yet, continue polling.
		return "", true, nil
	case "slow_down":
		// We're polling too fast, continue but with delay.
		time.Sleep(5 * time.Second)
		return "", true, nil
	case "expired_token":
		return "", false, fmt.Errorf("%w: device code expired, please try again", errUtils.ErrAuthenticationFailed)
	case "access_denied":
		return "", false, fmt.Errorf("%w: user denied authorization", errUtils.ErrAuthenticationFailed)
	default:
		return "", false, fmt.Errorf("%w: %s: %s", errUtils.ErrAuthenticationFailed, result.Error, result.ErrorDesc)
	}
}

// GetCachedToken retrieves a cached token from the OS keychain.
func (c *realDeviceFlowClient) GetCachedToken(ctx context.Context) (string, error) {
	defer perf.Track(nil, "github.realDeviceFlowClient.GetCachedToken")()

	if c.keychainStore == nil {
		return "", fmt.Errorf("keychain store not initialized")
	}

	token, err := c.keychainStore.Get(c.keychainSvc, "github-token")
	if err != nil {
		// Token not found or keychain error - not a fatal error.
		return "", nil
	}

	return token, nil
}

// StoreToken stores a token in the OS keychain.
func (c *realDeviceFlowClient) StoreToken(ctx context.Context, token string) error {
	defer perf.Track(nil, "github.realDeviceFlowClient.StoreToken")()

	if c.keychainStore == nil {
		return fmt.Errorf("keychain store not initialized")
	}

	return c.keychainStore.Set(c.keychainSvc, "github-token", token)
}

// DeleteToken removes a token from the OS keychain.
func (c *realDeviceFlowClient) DeleteToken(ctx context.Context) error {
	defer perf.Track(nil, "github.realDeviceFlowClient.DeleteToken")()

	if c.keychainStore == nil {
		return fmt.Errorf("keychain store not initialized")
	}

	return c.keychainStore.Delete(c.keychainSvc, "github-token")
}
