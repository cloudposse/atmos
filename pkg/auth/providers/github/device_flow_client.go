package github

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
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
type realDeviceFlowClient struct {
	clientID   string
	scopes     []string
	httpClient *http.Client
}

// NewDeviceFlowClient creates a new real Device Flow client.
func NewDeviceFlowClient(clientID string, scopes []string) DeviceFlowClient {
	return &realDeviceFlowClient{
		clientID: clientID,
		scopes:   scopes,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
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

	fmt.Fprintf(os.Stderr, "DEBUG: Device Flow response - interval: %d seconds\n", result.Interval)

	// Set default interval if not provided.
	if result.Interval == 0 {
		result.Interval = defaultInterval
	}

	return &result, nil
}

// PollForToken polls GitHub for the access token after user authorization.
func (c *realDeviceFlowClient) PollForToken(ctx context.Context, deviceCode string, interval int) (string, error) {
	defer perf.Track(nil, "github.realDeviceFlowClient.PollForToken")()

	currentInterval := interval
	attempts := 0

	for {
		// Wait for the current interval.
		select {
		case <-ctx.Done():
			return "", fmt.Errorf("%w: context cancelled while polling for token", errUtils.ErrAuthenticationFailed)
		case <-time.After(time.Duration(currentInterval) * time.Second):
			attempts++
			if attempts > maxPollAttempts {
				return "", fmt.Errorf("%w: token polling timeout after %d attempts", errUtils.ErrAuthenticationFailed, maxPollAttempts)
			}

			token, shouldContinue, newInterval, err := c.pollOnce(ctx, deviceCode)
			if err != nil {
				return "", err
			}
			if newInterval > 0 {
				// slow_down received - increase interval.
				currentInterval = newInterval
				fmt.Fprintf(os.Stderr, "DEBUG: Adjusting poll interval to %d seconds\n", currentInterval)
			}
			if !shouldContinue {
				return token, nil
			}
		}
	}
}

// pollOnce performs a single poll attempt.
func (c *realDeviceFlowClient) pollOnce(ctx context.Context, deviceCode string) (token string, shouldContinue bool, newInterval int, err error) {
	data := url.Values{}
	data.Set("client_id", c.clientID)
	data.Set("device_code", deviceCode)
	data.Set("grant_type", "urn:ietf:params:oauth:grant-type:device_code")

	req, err := http.NewRequestWithContext(ctx, "POST", accessTokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		return "", false, 0, fmt.Errorf("%w: failed to create access token request: %v", errUtils.ErrAuthenticationFailed, err)
	}

	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", false, 0, fmt.Errorf("%w: failed to request access token: %v", errUtils.ErrAuthenticationFailed, err)
	}
	defer resp.Body.Close()

	// Read the full response body for debugging.
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", false, 0, fmt.Errorf("%w: failed to read response body: %v", errUtils.ErrAuthenticationFailed, err)
	}

	var result struct {
		AccessToken string `json:"access_token"`
		Error       string `json:"error"`
		ErrorDesc   string `json:"error_description"`
		Interval    int    `json:"interval"` // GitHub may return new interval with slow_down.
	}

	if err := json.Unmarshal(bodyBytes, &result); err != nil {
		return "", false, 0, fmt.Errorf("%w: failed to decode access token response: %v (body: %s)", errUtils.ErrAuthenticationFailed, err, string(bodyBytes))
	}

	// Debug logging.
	if result.Error != "" {
		fmt.Fprintf(os.Stderr, "DEBUG: Poll response - error: %s, description: %s\n", result.Error, result.ErrorDesc)
	} else if result.AccessToken != "" {
		fmt.Fprintf(os.Stderr, "DEBUG: Poll response - got access token (length: %d)\n", len(result.AccessToken))
	} else {
		fmt.Fprintf(os.Stderr, "DEBUG: Poll response - unexpected: %s\n", string(bodyBytes))
	}

	// Check for errors.
	switch result.Error {
	case "":
		// Success!
		if result.AccessToken == "" {
			return "", false, 0, fmt.Errorf("%w: received empty access token", errUtils.ErrAuthenticationFailed)
		}
		return result.AccessToken, false, 0, nil
	case "authorization_pending":
		// User hasn't authorized yet, continue polling.
		return "", true, 0, nil
	case "slow_down":
		// We're polling too fast - return new interval (add 5 seconds per RFC 8628).
		newInterval := result.Interval
		if newInterval == 0 {
			newInterval = 10 // Default to 10 seconds if not specified.
		}
		return "", true, newInterval, nil
	case "expired_token":
		return "", false, 0, fmt.Errorf("%w: device code expired, please try again", errUtils.ErrAuthenticationFailed)
	case "access_denied":
		return "", false, 0, fmt.Errorf("%w: user denied authorization", errUtils.ErrAuthenticationFailed)
	default:
		return "", false, 0, fmt.Errorf("%w: %s: %s", errUtils.ErrAuthenticationFailed, result.Error, result.ErrorDesc)
	}
}
