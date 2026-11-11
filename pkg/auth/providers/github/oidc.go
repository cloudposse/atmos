package github

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/spf13/viper"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth/types"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/retry"
	"github.com/cloudposse/atmos/pkg/schema"
)

// OidcTimeout is the per-attempt timeout, in seconds, for a token request.
const OidcTimeout = 10

// Retry policy for the token request. The endpoint occasionally hangs or returns 5xx for a
// request while the next one succeeds, so a few spaced-out attempts turn a failed command into a
// slower one. Worst case is about 4 x 10 s of attempts plus 1 + 2 + 4 s of backoff (+/- jitter).
const (
	oidcMaxAttempts    = 4
	oidcInitialBackoff = 1 * time.Second
	oidcMaxBackoff     = 8 * time.Second
	oidcBackoffJitter  = 0.5
	oidcMaxDrainBytes  = 4 << 10 // Error-response bytes read so the connection can be reused.
	oidcMaxBodyBytes   = 1 << 20 // Upper bound on a token response; a JWT is a few KiB.
	// Fallback transport settings, matching net/http's DefaultTransport.
	oidcTLSHandshakeTimeout = 10 * time.Second
	oidcIdleConnTimeout     = 90 * time.Second
	logKeyProvider          = "provider"
)

// defaultOIDCHTTPClient is shared by every provider instance so repeated requests can reuse
// connections. Its Timeout bounds each attempt, not the whole retry sequence.
var defaultOIDCHTTPClient = newOIDCHTTPClient()

// newOIDCHTTPClient returns a client with its own connection pool, so connection handling here
// never touches other users of http.DefaultTransport.
func newOIDCHTTPClient() *http.Client {
	transport, ok := http.DefaultTransport.(*http.Transport)
	if ok {
		transport = transport.Clone()
	} else {
		// http.DefaultTransport was replaced (for example by instrumentation). Build a pool of our
		// own rather than fall back to it, since a nil Transport would resolve to the shared one.
		transport = &http.Transport{
			Proxy:               http.ProxyFromEnvironment,
			ForceAttemptHTTP2:   true,
			TLSHandshakeTimeout: oidcTLSHandshakeTimeout,
			IdleConnTimeout:     oidcIdleConnTimeout,
		}
	}
	return &http.Client{Timeout: OidcTimeout * time.Second, Transport: transport}
}

// oidcProvider implements GitHub OIDC authentication.
type oidcProvider struct {
	name       string
	config     *schema.Provider
	realm      string       // Credential isolation realm set by auth manager.
	httpClient *http.Client // Optional; if nil, the shared default client is used.
	clock      retry.Clock  // Optional; retry.WithClock ignores nil and keeps wall-clock time. Tests inject a fake.
}

// NewOIDCProvider creates a new GitHub OIDC provider.
func NewOIDCProvider(name string, config *schema.Provider) (types.Provider, error) {
	if config == nil {
		return nil, fmt.Errorf("%w: provider config is required", errUtils.ErrInvalidProviderConfig)
	}

	if name == "" {
		return nil, fmt.Errorf("%w: provider name is required", errUtils.ErrInvalidProviderConfig)
	}

	return &oidcProvider{
		name:   name,
		config: config,
	}, nil
}

// getHTTPClient returns the HTTP client to use for requests.
func (p *oidcProvider) getHTTPClient() *http.Client {
	if p.httpClient != nil {
		return p.httpClient
	}
	return defaultOIDCHTTPClient
}

// now returns the current time from the injected clock, or wall-clock time.
func (p *oidcProvider) now() time.Time {
	if p.clock != nil {
		return p.clock.Now()
	}
	return time.Now()
}

// Name returns the provider name.
func (p *oidcProvider) Name() string {
	return p.name
}

// SetRealm sets the credential isolation realm for this provider.
func (p *oidcProvider) SetRealm(realm string) {
	p.realm = realm
}

// PreAuthenticate is a no-op for GitHub OIDC provider.
func (p *oidcProvider) PreAuthenticate(_ types.AuthManager) error {
	return nil
}

