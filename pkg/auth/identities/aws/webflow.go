package aws

import (
	"bufio"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-isatty"
	"github.com/spf13/viper"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/browser"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/telemetry"
	"github.com/cloudposse/atmos/pkg/ui/theme"
	"github.com/cloudposse/atmos/pkg/utils"
	"github.com/cloudposse/atmos/pkg/xdg"
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
	// webflowTokenRefreshBuffer is the buffer before expiration to trigger refresh.
	webflowTokenRefreshBuffer = 1 * time.Minute
)

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
	RefreshToken string             `json:"refreshToken"`
	TokenType    string             `json:"tokenType"`
	IDToken      string             `json:"idToken"`
}

// webflowAccessToken holds the nested AWS credential fields.
type webflowAccessToken struct {
	AccessKeyID     string `json:"accessKeyId"`
	SecretAccessKey string `json:"secretAccessKey"`
	SessionToken    string `json:"sessionToken"`
}

// webflowRefreshCache stores the refresh token for later use.
type webflowRefreshCache struct {
	RefreshToken string    `json:"refreshToken"`
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

// resolveCredentialsViaWebflow attempts to obtain AWS credentials via the OAuth2 browser flow.
// It first tries to refresh using a cached refresh token, then falls back to a full browser flow.
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
		return nil, fmt.Errorf("%w: %w", errUtils.ErrWebflowRefreshFailed, err)
	}

	// Check if session has expired.
	if time.Now().Add(webflowTokenRefreshBuffer).After(cache.ExpiresAt) {
		return nil, fmt.Errorf("%w: refresh token session expired", errUtils.ErrWebflowRefreshFailed)
	}

	// Exchange refresh token for new credentials.
	tokenResp, err := exchangeRefreshToken(ctx, defaultHTTPClient, region, cache.RefreshToken)
	if err != nil {
		// Only clear cache for definitive server rejections (invalid_grant, etc.),
		// not transient errors (network issues, timeouts) that don't invalidate the token.
		if !isTransientTokenError(err) {
			i.deleteRefreshCache()
		}
		return nil, fmt.Errorf("%w: %w", errUtils.ErrWebflowRefreshFailed, err)
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

// browserWebflow performs the full OAuth2 PKCE browser authentication flow.
func (i *userIdentity) browserWebflow(ctx context.Context, region string) (*types.AWSCredentials, error) {
	defer perf.Track(nil, "aws.userIdentity.browserWebflow")()

	allowPrompts := types.AllowPrompts(ctx)

	if allowPrompts && webflowIsTTYFunc() {
		return i.browserWebflowInteractive(ctx, region)
	}

	// Non-interactive mode: display URL for manual auth.
	if allowPrompts {
		return i.browserWebflowNonInteractive(ctx, region)
	}

	return nil, errUtils.Build(errUtils.ErrWebflowAuthFailed).
		WithExplanation("Browser authentication requires an interactive terminal or prompts").
		WithHint("Use 'atmos auth login' in an interactive terminal").
		WithHint("Or configure static credentials in atmos.yaml or keychain").
		WithContext("identity", i.name).
		WithExitCode(2).
		Err()
}

// browserWebflowInteractive performs the browser flow with automatic browser opening and callback server.
func (i *userIdentity) browserWebflowInteractive(ctx context.Context, region string) (*types.AWSCredentials, error) {
	// Generate PKCE pair.
	verifier, challenge, err := generatePKCEPair()
	if err != nil {
		return nil, fmt.Errorf("%w: failed to generate PKCE pair: %w", errUtils.ErrWebflowAuthFailed, err)
	}

	// Generate state for CSRF protection.
	state, err := generateRandomString(16)
	if err != nil {
		return nil, fmt.Errorf("%w: failed to generate state: %w", errUtils.ErrWebflowAuthFailed, err)
	}

	// Start callback server.
	listener, resultCh, err := startCallbackServer(ctx, state)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", errUtils.ErrWebflowCallbackServer, err)
	}
	defer listener.Close()

	// Build authorization URL.
	port := listener.Addr().(*net.TCPAddr).Port
	authURL := buildAuthorizationURL(region, port, challenge, state)
	redirectURI := fmt.Sprintf("http://127.0.0.1:%d%s", port, webflowCallbackPath)

	// Display UI and open browser.
	displayWebflowDialogFunc(authURL)

	if !telemetry.IsCI() {
		if err := openURLFunc(authURL); err != nil {
			log.Debug("Failed to open browser automatically", "error", err)
		}
	}

	// Wait for callback with spinner.
	tokenResp, err := i.waitForCallbackWithSpinner(ctx, resultCh, region, verifier, redirectURI)
	if err != nil {
		return nil, err
	}

	return i.processTokenResponse(tokenResp, region)
}

