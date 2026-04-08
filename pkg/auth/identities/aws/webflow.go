package aws

// Browser-based OAuth2 PKCE authentication flow for aws/user identities.
// This file holds the top-level Authenticate entry points, package-level
// types and override variables. Implementation details are split across:
//
//   webflow_browser.go  — interactive/non-interactive dispatch and wait loops
//   webflow_oauth.go    — PKCE/state helpers and local callback HTTP server
//   webflow_token.go    — token endpoint HTTP + response parsing
//   webflow_cache.go    — refresh-token XDG cache file I/O
//   webflow_ui.go       — TTY detection, display dialogs, bubbletea spinner model

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/browser"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
)

const (
	webflowOAuthClientID       = "arn:aws:signin:::devtools/same-device"
	webflowCallbackPath        = "/oauth/callback"
	webflowCodeVerifierBytes   = 32
	webflowScope               = "openid"
	webflowCodeChallengeMethod = "SHA-256"
	webflowResponseType        = "code"
	webflowGrantTypeAuthCode   = "authorization_code"
	webflowGrantTypeRefresh    = "refresh_token"
	webflowCallbackTimeout     = 5 * time.Minute
	webflowTokenMaxBodyBytes   = 1 << 20 // 1 MB max response body.
	webflowCacheSubdir         = "aws-webflow"
	webflowCacheFilename       = "refresh.json"
	webflowCacheDirPerms       = 0o700
	webflowCacheFilePerms      = 0o600
	webflowSessionDuration     = 12 * time.Hour
	// Buffer before expiration at which webflowTokenRefreshBuffer triggers a refresh.
	webflowTokenRefreshBuffer = 1 * time.Minute
	// Byte length for generated state strings (CSRF protection).
	webflowStateBytes = 16
	// Read-header timeout for the local callback HTTP server.
	webflowCallbackReadHeaderTimeout = 10 * time.Second
	// Shutdown timeout for the local callback HTTP server.
	webflowCallbackShutdownTimeout = 5 * time.Second
)

// wrapWebflowErr wraps a cause error with a sentinel so both remain in the
// error chain. Centralizes the common `fmt.Errorf("%w: %w", sentinel, cause)`
// pattern used throughout webflow error handling.
func wrapWebflowErr(sentinel, cause error) error {
	return fmt.Errorf("%w: %w", sentinel, cause)
}

// HTTPClient abstracts HTTP requests for token exchange (testability).
type HTTPClient interface {
	Do(req *http.Request) (*http.Response, error)
}

// webflowResult holds the authorization code from the OAuth2 callback.
type webflowResult struct {
	code  string
	state string
	err   error
}

// webflowTokenResponse matches the AWS signin /v1/token JSON response.
type webflowTokenResponse struct {
	AccessToken  webflowAccessToken `json:"accessToken"`
	ExpiresIn    int                `json:"expiresIn"`
	RefreshToken string             `json:"refreshToken"` //nolint:gosec // G117: OAuth2 refresh token field name, not a hardcoded secret.
	TokenType    string             `json:"tokenType"`
	IDToken      string             `json:"idToken"`
}

// webflowAccessToken holds the nested AWS credential fields.
type webflowAccessToken struct {
	AccessKeyID     string `json:"accessKeyId"`
	SecretAccessKey string `json:"secretAccessKey"`
	SessionToken    string `json:"sessionToken"` //nolint:gosec // G117: AWS STS session token field name, not a hardcoded secret.
}

// webflowRefreshCache stores the refresh token for later use.
type webflowRefreshCache struct {
	RefreshToken string    `json:"refreshToken"` //nolint:gosec // G117: OAuth2 refresh token field name, not a hardcoded secret.
	Region       string    `json:"region"`
	ExpiresAt    time.Time `json:"expiresAt"` // Session end time (~12h from initial auth).
}

// webflowTokenErrorResponse represents an error response from the token endpoint.
type webflowTokenErrorResponse struct {
	Error            string `json:"error"`
	ErrorDescription string `json:"error_description"`
}

// defaultHTTPClient is the default HTTP client for token exchange.
var defaultHTTPClient HTTPClient = &http.Client{Timeout: 30 * time.Second}