// IsAmbient satisfies types.AmbientProvider. The OIDC token is requested from the GitHub
// Actions token endpoint (ACTIONS_ID_TOKEN_REQUEST_URL / _TOKEN) and is short-lived.
// Persisting it would let the auth manager replay a token that the Actions runtime has
// already rotated, so the manager must never cache credentials for chains rooted at this
// provider. The provider itself reuses a token only within one process, in memory, while
// it has more than oidcTokenMinRemaining left (see processJWTCache).
func (p *oidcProvider) IsAmbient() bool {
	return true
}

// Kind returns the provider kind.
func (p *oidcProvider) Kind() string {
	return "github/oidc"
}

// Authenticate performs GitHub OIDC authentication.
func (p *oidcProvider) Authenticate(ctx context.Context) (types.ICredentials, error) {
	log.Info("Starting GitHub OIDC authentication", "provider", p.name)

	// Validate provider configuration early.
	if err := p.Validate(); err != nil {
		return nil, err
	}

	// Check if we're running in GitHub Actions.
	if !p.isGitHubActions() {
		return nil, fmt.Errorf("%w: GitHub OIDC authentication is only available in GitHub Actions environment", errUtils.ErrAuthenticationFailed)
	}

	requestURL, token, err := p.requestParams()
	if err != nil {
		return nil, err
	}

	aud, err := p.audience()
	if err != nil {
		return nil, err
	}

	jwtToken, err := p.resolveJWT(ctx, requestURL, token, aud)
	if err != nil {
		return nil, err
	}

	log.Info("GitHub OIDC authentication successful", "provider", p.name)

	// Return the JWT token as credentials (used by downstream identities).
	return &types.OIDCCredentials{
		Token:    jwtToken,
		Provider: "github",
		Audience: aud,
	}, nil
}

// isGitHubActions checks if we're running in GitHub Actions environment.
func (p *oidcProvider) isGitHubActions() bool {
	if err := viper.BindEnv("github.actions", "GITHUB_ACTIONS"); err != nil {
		log.Trace("Failed to bind github.actions environment variable", "error", err)
	}
	return viper.GetString("github.actions") == "true"
}

// requestParams loads the request URL and token from the GitHub Actions environment.
func (p *oidcProvider) requestParams() (string, string, error) {
	if err := viper.BindEnv("github.oidc.request_token", "ACTIONS_ID_TOKEN_REQUEST_TOKEN"); err != nil {
		log.Trace("Failed to bind github.oidc.request_token environment variable", "error", err)
	}
	token := viper.GetString("github.oidc.request_token")
	if token == "" {
		return "", "", fmt.Errorf("%w: ACTIONS_ID_TOKEN_REQUEST_TOKEN not found - ensure job has 'id-token: write' permission", errUtils.ErrAuthenticationFailed)
	}

	if err := viper.BindEnv("github.oidc.request_url", "ACTIONS_ID_TOKEN_REQUEST_URL"); err != nil {
		log.Trace("Failed to bind github.oidc.request_url environment variable", "error", err)
	}
	requestURL := viper.GetString("github.oidc.request_url")
	if requestURL == "" {
		return "", "", fmt.Errorf("%w: ACTIONS_ID_TOKEN_REQUEST_URL not found - ensure job has 'id-token: write' permission", errUtils.ErrAuthenticationFailed)
	}

	// Validate the URL to prevent SSRF: scheme must be https and host must be the
	// GitHub Actions token endpoint.
	if err := validateGitHubActionsURL(requestURL); err != nil {
		return "", "", err
	}

	return requestURL, token, nil
}

// validateGitHubActionsURL validates that the URL is a safe GitHub Actions OIDC endpoint.
// This prevents SSRF by ensuring the URL uses HTTPS and has a non-empty host.
func validateGitHubActionsURL(rawURL string) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("%w: invalid ACTIONS_ID_TOKEN_REQUEST_URL: %w", errUtils.ErrAuthenticationFailed, err)
	}
	if u.Scheme != "https" {
		return fmt.Errorf("%w: ACTIONS_ID_TOKEN_REQUEST_URL must use https scheme, got %q", errUtils.ErrAuthenticationFailed, u.Scheme)
	}
	if u.Hostname() == "" {
		return fmt.Errorf("%w: ACTIONS_ID_TOKEN_REQUEST_URL must have a non-empty host", errUtils.ErrAuthenticationFailed)
	}
	return nil
}