// browserWebflowNonInteractive performs the browser flow with manual code entry.
func (i *userIdentity) browserWebflowNonInteractive(ctx context.Context, region string) (*types.AWSCredentials, error) {
	// Generate PKCE pair.
	verifier, challenge, err := generatePKCEPair()
	if err != nil {
		return nil, fmt.Errorf("%w: failed to generate PKCE pair: %w", errUtils.ErrWebflowAuthFailed, err)
	}

	// Generate state for CSRF protection.
	state, err := generateRandomString(16)
	if err != nil {
		return nil, fmt.Errorf("%w: failed to generate state: %w", errUtils.ErrWebflowAuthFailed, err)
	}

	// Start callback server (user might be on the same machine and browser redirects).
	listener, resultCh, err := startCallbackServer(ctx, state)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", errUtils.ErrWebflowCallbackServer, err)
	}
	defer listener.Close()

	port := listener.Addr().(*net.TCPAddr).Port
	authURL := buildAuthorizationURL(region, port, challenge, state)
	redirectURI := fmt.Sprintf("http://127.0.0.1:%d%s", port, webflowCallbackPath)

	// Display URL for manual use.
	displayWebflowDialogPlainText(authURL)

	// Read authorization code from stdin (non-interactive: no TUI, just line input).
	codeCh := make(chan string, 1)
	errCh := make(chan error, 1)

	go func() {
		fmt.Fprintf(os.Stderr, "Authorization code: ")
		scanner := bufio.NewScanner(os.Stdin)
		if scanner.Scan() {
			code := strings.TrimSpace(scanner.Text())
			if code == "" {
				errCh <- fmt.Errorf("authorization code is required")
				return
			}
			codeCh <- code
		} else if err := scanner.Err(); err != nil {
			errCh <- fmt.Errorf("failed to read authorization code: %w", err)
		} else {
			errCh <- fmt.Errorf("no input received (stdin closed)")
		}
	}()

	// Race between manual code entry and callback.
	select {
	case result := <-resultCh:
		if result.err != nil {
			return nil, fmt.Errorf("%w: callback error: %w", errUtils.ErrWebflowAuthFailed, result.err)
		}
		tokenResp, tokenErr := exchangeCodeForCredentials(ctx, defaultHTTPClient, region, result.code, verifier, redirectURI)
		if tokenErr != nil {
			return nil, tokenErr
		}
		return i.processTokenResponse(tokenResp, region)
	case code := <-codeCh:
		tokenResp, tokenErr := exchangeCodeForCredentials(ctx, defaultHTTPClient, region, code, verifier, redirectURI)
		if tokenErr != nil {
			return nil, tokenErr
		}
		return i.processTokenResponse(tokenResp, region)
	case readErr := <-errCh:
		return nil, fmt.Errorf("%w: %w", errUtils.ErrWebflowAuthFailed, readErr)
	case <-ctx.Done():
		return nil, fmt.Errorf("%w: %w", errUtils.ErrWebflowTimeout, ctx.Err())
	}
}

// waitForCallbackWithSpinner waits for the OAuth2 callback with a spinner UI.
func (i *userIdentity) waitForCallbackWithSpinner(ctx context.Context, resultCh <-chan webflowResult, region, verifier, redirectURI string) (*webflowTokenResponse, error) {
	if !webflowIsTTYFunc() || telemetry.IsCI() {
		return i.waitForCallbackSimple(ctx, resultCh, region, verifier, redirectURI)
	}

	ctx, cancel := context.WithTimeout(ctx, webflowCallbackTimeout)
	defer cancel()

	tokenCh := make(chan webflowSpinnerTokenResult, 1)

	go func() {
		defer close(tokenCh)
		select {
		case result := <-resultCh:
			if result.err != nil {
				tokenCh <- webflowSpinnerTokenResult{err: fmt.Errorf("%w: %w", errUtils.ErrWebflowAuthFailed, result.err)}
				return
			}
			resp, err := exchangeCodeForCredentials(ctx, defaultHTTPClient, region, result.code, verifier, redirectURI)
			tokenCh <- webflowSpinnerTokenResult{resp: resp, err: err}
		case <-ctx.Done():
			tokenCh <- webflowSpinnerTokenResult{err: fmt.Errorf("%w: %w", errUtils.ErrWebflowTimeout, ctx.Err())}
		}
	}()

	// Run spinner.
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = theme.GetCurrentStyles().Spinner

	model := webflowSpinnerModel{
		spinner: s,
		message: "Waiting for browser authentication",
		tokenCh: tokenCh,
		cancel:  cancel,
	}

	prog := tea.NewProgram(model, tea.WithOutput(os.Stderr))
	finalModel, err := prog.Run()
	if err != nil {
		cancel()
		res := <-tokenCh
		return res.resp, res.err
	}

	final := finalModel.(webflowSpinnerModel)
	if final.result == nil {
		return nil, errUtils.Build(errUtils.ErrWebflowAuthFailed).
			WithExplanation("Browser authentication did not complete").
			WithHint("Try running the authentication again").
			WithContext("identity", i.name).
			WithExitCode(1).
			Err()
	}
	return final.result.resp, final.result.err
}