// openURLFunc opens a URL in the default browser. Overridable for testing.
var openURLFunc = func(url string) error {
	return browser.New().Open(url)
}

// webflowIsTTYFunc checks if the terminal is a TTY. Overridable for testing.
var webflowIsTTYFunc = webflowIsTTY

// displayWebflowDialogFunc shows the authentication URL. Overridable for testing.
var displayWebflowDialogFunc = displayWebflowDialog

// displayWebflowPlainTextFunc shows the authentication URL in plain-text mode. Overridable for testing.
var displayWebflowPlainTextFunc = displayWebflowDialogPlainText

// webflowStdinReader is the io.Reader used by browserWebflowNonInteractive to
// read the manual authorization code. Defaults to os.Stdin but is overridable
// for testing so unit tests can supply a controlled reader without mutating
// the global os.Stdin (which causes data races with the reader goroutine).
var webflowStdinReader io.Reader = os.Stdin

// resolveCredentialsViaWebflow attempts to obtain AWS credentials via the OAuth2 browser flow.
// It first tries to use a cached refresh token, then falls back to the full browser flow.
func (i *userIdentity) resolveCredentialsViaWebflow(ctx context.Context) (*types.AWSCredentials, error) {
	defer perf.Track(nil, "aws.userIdentity.resolveCredentialsViaWebflow")()

	if !i.isWebflowEnabled() {
		return nil, errUtils.ErrWebflowDisabled
	}

	region := i.resolveRegion()

	// Try refresh token first (avoids opening browser).
	creds, err := i.refreshWebflowCredentials(ctx, region)
	if err == nil {
		return creds, nil
	}
	log.Debug("Refresh token not available or expired, starting browser flow", logKeyIdentity, i.name, "error", err)

	// Fall back to full browser flow.
	return i.browserWebflow(ctx, region)
}

// refreshWebflowCredentials attempts to get new credentials using a cached refresh token.
func (i *userIdentity) refreshWebflowCredentials(ctx context.Context, region string) (*types.AWSCredentials, error) {
	defer perf.Track(nil, "aws.userIdentity.refreshWebflowCredentials")()

	cache, err := i.loadRefreshCache()
	if err != nil {
		return nil, wrapWebflowErr(errUtils.ErrWebflowRefreshFailed, err)
	}

	// Check if session has expired.
	if time.Now().Add(webflowTokenRefreshBuffer).After(cache.ExpiresAt) {
		return nil, fmt.Errorf("%w: refresh token session expired", errUtils.ErrWebflowRefreshFailed)
	}

	// Exchange refresh token for new credentials.
	tokenResp, err := exchangeRefreshToken(ctx, defaultHTTPClient, region, cache.RefreshToken)
	if err != nil {
		// Only delete the cached refresh token when the server definitively rejects it
		// (HTTP 400 invalid_grant/invalid_token per RFC 6749 §5.2). Transient failures —
		// HTTP 5xx, 429, network errors, context cancellation, malformed responses —
		// must preserve the cache so unattended runs can retry without a browser prompt.
		if errors.Is(err, errUtils.ErrWebflowRefreshTokenRevoked) {
			i.deleteRefreshCache()
		}
		return nil, wrapWebflowErr(errUtils.ErrWebflowRefreshFailed, err)
	}

	creds := tokenResponseToCredentials(tokenResp, region)

	// Update refresh cache with new refresh token if provided.
	if tokenResp.RefreshToken != "" {
		i.saveRefreshCache(&webflowRefreshCache{
			RefreshToken: tokenResp.RefreshToken,
			Region:       region,
			ExpiresAt:    cache.ExpiresAt, // Session end time doesn't change.
		})
	}

	log.Debug("Refreshed webflow credentials successfully", logKeyIdentity, i.name)
	return creds, nil
}

// isWebflowEnabled checks if browser authentication is enabled for this identity.
// Returns true by default unless explicitly disabled via credentials.webflow_enabled: false.
func (i *userIdentity) isWebflowEnabled() bool {
	if i.config.Credentials == nil {
		return true
	}
	enabled, ok := i.config.Credentials["webflow_enabled"].(bool)
	if !ok {
		return true // Default: enabled.
	}
	return enabled
}