// audience extracts the required audience from provider config.
func (p *oidcProvider) audience() (string, error) {
	if p.config != nil && p.config.Spec != nil {
		if v, ok := p.config.Spec["audience"].(string); ok && v != "" {
			return v, nil
		}
		return "", fmt.Errorf("%w: audience is required in provider spec", errUtils.ErrInvalidProviderConfig)
	}
	return "", nil
}

// resolveJWT returns an ACTIONS_ID_TOKEN if present, otherwise fetches a token from the endpoint.
func (p *oidcProvider) resolveJWT(ctx context.Context, requestURL, token, aud string) (string, error) {
	if err := viper.BindEnv("github.oidc.id_token", "ACTIONS_ID_TOKEN"); err != nil {
		log.Trace("Failed to bind github.oidc.id_token environment variable", "error", err)
	}
	if jwt := viper.GetString("github.oidc.id_token"); jwt != "" {
		return jwt, nil
	}
	jwtToken, err := p.getOIDCToken(ctx, requestURL, token, aud)
	if err != nil {
		return "", errors.Join(errUtils.ErrAuthenticationFailed, err)
	}
	return jwtToken, nil
}

// getOIDCToken returns a JWT for the audience, reusing one fetched earlier in this process while
// it is still fresh. Concurrent callers that miss the cache share a single fetch.
func (p *oidcProvider) getOIDCToken(ctx context.Context, requestURL, requestToken, audience string) (string, error) {
	key := jwtCacheKey(requestURL, requestToken, audience)
	if token, ok := processJWTCache.get(key, p.now()); ok {
		log.Debug("Reusing GitHub OIDC token fetched earlier in this process", logKeyProvider, p.name)
		return token, nil
	}

	for {
		token, abandoned, err := p.joinOIDCTokenFlight(ctx, key, requestURL, requestToken, audience)
		// A flight abandoned because its starter's context ended says nothing about this caller or
		// the endpoint, so a caller whose own context is still live starts (or joins) a new one.
		if abandoned && ctx.Err() == nil {
			log.Debug("Shared GitHub OIDC token request was cancelled by another caller; requesting again", logKeyProvider, p.name)
			continue
		}
		return token, err
	}
}

// jwtFlightResult is the value a shared token fetch hands to every caller waiting on it.
type jwtFlightResult struct {
	token string
	// abandoned reports that the fetch stopped because the context of the caller that started it
	// ended, rather than because the endpoint failed.
	abandoned bool
}

// joinOIDCTokenFlight starts the fetch for key, or waits for one already in flight. The fetch runs
// with the context of the caller that started it; a caller whose own context ends first stops
// waiting and returns its context error.
func (p *oidcProvider) joinOIDCTokenFlight(ctx context.Context, key, requestURL, requestToken, audience string) (string, bool, error) {
	flight := processJWTCache.flights.DoChan(key, func() (any, error) {
		// Another flight may have filled the cache between the caller's lookup and this one.
		if token, ok := processJWTCache.get(key, p.now()); ok {
			return jwtFlightResult{token: token}, nil
		}
		token, err := p.fetchOIDCTokenWithRetry(ctx, requestURL, requestToken, audience)
		if err != nil {
			// Only a failure caused by the starter's cancellation counts; an endpoint error that lands
			// as the context ends is a real answer and is returned to every waiter as is.
			return jwtFlightResult{abandoned: ctx.Err() != nil && errors.Is(err, ctx.Err())}, err
		}
		processJWTCache.put(key, token, p.now())
		return jwtFlightResult{token: token}, nil
	})

	select {
	case <-ctx.Done():
		return "", false, fmt.Errorf("%w: wait for OIDC token: %w", errUtils.ErrAuthenticationFailed, ctx.Err())
	case res := <-flight:
		result, _ := res.Val.(jwtFlightResult)
		return result.token, result.abandoned, res.Err
	}
}