// waitForCallbackSimple waits for callback without spinner (non-TTY environments).
func (i *userIdentity) waitForCallbackSimple(ctx context.Context, resultCh <-chan webflowResult, region, verifier, redirectURI string) (*webflowTokenResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, webflowCallbackTimeout)
	defer cancel()

	select {
	case result := <-resultCh:
		if result.err != nil {
			return nil, fmt.Errorf("%w: %w", errUtils.ErrWebflowAuthFailed, result.err)
		}
		return exchangeCodeForCredentials(ctx, defaultHTTPClient, region, result.code, verifier, redirectURI)
	case <-ctx.Done():
		return nil, fmt.Errorf("%w: %w", errUtils.ErrWebflowTimeout, ctx.Err())
	}
}

// processTokenResponse converts a token response to AWS credentials and caches the refresh token.
func (i *userIdentity) processTokenResponse(tokenResp *webflowTokenResponse, region string) (*types.AWSCredentials, error) {
	creds := tokenResponseToCredentials(tokenResp, region)

	// Cache refresh token for future use.
	if tokenResp.RefreshToken != "" {
		i.saveRefreshCache(&webflowRefreshCache{
			RefreshToken: tokenResp.RefreshToken,
			Region:       region,
			ExpiresAt:    time.Now().Add(webflowSessionDuration),
		})
	}

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

// generatePKCEPair generates a PKCE code verifier and code challenge.
// The verifier is 32 random bytes, base64url-encoded without padding.
// The challenge is the SHA-256 hash of the verifier, base64url-encoded without padding.
func generatePKCEPair() (verifier string, challenge string, err error) {
	defer perf.Track(nil, "aws.generatePKCEPair")()

	buf := make([]byte, webflowCodeVerifierBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", "", fmt.Errorf("failed to generate random bytes: %w", err)
	}

	verifier = base64.RawURLEncoding.EncodeToString(buf)

	hash := sha256.Sum256([]byte(verifier))
	challenge = base64.RawURLEncoding.EncodeToString(hash[:])

	return verifier, challenge, nil
}

// generateRandomString generates a random string of the specified byte length, base64url-encoded.
func generateRandomString(n int) (string, error) {
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("failed to generate random bytes: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

// getSigninEndpoint returns the AWS signin endpoint for the given region.
// us-east-1 uses the global endpoint (no region prefix).
func getSigninEndpoint(region string) string {
	if region == "us-east-1" {
		return "https://signin.aws.amazon.com"
	}
	return fmt.Sprintf("https://%s.signin.aws.amazon.com", region)
}

// buildAuthorizationURL constructs the OAuth2 authorization URL.
func buildAuthorizationURL(region string, port int, codeChallenge, state string) string {
	baseURL := getSigninEndpoint(region)
	params := url.Values{}
	params.Set("client_id", webflowOAuthClientID)
	params.Set("redirect_uri", fmt.Sprintf("http://127.0.0.1:%d%s", port, webflowCallbackPath))
	params.Set("response_type", webflowResponseType)
	params.Set("code_challenge", codeChallenge)
	params.Set("code_challenge_method", webflowCodeChallengeMethod)
	params.Set("scope", webflowScope)
	params.Set("state", state)
	return fmt.Sprintf("%s/v1/authorize?%s", baseURL, params.Encode())
}

// startCallbackServer starts an HTTP server on an ephemeral port to receive the OAuth2 callback.
// It returns the listener, a channel for the result, and any error.
func startCallbackServer(ctx context.Context, expectedState string) (net.Listener, <-chan webflowResult, error) {
	defer perf.Track(nil, "aws.startCallbackServer")()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, nil, fmt.Errorf("failed to bind to loopback address: %w", err)
	}

	resultCh := make(chan webflowResult, 1)

	mux := http.NewServeMux()
	mux.HandleFunc(webflowCallbackPath, func(w http.ResponseWriter, r *http.Request) {
		code := r.URL.Query().Get("code")
		state := r.URL.Query().Get("state")
		errParam := r.URL.Query().Get("error")

		if errParam != "" {
			errDesc := r.URL.Query().Get("error_description")
			resultCh <- webflowResult{err: fmt.Errorf("authorization error: %s: %s", errParam, errDesc)}
			http.Error(w, "Authorization failed. You can close this tab.", http.StatusBadRequest)
			return
		}

		if code == "" {
			resultCh <- webflowResult{err: fmt.Errorf("missing authorization code in callback")}
			http.Error(w, "Missing authorization code. You can close this tab.", http.StatusBadRequest)
			return
		}

		if state != expectedState {
			resultCh <- webflowResult{err: fmt.Errorf("state mismatch: possible CSRF attack")}
			http.Error(w, "State mismatch. You can close this tab.", http.StatusBadRequest)
			return
		}

		resultCh <- webflowResult{code: code, state: state}

		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, `<!DOCTYPE html><html><body><h2>Authentication successful!</h2><p>You can close this tab and return to your terminal.</p><script>window.close()</script></body></html>`)
	})

	server := &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		if err := server.Serve(listener); err != nil && err != http.ErrServerClosed {
			log.Debug("Callback server error", "error", err)
		}
	}()

	// Shut down server when context is cancelled.
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		server.Shutdown(shutdownCtx) //nolint:errcheck // Best-effort shutdown.
	}()

	return listener, resultCh, nil
}

