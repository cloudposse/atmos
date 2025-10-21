package github

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os/exec"
	"runtime"
	"strings"
	"time"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
)

const (
	// WebFlowHTTPTimeout is the timeout for HTTP requests during Web Flow.
	WebFlowHTTPTimeout = 30 * time.Second

	// WebFlowStateBytes is the number of random bytes for the CSRF state parameter.
	WebFlowStateBytes = 32

	// WebFlowServerReadTimeout is the read header timeout for the local HTTP server.
	WebFlowServerReadTimeout = 10 * time.Second

	// SuccessPageHTML is the HTML response shown after successful authentication.
	SuccessPageHTML = `<!DOCTYPE html>
<html>
<head>
    <title>Authentication Successful</title>
    <style>
        body { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Helvetica, Arial, sans-serif;
               padding: 50px; text-align: center; background: #f6f8fa; }
        .success { color: #28a745; font-size: 24px; margin-bottom: 20px; }
        .message { color: #586069; }
    </style>
</head>
<body>
    <div class="success">✓ Authentication Successful!</div>
    <div class="message">You can close this window and return to the terminal.</div>
</body>
</html>`
)

// WebFlowClient handles GitHub OAuth Web Application Flow.
// This flow starts a local HTTP server, opens the browser for authorization,
// receives the callback with authorization code, and exchanges it for an access token.
type WebFlowClient interface {
	// StartWebFlow initiates the OAuth Web Application Flow.
	// Returns the authorization URL and state for CSRF protection.
	StartWebFlow(ctx context.Context) (authURL string, state string, err error)

	// WaitForCallback starts a local HTTP server and waits for the OAuth callback.
	// Returns the access token after successful authorization.
	WaitForCallback(ctx context.Context, state string) (token string, err error)
}

// realWebFlowClient implements WebFlowClient for production use.
type realWebFlowClient struct {
	clientID     string
	clientSecret string
	scopes       []string
	callbackPort int
	httpClient   *http.Client
	server       *http.Server
	tokenChan    chan string
	errorChan    chan error
}

// NewWebFlowClient creates a new WebFlowClient.
func NewWebFlowClient(clientID, clientSecret string, scopes []string) WebFlowClient {
	return &realWebFlowClient{
		clientID:     clientID,
		clientSecret: clientSecret,
		scopes:       scopes,
		httpClient: &http.Client{
			Timeout: WebFlowHTTPTimeout,
		},
		tokenChan: make(chan string, 1),
		errorChan: make(chan error, 1),
	}
}

// StartWebFlow initiates the OAuth Web Application Flow.
func (c *realWebFlowClient) StartWebFlow(ctx context.Context) (string, string, error) {
	defer perf.Track(nil, "github.realWebFlowClient.StartWebFlow")()

	// Generate random state for CSRF protection.
	stateBytes := make([]byte, WebFlowStateBytes)
	if _, err := rand.Read(stateBytes); err != nil {
		return "", "", fmt.Errorf("%w: failed to generate state: %v", errUtils.ErrAuthenticationFailed, err)
	}
	state := base64.URLEncoding.EncodeToString(stateBytes)

	// Find available port for local server.
	port, err := findAvailablePort()
	if err != nil {
		return "", "", fmt.Errorf("%w: failed to find available port: %v", errUtils.ErrAuthenticationFailed, err)
	}
	c.callbackPort = port

	// Build authorization URL.
	authURL := c.buildAuthURL(state)

	return authURL, state, nil
}

// WaitForCallback starts a local HTTP server and waits for the OAuth callback.
func (c *realWebFlowClient) WaitForCallback(ctx context.Context, state string) (string, error) {
	defer perf.Track(nil, "github.realWebFlowClient.WaitForCallback")()

	// Start local HTTP server.
	mux := http.NewServeMux()
	mux.HandleFunc("/callback", c.handleCallback(state))

	c.server = &http.Server{
		Addr:              fmt.Sprintf("127.0.0.1:%d", c.callbackPort),
		Handler:           mux,
		ReadHeaderTimeout: WebFlowServerReadTimeout,
	}

	// Start server in goroutine.
	go func() {
		if err := c.server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			c.errorChan <- fmt.Errorf("%w: failed to start callback server: %v", errUtils.ErrAuthenticationFailed, err)
		}
	}()

	// Wait for either token, error, or context cancellation.
	select {
	case token := <-c.tokenChan:
		_ = c.server.Shutdown(context.Background())
		return token, nil
	case err := <-c.errorChan:
		_ = c.server.Shutdown(context.Background())
		return "", err
	case <-ctx.Done():
		_ = c.server.Shutdown(context.Background())
		return "", fmt.Errorf("%w: authentication cancelled", errUtils.ErrAuthenticationFailed)
	}
}