// fetchOIDCTokenWithRetry requests a JWT, retrying timeouts, transport errors, 429 and 5xx with
// exponential backoff and jitter. Other failures, and a cancelled ctx, end the sequence at once.
func (p *oidcProvider) fetchOIDCTokenWithRetry(ctx context.Context, requestURL, requestToken, audience string) (string, error) {
	executor := retry.New(oidcRetryConfig(), retry.WithClock(p.clock))
	shouldRetry := func(err error) bool { return isRetryableOIDCError(ctx, err) }

	var token string
	attempt := 0
	err := executor.ExecuteWithPredicate(ctx, func() error {
		attempt++
		client := p.getHTTPClient()
		if attempt > 1 {
			client = freshConnectionClient(client)
			defer client.CloseIdleConnections()
		}
		t, err := fetchOIDCTokenOnce(ctx, client, requestURL, requestToken, audience)
		if err != nil {
			if attempt < oidcMaxAttempts && shouldRetry(err) {
				log.Warn("GitHub OIDC token request failed, retrying", logKeyProvider, p.name,
					"attempt", attempt, "max_attempts", oidcMaxAttempts, "error", err)
			}
			return err
		}
		token = t
		return nil
	}, shouldRetry)
	if err != nil {
		return "", err
	}
	return token, nil
}

// freshConnectionClient returns a copy of client whose transport has an empty connection pool.
// A timed-out attempt can leave its connection in the shared pool, and over HTTP/2 a retry would
// be multiplexed onto it, so one stalled connection would fail every attempt. Dropping idle
// connections is not enough: the timed-out stream is torn down asynchronously, so the connection
// can still look busy when the retry starts. A client whose transport is not an *http.Transport
// cannot be cloned; it is returned as is after dropping its idle connections.
func freshConnectionClient(client *http.Client) *http.Client {
	transport, ok := client.Transport.(*http.Transport)
	if !ok {
		client.CloseIdleConnections()
		return client
	}
	fresh := *client
	fresh.Transport = transport.Clone()
	return &fresh
}

// oidcRetryConfig returns the retry policy for the token request.
func oidcRetryConfig() schema.RetryConfig {
	maxAttempts := oidcMaxAttempts
	initialDelay := oidcInitialBackoff
	maxDelay := oidcMaxBackoff
	jitter := oidcBackoffJitter
	return schema.RetryConfig{
		MaxAttempts:     &maxAttempts,
		BackoffStrategy: schema.BackoffExponential,
		InitialDelay:    &initialDelay,
		MaxDelay:        &maxDelay,
		RandomJitter:    &jitter,
	}
}

// isRetryableOIDCError reports whether a failed attempt is worth repeating. A cancelled or expired
// ctx is never retried, even though the transport reports it like any other request error.
// The retry executor would stop on the done ctx anyway; checking here keeps such a failure from
// being logged as a retry.
func isRetryableOIDCError(ctx context.Context, err error) bool {
	if ctx.Err() != nil {
		return false
	}
	return errors.Is(err, errUtils.ErrGitHubOIDCEndpointUnavailable)
}

// fetchOIDCTokenOnce makes one request to GitHub's OIDC endpoint. Transport errors (including the
// per-attempt timeout, whether it fires before the headers or while the body is read), 429 and
// 5xx are wrapped with ErrGitHubOIDCEndpointUnavailable so the caller can retry them.
func fetchOIDCTokenOnce(ctx context.Context, client *http.Client, requestURL, requestToken, audience string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
	if err != nil {
		return "", fmt.Errorf("%w: create OIDC request: %w", errUtils.ErrAuthenticationFailed, err)
	}
	q := req.URL.Query()
	if audience != "" {
		q.Set("audience", audience)
	}
	req.URL.RawQuery = q.Encode()
	req.Header.Set("Authorization", "bearer "+requestToken)
	req.Header.Set("Accept", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("%w: %w: call OIDC endpoint: %w", errUtils.ErrAuthenticationFailed, errUtils.ErrGitHubOIDCEndpointUnavailable, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		// Drain a bounded amount so the shared client can reuse the connection.
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, oidcMaxDrainBytes))
		if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= http.StatusInternalServerError {
			return "", fmt.Errorf("%w: %w: OIDC endpoint returned status %s", errUtils.ErrAuthenticationFailed, errUtils.ErrGitHubOIDCEndpointUnavailable, resp.Status)
		}
		return "", fmt.Errorf("%w: OIDC endpoint returned status %s", errUtils.ErrAuthenticationFailed, resp.Status)
	}
	return readOIDCTokenResponse(resp.Body)
}