// exchangeCodeForCredentials exchanges an authorization code for AWS credentials.
func exchangeCodeForCredentials(ctx context.Context, client HTTPClient, region, code, codeVerifier, redirectURI string) (*webflowTokenResponse, error) {
	defer perf.Track(nil, "aws.exchangeCodeForCredentials")()

	body := map[string]string{
		"clientId":     webflowOAuthClientID,
		"grantType":    webflowGrantTypeAuthCode,
		"code":         code,
		"codeVerifier": codeVerifier,
		"redirectUri":  redirectURI,
	}

	return callTokenEndpoint(ctx, client, region, body)
}

// exchangeRefreshToken exchanges a refresh token for new AWS credentials.
func exchangeRefreshToken(ctx context.Context, client HTTPClient, region, refreshToken string) (*webflowTokenResponse, error) {
	defer perf.Track(nil, "aws.exchangeRefreshToken")()

	body := map[string]string{
		"clientId":     webflowOAuthClientID,
		"grantType":    webflowGrantTypeRefresh,
		"refreshToken": refreshToken,
	}

	return callTokenEndpoint(ctx, client, region, body)
}

// callTokenEndpoint makes a POST request to the AWS signin token endpoint.
func callTokenEndpoint(ctx context.Context, client HTTPClient, region string, body map[string]string) (*webflowTokenResponse, error) {
	endpoint := fmt.Sprintf("%s/v1/token", getSigninEndpoint(region))

	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("%w: failed to marshal request body: %w", errUtils.ErrWebflowTokenExchange, err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(string(jsonBody)))
	if err != nil {
		return nil, fmt.Errorf("%w: failed to create request: %w", errUtils.ErrWebflowTokenExchange, err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, errUtils.Build(errUtils.ErrWebflowTokenExchange).
			WithCause(err).
			WithExplanation("Failed to contact the AWS signin service").
			WithHint("Check your network connectivity").
			WithHintf("Ensure the region '%s' is correct", region).
			WithContext("endpoint", endpoint).
			Err()
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, webflowTokenMaxBodyBytes))
	if err != nil {
		return nil, fmt.Errorf("%w: failed to read response: %w", errUtils.ErrWebflowTokenExchange, err)
	}

	if resp.StatusCode != http.StatusOK {
		var errResp webflowTokenErrorResponse
		if jsonErr := json.Unmarshal(respBody, &errResp); jsonErr == nil && errResp.Error != "" {
			return nil, errUtils.Build(errUtils.ErrWebflowTokenExchange).
				WithExplanationf("AWS signin service returned error: %s", errResp.Error).
				WithHintf("%s", errResp.ErrorDescription).
				WithContext("status", fmt.Sprintf("%d", resp.StatusCode)).
				WithContext("endpoint", endpoint).
				Err()
		}
		return nil, errUtils.Build(errUtils.ErrWebflowTokenExchange).
			WithExplanationf("AWS signin service returned HTTP %d", resp.StatusCode).
			WithHint("Ensure you completed authentication in the browser").
			WithContext("endpoint", endpoint).
			Err()
	}

	var tokenResp webflowTokenResponse
	if err := json.Unmarshal(respBody, &tokenResp); err != nil {
		return nil, fmt.Errorf("%w: failed to parse token response: %w", errUtils.ErrWebflowTokenExchange, err)
	}

	if tokenResp.AccessToken.AccessKeyID == "" || tokenResp.AccessToken.SecretAccessKey == "" {
		return nil, fmt.Errorf("%w: token response missing credentials", errUtils.ErrWebflowTokenExchange)
	}

	return &tokenResp, nil
}