// handleCallback handles the OAuth callback from GitHub.
func (c *realWebFlowClient) handleCallback(expectedState string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Parse query parameters.
		query := r.URL.Query()
		code := query.Get("code")
		state := query.Get("state")
		errorParam := query.Get("error")
		errorDescription := query.Get("error_description")

		// Check for OAuth errors.
		if errorParam != "" {
			msg := fmt.Sprintf("GitHub OAuth error: %s", errorParam)
			if errorDescription != "" {
				msg += fmt.Sprintf(" - %s", errorDescription)
			}
			c.errorChan <- fmt.Errorf("%w: %s", errUtils.ErrAuthenticationFailed, msg)
			http.Error(w, "Authentication failed. You can close this window.", http.StatusBadRequest)
			return
		}

		// Verify state for CSRF protection.
		if state != expectedState {
			c.errorChan <- fmt.Errorf("%w: state mismatch (possible CSRF attack)", errUtils.ErrAuthenticationFailed)
			http.Error(w, "Invalid state parameter. You can close this window.", http.StatusBadRequest)
			return
		}

		// Check for authorization code.
		if code == "" {
			c.errorChan <- fmt.Errorf("%w: no authorization code received", errUtils.ErrAuthenticationFailed)
			http.Error(w, "No authorization code received. You can close this window.", http.StatusBadRequest)
			return
		}

		// Exchange code for token.
		token, err := c.exchangeCodeForToken(code)
		if err != nil {
			c.errorChan <- err
			http.Error(w, "Failed to exchange code for token. You can close this window.", http.StatusInternalServerError)
			return
		}

		// Success - send token and display success page.
		c.tokenChan <- token
		c.renderSuccessPage(w)
	}
}

// renderSuccessPage renders the HTML success page after authentication.
func (c *realWebFlowClient) renderSuccessPage(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/html")
	fmt.Fprint(w, SuccessPageHTML)
}

// exchangeCodeForToken exchanges the authorization code for an access token.
func (c *realWebFlowClient) exchangeCodeForToken(code string) (string, error) {
	defer perf.Track(nil, "github.realWebFlowClient.exchangeCodeForToken")()

	// Build token exchange request.
	data := url.Values{}
	data.Set("client_id", c.clientID)
	data.Set("client_secret", c.clientSecret)
	data.Set("code", code)

	req, err := http.NewRequest("POST", "https://github.com/login/oauth/access_token", strings.NewReader(data.Encode()))
	if err != nil {
		return "", fmt.Errorf("%w: failed to create token request: %v", errUtils.ErrAuthenticationFailed, err)
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	// Send request.
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("%w: failed to exchange code for token: %v", errUtils.ErrAuthenticationFailed, err)
	}
	defer resp.Body.Close()

	// Read response.
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("%w: failed to read token response: %v", errUtils.ErrAuthenticationFailed, err)
	}

	// Check HTTP status.
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("%w: token exchange failed with status %d: %s", errUtils.ErrAuthenticationFailed, resp.StatusCode, string(body))
	}

	// Parse response.
	var tokenResp struct {
		AccessToken string `json:"access_token"`
		TokenType   string `json:"token_type"`
		Scope       string `json:"scope"`
		Error       string `json:"error"`
		ErrorDesc   string `json:"error_description"`
	}

	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return "", fmt.Errorf("%w: failed to parse token response: %v", errUtils.ErrAuthenticationFailed, err)
	}

	// Check for errors in response.
	if tokenResp.Error != "" {
		return "", fmt.Errorf("%w: %s: %s", errUtils.ErrAuthenticationFailed, tokenResp.Error, tokenResp.ErrorDesc)
	}

	if tokenResp.AccessToken == "" {
		return "", fmt.Errorf("%w: no access token in response", errUtils.ErrAuthenticationFailed)
	}

	return tokenResp.AccessToken, nil
}

// buildAuthURL builds the GitHub OAuth authorization URL.
func (c *realWebFlowClient) buildAuthURL(state string) string {
	params := url.Values{}
	params.Set("client_id", c.clientID)
	params.Set("redirect_uri", fmt.Sprintf("http://127.0.0.1:%d/callback", c.callbackPort))
	params.Set("state", state)
	params.Set("scope", strings.Join(c.scopes, " "))

	return fmt.Sprintf("https://github.com/login/oauth/authorize?%s", params.Encode())
}

// findAvailablePort finds an available TCP port for the local HTTP server.
func findAvailablePort() (int, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer listener.Close()

	addr := listener.Addr().(*net.TCPAddr)
	return addr.Port, nil
}

// OpenBrowser opens the default browser to the specified URL.
func OpenBrowser(url string) error {
	defer perf.Track(nil, "github.OpenBrowser")()

	var cmd *exec.Cmd

	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "linux":
		cmd = exec.Command("xdg-open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		return fmt.Errorf("%w: %s", errUtils.ErrUnsupportedPlatform, runtime.GOOS)
	}

	return cmd.Start()
}