// readOIDCTokenResponse extracts the token from a 200 response. It decodes from the stream and
// returns once the JSON object is complete, without waiting for the end of the body. A read that
// fails first (the per-attempt timeout firing mid-body, a dropped connection, a body cut short)
// is reported as retryable. A body that is not a token object, or whose object is larger than
// oidcMaxBodyBytes, is not.
func readOIDCTokenResponse(body io.Reader) (string, error) {
	limited := &countingReader{r: io.LimitReader(body, oidcMaxBodyBytes+1)}
	var out struct {
		Value string `json:"value"`
	}
	err := json.NewDecoder(limited).Decode(&out)
	if limited.n > oidcMaxBodyBytes {
		return "", fmt.Errorf("%w: OIDC response exceeds %d bytes", errUtils.ErrAuthenticationFailed, oidcMaxBodyBytes)
	}
	if err != nil {
		var syntaxErr *json.SyntaxError
		var typeErr *json.UnmarshalTypeError
		if errors.As(err, &syntaxErr) || errors.As(err, &typeErr) {
			return "", fmt.Errorf("%w: decode OIDC response: %w", errUtils.ErrAuthenticationFailed, err)
		}
		return "", fmt.Errorf("%w: %w: read OIDC response: %w", errUtils.ErrAuthenticationFailed, errUtils.ErrGitHubOIDCEndpointUnavailable, err)
	}
	if out.Value == "" {
		return "", fmt.Errorf("%w: empty token in OIDC response", errUtils.ErrAuthenticationFailed)
	}
	return out.Value, nil
}

// countingReader counts the bytes read through it.
type countingReader struct {
	r io.Reader
	n int64
}

func (c *countingReader) Read(b []byte) (int, error) {
	n, err := c.r.Read(b)
	c.n += int64(n)
	return n, err
}

// Validate validates the provider configuration.
func (p *oidcProvider) Validate() error {
	audience, err := p.audience()
	if err != nil {
		return err
	}
	if audience == "" {
		return fmt.Errorf("%w: audience is required in provider spec", errUtils.ErrInvalidProviderConfig)
	}
	return nil
}

// Environment returns environment variables for this provider.
func (p *oidcProvider) Environment() (map[string]string, error) {
	// GitHub OIDC provider doesn't set additional environment variables.
	// The OIDC token is passed to downstream identities via credentials.
	return map[string]string{}, nil
}

// Paths returns credential files/directories used by this provider.
func (p *oidcProvider) Paths() ([]types.Path, error) {
	// GitHub OIDC provider doesn't use filesystem credentials.
	// The OIDC token comes from GitHub Actions environment.
	return []types.Path{}, nil
}

// PrepareEnvironment prepares environment variables for external processes.
// For GitHub OIDC providers, we don't modify the environment since OIDC tokens
// are used for authentication chains but not directly consumed by external processes.
func (p *oidcProvider) PrepareEnvironment(_ context.Context, environ map[string]string) (map[string]string, error) {
	defer perf.Track(nil, "github.oidcProvider.PrepareEnvironment")()

	// GitHub OIDC provider doesn't need to modify environment for external processes.
	// The OIDC token is used during authentication to obtain cloud credentials,
	// which are then managed by the respective cloud identities.
	return environ, nil
}

// Logout removes provider-specific credential storage.
func (p *oidcProvider) Logout(ctx context.Context) error {
	// GitHub OIDC provider has no logout concept - tokens come from GitHub Actions environment.
	// Nothing is persisted for this provider: it is ambient (see IsAmbient), so the auth
	// manager never writes its credentials to the keyring and purges any entry left behind
	// by an older Atmos version.
	// Return ErrLogoutNotSupported to indicate successful no-op (exit 0).
	log.Debug("Logout not supported for GitHub OIDC provider (no files to clean up)", "provider", p.name)
	return errUtils.ErrLogoutNotSupported
}

// GetFilesDisplayPath returns the display path for credential files.
// GitHub OIDC provider doesn't use file-based credentials.
func (p *oidcProvider) GetFilesDisplayPath() string {
	return "" // No files for GitHub OIDC provider
}