// tokenResponseToCredentials converts a token response to AWSCredentials.
func tokenResponseToCredentials(resp *webflowTokenResponse, region string) *types.AWSCredentials {
	expiration := ""
	if resp.ExpiresIn > 0 {
		expiration = time.Now().Add(time.Duration(resp.ExpiresIn) * time.Second).Format(time.RFC3339)
	}

	return &types.AWSCredentials{
		AccessKeyID:     resp.AccessToken.AccessKeyID,
		SecretAccessKey: resp.AccessToken.SecretAccessKey,
		SessionToken:    resp.AccessToken.SessionToken,
		Region:          region,
		Expiration:      expiration,
	}
}

// Refresh token cache (XDG file-based, following SSO cache pattern).

// getRefreshCachePath returns the XDG-compliant cache path for the refresh token.
func (i *userIdentity) getRefreshCachePath() (string, error) {
	cacheDir, err := xdg.GetXDGCacheDir(webflowCacheSubdir, webflowCacheDirPerms)
	if err != nil {
		return "", fmt.Errorf("failed to get XDG cache directory: %w", err)
	}

	identityDir := fmt.Sprintf("%s-%s", i.name, i.realm)
	fullDir := filepath.Join(cacheDir, identityDir)
	if err := os.MkdirAll(fullDir, webflowCacheDirPerms); err != nil {
		return "", fmt.Errorf("failed to create cache directory: %w", err)
	}

	return filepath.Join(fullDir, webflowCacheFilename), nil
}

// loadRefreshCache loads the cached refresh token.
func (i *userIdentity) loadRefreshCache() (*webflowRefreshCache, error) {
	path, err := i.getRefreshCachePath()
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("no cached refresh token: %w", err)
	}

	var cache webflowRefreshCache
	if err := json.Unmarshal(data, &cache); err != nil {
		return nil, fmt.Errorf("failed to parse refresh cache: %w", err)
	}

	if cache.RefreshToken == "" {
		return nil, fmt.Errorf("cached refresh token is empty")
	}

	return &cache, nil
}

// saveRefreshCache saves the refresh token to cache.
func (i *userIdentity) saveRefreshCache(cache *webflowRefreshCache) {
	path, err := i.getRefreshCachePath()
	if err != nil {
		log.Debug("Failed to get refresh cache path", logKeyIdentity, i.name, "error", err)
		return
	}

	data, err := json.MarshalIndent(cache, "", "  ")
	if err != nil {
		log.Debug("Failed to marshal refresh cache", logKeyIdentity, i.name, "error", err)
		return
	}

	if err := os.WriteFile(path, data, webflowCacheFilePerms); err != nil {
		log.Debug("Failed to write refresh cache", logKeyIdentity, i.name, "error", err)
		return
	}

	log.Debug("Saved webflow refresh token to cache", logKeyIdentity, i.name, "expiresAt", cache.ExpiresAt)
}

// deleteRefreshCache removes the cached refresh token.
func (i *userIdentity) deleteRefreshCache() {
	path, err := i.getRefreshCachePath()
	if err != nil {
		return
	}

	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		log.Debug("Failed to delete refresh cache", logKeyIdentity, i.name, "error", err)
	}
}

// isTransientTokenError checks if a token exchange error is transient (network/timeout)
// vs a definitive server rejection (invalid_grant). Transient errors should not
// invalidate cached refresh tokens since the token may still be valid.
func isTransientTokenError(err error) bool {
	if err == nil {
		return false
	}
	// Context cancellation/timeout is transient.
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	// Network errors (connectivity, DNS, etc.) are transient.
	var netErr net.Error
	if errors.As(err, &netErr) {
		return true
	}
	return false
}

