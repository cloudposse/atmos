package okta

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
	oktaCloud "github.com/cloudposse/atmos/pkg/auth/cloud/okta"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
)

// slowDownInterval is the amount to increase the polling interval per RFC 8628 section 3.5.
const slowDownInterval = 5 * time.Second

// pollResult indicates the result of a token polling attempt.
type pollResult int

const (
	pollResultPending  pollResult = iota // User hasn't completed authentication yet.
	pollResultSlowDown                   // Server requests slower polling (increase interval by 5s per RFC 8628).
	pollResultDenied                     // User denied the request.
)

// pollForToken polls the token endpoint until the user completes authentication.
func (p *deviceCodeProvider) pollForToken(ctx context.Context, deviceAuth *oktaCloud.DeviceAuthorizationResponse) (*oktaCloud.OktaTokens, error) {
	defer perf.Track(nil, "okta.pollForToken")()

	interval := time.Duration(deviceAuth.Interval) * time.Second
	if interval == 0 {
		interval = defaultPollingInterval
	}

	// Calculate expiration time.
	expiresAt := time.Now().Add(time.Duration(deviceAuth.ExpiresIn) * time.Second)

	// Build token request body.
	data := url.Values{}
	data.Set("client_id", p.clientID)
	data.Set("device_code", deviceAuth.DeviceCode)
	data.Set("grant_type", "urn:ietf:params:oauth:grant-type:device_code")

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("%w: context cancelled while waiting for token", errUtils.ErrAuthenticationFailed)
		case <-ticker.C:
			// Check if device code has expired.
			if time.Now().After(expiresAt) {
				return nil, errUtils.ErrOktaDeviceCodeExpired
			}

			// Poll for token.
			tokens, result, err := p.tryGetToken(ctx, data)
			if err != nil {
				return nil, err
			}
			if tokens != nil {
				return tokens, nil
			}
			if result == pollResultSlowDown {
				// Per RFC 8628 section 3.5, increase interval by 5 seconds on slow_down.
				interval += slowDownInterval
				ticker.Reset(interval)
				log.Debug("Increased polling interval due to slow_down", "new_interval", interval)
				continue
			}
			if result == pollResultDenied {
				return nil, errUtils.ErrOktaDeviceCodeDenied
			}
			// pollResultPending: continue polling at current interval.
		}
	}
}

// tryGetToken attempts to get a token from Okta.
// Returns (tokens, pollResult, error).
func (p *deviceCodeProvider) tryGetToken(ctx context.Context, data url.Values) (*oktaCloud.OktaTokens, pollResult, error) {
	// Create request.
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.getTokenEndpoint(), strings.NewReader(data.Encode()))
	if err != nil {
		return nil, pollResultPending, fmt.Errorf("%w: failed to create token request: %w", errUtils.ErrAuthenticationFailed, err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	// Send request.
	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, pollResultPending, fmt.Errorf("%w: failed to send token request: %w", errUtils.ErrAuthenticationFailed, err)
	}
	defer resp.Body.Close()

	// Read response body.
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, pollResultPending, fmt.Errorf("%w: failed to read token response: %w", errUtils.ErrAuthenticationFailed, err)
	}

	// Handle success.
	if resp.StatusCode == http.StatusOK {
		var tokenResp oktaCloud.TokenResponse
		if err := json.Unmarshal(body, &tokenResp); err != nil {
			return nil, pollResultPending, fmt.Errorf("%w: failed to parse token response: %w", errUtils.ErrAuthenticationFailed, err)
		}
		return tokenResp.ToOktaTokens(), pollResultPending, nil
	}

	// Handle errors.
	var errorResp oktaCloud.TokenErrorResponse
	if err := json.Unmarshal(body, &errorResp); err != nil {
		return nil, pollResultPending, fmt.Errorf("%w: failed to parse error response: %w", errUtils.ErrAuthenticationFailed, err)
	}

	switch errorResp.Error {
	case "authorization_pending":
		// User hasn't completed authentication yet, continue polling.
		log.Debug("Authorization pending, continuing to poll")
		return nil, pollResultPending, nil
	case "slow_down":
		// Server requests increased polling interval per RFC 8628 section 3.5.
		log.Debug("Server requested slower polling")
		return nil, pollResultSlowDown, nil
	case "access_denied":
		// User denied the request.
		return nil, pollResultDenied, nil
	case "expired_token":
		return nil, pollResultPending, errUtils.ErrOktaDeviceCodeExpired
	default:
		return nil, pollResultPending, fmt.Errorf("%w: %s: %s", errUtils.ErrAuthenticationFailed, errorResp.Error, errorResp.ErrorDescription)
	}
}

// refreshToken exchanges a refresh token for new tokens.
func (p *deviceCodeProvider) refreshToken(ctx context.Context, refreshToken string) (*oktaCloud.OktaTokens, error) {
	defer perf.Track(nil, "okta.refreshToken")()

	// Build request body.
	data := url.Values{}
	data.Set("client_id", p.clientID)
	data.Set("grant_type", "refresh_token")
	data.Set("refresh_token", refreshToken)
	data.Set("scope", strings.Join(p.scopes, " "))

	// Create request.
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.getTokenEndpoint(), strings.NewReader(data.Encode()))
	if err != nil {
		return nil, fmt.Errorf("%w: failed to create refresh token request: %w", errUtils.ErrOktaTokenRefreshFailed, err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	// Send request.
	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: failed to send refresh token request: %w", errUtils.ErrOktaTokenRefreshFailed, err)
	}
	defer resp.Body.Close()

	// Read response body.
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("%w: failed to read refresh token response: %w", errUtils.ErrOktaTokenRefreshFailed, err)
	}

	// Check for errors.
	if resp.StatusCode != http.StatusOK {
		var errorResp oktaCloud.TokenErrorResponse
		if err := json.Unmarshal(body, &errorResp); err == nil {
			return nil, fmt.Errorf("%w: %s: %s", errUtils.ErrOktaTokenRefreshFailed, errorResp.Error, errorResp.ErrorDescription)
		}
		return nil, fmt.Errorf("%w: refresh failed with status %d: %s", errUtils.ErrOktaTokenRefreshFailed, resp.StatusCode, string(body))
	}

	// Parse response.
	var tokenResp oktaCloud.TokenResponse
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return nil, fmt.Errorf("%w: failed to parse refresh token response: %w", errUtils.ErrOktaTokenRefreshFailed, err)
	}

	log.Debug("Successfully refreshed Okta token",
		"expires_in", tokenResp.ExpiresIn,
		"has_refresh_token", tokenResp.RefreshToken != "",
	)

	return tokenResp.ToOktaTokens(), nil
}

// tryCachedTokens attempts to use cached tokens, refreshing if needed.
func (p *deviceCodeProvider) tryCachedTokens(ctx context.Context) (*oktaCloud.OktaTokens, error) {
	defer perf.Track(nil, "okta.tryCachedTokens")()

	fileManager, err := p.getFileManager()
	if err != nil {
		return nil, err
	}

	// Check if tokens exist.
	if !fileManager.TokensExist(p.name) {
		return nil, nil
	}

	// Load cached tokens.
	tokens, err := fileManager.LoadTokens(p.name)
	if err != nil {
		log.Debug("Failed to load cached tokens", "error", err)
		return nil, nil
	}

	// Check if tokens are still valid.
	if !tokens.IsExpired() {
		log.Debug("Using cached Okta tokens",
			"expires_at", tokens.ExpiresAt.Format(time.RFC3339),
		)
		return tokens, nil
	}

	// Try to refresh if we have a refresh token.
	if tokens.CanRefresh() {
		log.Debug("Access token expired, attempting refresh")
		refreshedTokens, err := p.refreshToken(ctx, tokens.RefreshToken)
		if err != nil {
			log.Debug("Failed to refresh token, will re-authenticate", "error", err)
			return nil, nil
		}

		// Save refreshed tokens.
		if err := fileManager.WriteTokens(p.name, refreshedTokens); err != nil {
			log.Debug("Failed to save refreshed tokens", "error", err)
		}

		return refreshedTokens, nil
	}

	log.Debug("Cached tokens expired and no refresh token available")
	return nil, nil
}