// TTY detection helpers (mirrors providers/aws/sso.go pattern).

// webflowIsTTY checks if stderr is a terminal.
func webflowIsTTY() bool {
	return isatty.IsTerminal(os.Stderr.Fd()) || isatty.IsCygwinTerminal(os.Stderr.Fd())
}

// webflowIsInteractive checks if we're running in an interactive terminal.
// Respects --force-tty / ATMOS_FORCE_TTY for environments where TTY detection fails.
func webflowIsInteractive() bool {
	if viper.GetBool("force-tty") {
		return true
	}
	return webflowIsTTY()
}

// UI display functions.

// displayWebflowDialog shows a styled dialog with the authentication URL (TTY mode).
func displayWebflowDialog(authURL string) {
	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color(theme.ColorCyan)).
		PaddingLeft(1).
		PaddingRight(1)

	urlStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(theme.ColorBorder)).
		Italic(true)

	instructionStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(theme.ColorDarkGray))

	boxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(theme.ColorBorder)).
		Padding(1, 2).
		MarginTop(1).
		MarginBottom(1)

	var content strings.Builder
	content.WriteString(titleStyle.Render("🔐 AWS Browser Authentication"))
	content.WriteString("\n\n")
	content.WriteString(instructionStyle.Render("Opening browser for authentication..."))
	content.WriteString("\n")
	content.WriteString(instructionStyle.Render("If the browser doesn't open, visit:"))
	content.WriteString("\n\n")
	content.WriteString(urlStyle.Render(authURL))

	fmt.Fprintf(os.Stderr, "%s\n", boxStyle.Render(content.String()))
}

// displayWebflowDialogPlainText shows the authentication URL in plain text (non-TTY).
func displayWebflowDialogPlainText(authURL string) {
	utils.PrintfMessageToTUI("🔐 **AWS Browser Authentication (Non-Interactive)**\n")
	utils.PrintfMessageToTUI("Visit this URL on a device with a browser:\n")
	utils.PrintfMessageToTUI("%s\n", authURL)
	utils.PrintfMessageToTUI("After signing in, paste the authorization code below.\n")
}

// Spinner model for interactive waiting (follows SSO pattern from sso.go).

// webflowSpinnerTokenResult wraps the token exchange result.
type webflowSpinnerTokenResult struct {
	resp *webflowTokenResponse
	err  error
}

// webflowSpinnerModel is a bubbletea model for the authentication spinner.
type webflowSpinnerModel struct {
	spinner spinner.Model
	message string
	done    bool
	tokenCh <-chan webflowSpinnerTokenResult
	cancel  context.CancelFunc
	result  *webflowSpinnerTokenResult
}

//nolint:gocritic // Bubbletea framework requires value receivers, not pointer receivers.
func (m webflowSpinnerModel) Init() tea.Cmd {
	return tea.Batch(m.spinner.Tick, m.checkResult())
}

//nolint:gocritic // Bubbletea framework requires value receivers, not pointer receivers.
func (m webflowSpinnerModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if msg.String() == "ctrl+c" {
			m.done = true
			m.result = &webflowSpinnerTokenResult{err: errUtils.ErrUserAborted}
			if m.cancel != nil {
				m.cancel()
			}
			return m, tea.Quit
		}
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	case webflowSpinnerTokenResult:
		m.done = true
		m.result = &msg
		if m.cancel != nil {
			m.cancel()
		}
		return m, tea.Quit
	}
	return m, nil
}

//nolint:gocritic // Bubbletea framework requires value receivers, not pointer receivers.
func (m webflowSpinnerModel) View() string {
	if m.done {
		if m.result != nil && m.result.err != nil {
			return fmt.Sprintf("%s Authentication failed\n", theme.Styles.XMark)
		}
		return ""
	}
	return fmt.Sprintf("%s %s...", m.spinner.View(), m.message)
}

//nolint:gocritic // Bubbletea framework requires value receivers, not pointer receivers.
func (m webflowSpinnerModel) checkResult() tea.Cmd {
	return func() tea.Msg {
		select {
		case res := <-m.tokenCh:
			return webflowSpinnerTokenResult{resp: res.resp, err: res.err}
		case <-time.After(100 * time.Millisecond):
			return m.checkResult()()
		}
	}
}
