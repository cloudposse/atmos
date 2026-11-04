package github

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth/types"
)

const (
	testRequestToken = "test-request-token"
	testAudience     = "sts.amazonaws.com"
)

// fakeClock satisfies retry.Clock. Backoff waits fire at once (or never, when blockAfter is set)
// and are recorded, so tests neither sleep nor depend on wall-clock time.
type fakeClock struct {
	mu         sync.Mutex
	now        time.Time
	delays     []time.Duration
	blockAfter bool
	onAfter    func()
}

func newFakeClock() *fakeClock {
	return &fakeClock{now: time.Date(2026, 11, 4, 0, 0, 0, 0, time.UTC)}
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *fakeClock) After(d time.Duration) <-chan time.Time {
	c.mu.Lock()
	c.delays = append(c.delays, d)
	block, hook, now := c.blockAfter, c.onAfter, c.now
	c.mu.Unlock()

	if hook != nil {
		hook()
	}
	ch := make(chan time.Time, 1)
	if !block {
		ch <- now
	}
	return ch
}

func (c *fakeClock) advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

func (c *fakeClock) recordedDelays() []time.Duration {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]time.Duration(nil), c.delays...)
}

// makeJWT builds an unsigned JWT whose payload carries the given exp claim. The provider never
// verifies the signature; it only reads exp to decide whether the token can be reused.
func makeJWT(t *testing.T, exp time.Time, subject string) string {
	t.Helper()
	enc := base64.RawURLEncoding
	header := enc.EncodeToString([]byte(`{"alg":"RS256","typ":"JWT"}`))
	payload := enc.EncodeToString([]byte(fmt.Sprintf(`{"sub":%q,"exp":%d}`, subject, exp.Unix())))
	return header + "." + payload + ".sig"
}

// tokenServer is an httptest TLS server that answers the Nth request with handlers[N], repeating
// the last handler once the list runs out, and counts requests.
type tokenServer struct {
	*httptest.Server
	requests atomic.Int32
}

func newTokenServer(t *testing.T, handlers ...http.HandlerFunc) *tokenServer {
	t.Helper()
	require.NotEmpty(t, handlers, "tokenServer needs at least one handler")
	ts := &tokenServer{}
	ts.Server = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := int(ts.requests.Add(1)) - 1
		if n >= len(handlers) {
			n = len(handlers) - 1
		}
		handlers[n](w, r)
	}))
	t.Cleanup(ts.Close)
	return ts
}

func respondToken(token string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprintf(w, `{"value":%q}`, token)
	}
}

func respondStatus(code int) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, http.StatusText(code), code)
	}
}

func respondBody(body string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(body))
	}
}

// respondHang blocks until the client gives up on the request.
func respondHang() http.HandlerFunc {
	return func(_ http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}
}

// newTestOIDCProvider returns a provider that trusts ts and times out each attempt after attemptTimeout.
func newTestOIDCProvider(ts *tokenServer, clock *fakeClock, attemptTimeout time.Duration) *oidcProvider {
	// Copy the server's client: it is shared, and callers may run concurrently.
	client := *ts.Client()
	client.Timeout = attemptTimeout
	return &oidcProvider{name: "github-oidc", config: validOidcSpec(), httpClient: &client, clock: clock}
}

// isolateJWTCache empties the process-wide JWT cache before and after the test.
func isolateJWTCache(t *testing.T) {
	t.Helper()
	processJWTCache.reset()
	t.Cleanup(processJWTCache.reset)
}

func TestOIDCProvider_getOIDCToken_Retry(t *testing.T) {
	clock := newFakeClock()
	validJWT := makeJWT(t, clock.Now().Add(5*time.Minute), "repo:acme/infra")

	tests := []struct {
		name           string
		handlers       []http.HandlerFunc
		attemptTimeout time.Duration
		wantToken      string
		wantRequests   int32
		wantErrIs      []error
		wantErrNotIs   []error
		wantErrContain string
	}{
		{
			name:         "success on first attempt",
			handlers:     []http.HandlerFunc{respondToken(validJWT)},
			wantToken:    validJWT,
			wantRequests: 1,
		},
		{
			name:         "503 twice then success",
			handlers:     []http.HandlerFunc{respondStatus(503), respondStatus(503), respondToken(validJWT)},
			wantToken:    validJWT,
			wantRequests: 3,
		},
		{
			name:         "429 then success",
			handlers:     []http.HandlerFunc{respondStatus(429), respondToken(validJWT)},
			wantToken:    validJWT,
			wantRequests: 2,
		},
		{
			name:           "hang times out then success",
			handlers:       []http.HandlerFunc{respondHang(), respondToken(validJWT)},
			attemptTimeout: 100 * time.Millisecond,
			wantToken:      validJWT,
			wantRequests:   2,
		},
		{
			name:           "retries exhausted on 502",
			handlers:       []http.HandlerFunc{respondStatus(502)},
			wantRequests:   oidcMaxAttempts,
			wantErrIs:      []error{errUtils.ErrAuthenticationFailed, errUtils.ErrGitHubOIDCEndpointUnavailable},
			wantErrContain: fmt.Sprintf("max attempts (%d) exceeded", oidcMaxAttempts),
		},
		{
			name:           "retries exhausted on timeouts",
			handlers:       []http.HandlerFunc{respondHang()},
			attemptTimeout: 50 * time.Millisecond,
			wantRequests:   oidcMaxAttempts,
			wantErrIs:      []error{errUtils.ErrAuthenticationFailed, errUtils.ErrGitHubOIDCEndpointUnavailable},
			wantErrContain: "Client.Timeout exceeded",
		},
		{
			name:           "403 is not retried",
			handlers:       []http.HandlerFunc{respondStatus(403), respondToken(validJWT)},
			wantRequests:   1,
			wantErrIs:      []error{errUtils.ErrAuthenticationFailed},
			wantErrNotIs:   []error{errUtils.ErrGitHubOIDCEndpointUnavailable},
			wantErrContain: "403",
		},
		{
			name:         "404 is not retried",
			handlers:     []http.HandlerFunc{respondStatus(404), respondToken(validJWT)},
			wantRequests: 1,
			wantErrIs:    []error{errUtils.ErrAuthenticationFailed},
			wantErrNotIs: []error{errUtils.ErrGitHubOIDCEndpointUnavailable},
		},
		{
			name:           "malformed JSON is not retried",
			handlers:       []http.HandlerFunc{respondBody("not json"), respondToken(validJWT)},
			wantRequests:   1,
			wantErrIs:      []error{errUtils.ErrAuthenticationFailed},
			wantErrNotIs:   []error{errUtils.ErrGitHubOIDCEndpointUnavailable},
			wantErrContain: "decode OIDC response",
		},
		{
			name:           "empty token is not retried",
			handlers:       []http.HandlerFunc{respondBody(`{"value":""}`), respondToken(validJWT)},
			wantRequests:   1,
			wantErrIs:      []error{errUtils.ErrAuthenticationFailed},
			wantErrContain: "empty token",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			isolateJWTCache(t)
			clock := newFakeClock()
			attemptTimeout := tt.attemptTimeout
			if attemptTimeout == 0 {
				attemptTimeout = 5 * time.Second
			}
			ts := newTokenServer(t, tt.handlers...)
			p := newTestOIDCProvider(ts, clock, attemptTimeout)

			token, err := p.getOIDCToken(context.Background(), ts.URL, testRequestToken, testAudience)

			assert.Equal(t, tt.wantRequests, ts.requests.Load(), "token endpoint request count")
			// One backoff wait between consecutive attempts, growing exponentially within the jitter band.
			delays := clock.recordedDelays()
			require.Len(t, delays, int(tt.wantRequests)-1)
			for i, d := range delays {
				base := oidcInitialBackoff << i
				assert.GreaterOrEqual(t, d, time.Duration(float64(base)*(1-oidcBackoffJitter)), "delay %d", i)
				assert.LessOrEqual(t, d, time.Duration(float64(base)*(1+oidcBackoffJitter)), "delay %d", i)
			}

			if len(tt.wantErrIs) == 0 {
				require.NoError(t, err)
				assert.Equal(t, tt.wantToken, token)
				return
			}
			require.Error(t, err)
			assert.Empty(t, token)
			for _, target := range tt.wantErrIs {
				assert.ErrorIs(t, err, target)
			}
			for _, target := range tt.wantErrNotIs {
				assert.NotErrorIs(t, err, target)
			}
			if tt.wantErrContain != "" {
				assert.Contains(t, err.Error(), tt.wantErrContain)
			}
		})
	}
}

func TestOIDCProvider_getOIDCToken_SendsAudienceAndBearer(t *testing.T) {
	isolateJWTCache(t)
	clock := newFakeClock()
	jwt := makeJWT(t, clock.Now().Add(5*time.Minute), "s")

	var gotAuth, gotAudience string
	ts := newTokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotAudience = r.URL.Query().Get("audience")
		respondToken(jwt)(w, r)
	})
	p := newTestOIDCProvider(ts, clock, 5*time.Second)

	_, err := p.getOIDCToken(context.Background(), ts.URL, testRequestToken, testAudience)
	require.NoError(t, err)
	assert.Equal(t, "bearer "+testRequestToken, gotAuth)
	assert.Equal(t, testAudience, gotAudience)
}

func TestOIDCProvider_getOIDCToken_ContextCancellation(t *testing.T) {
	t.Run("already cancelled context makes no request", func(t *testing.T) {
		isolateJWTCache(t)
		ts := newTokenServer(t, respondToken("unused"))
		p := newTestOIDCProvider(ts, newFakeClock(), 5*time.Second)

		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		_, err := p.getOIDCToken(ctx, ts.URL, testRequestToken, testAudience)

		require.ErrorIs(t, err, context.Canceled)
		assert.Zero(t, ts.requests.Load())
	})

	t.Run("cancel during backoff stops retrying", func(t *testing.T) {
		isolateJWTCache(t)
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		clock := newFakeClock()
		clock.blockAfter = true
		clock.onAfter = cancel
		ts := newTokenServer(t, respondStatus(503))
		p := newTestOIDCProvider(ts, clock, 5*time.Second)

		_, err := p.getOIDCToken(ctx, ts.URL, testRequestToken, testAudience)

		require.ErrorIs(t, err, context.Canceled)
		assert.Equal(t, int32(1), ts.requests.Load())
	})

	t.Run("cancel during a hanging request is not retried", func(t *testing.T) {
		isolateJWTCache(t)
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		arrived := make(chan struct{})
		ts := newTokenServer(t, func(w http.ResponseWriter, r *http.Request) {
			close(arrived)
			respondHang()(w, r)
		})
		p := newTestOIDCProvider(ts, newFakeClock(), 30*time.Second)
		go func() {
			<-arrived
			cancel()
		}()

		start := time.Now()
		_, err := p.getOIDCToken(ctx, ts.URL, testRequestToken, testAudience)

		require.ErrorIs(t, err, context.Canceled)
		assert.Equal(t, int32(1), ts.requests.Load())
		assert.Less(t, time.Since(start), 10*time.Second, "cancellation must not wait for the attempt timeout")
	})
}

func TestOIDCProvider_getOIDCToken_Cache(t *testing.T) {
	const lifetime = 5 * time.Minute

	type call struct {
		advance      time.Duration // Moves the clock forward before the call.
		requestToken string
		audience     string
	}
	tests := []struct {
		name         string
		calls        []call
		tokenExp     time.Duration // Lifetime of each issued token; zero issues tokens without exp.
		wantRequests int32
	}{
		{
			name:         "second call reuses the token",
			calls:        []call{{}, {}},
			tokenExp:     lifetime,
			wantRequests: 1,
		},
		{
			name:         "reused while more than the margin remains",
			calls:        []call{{}, {advance: lifetime - oidcTokenMinRemaining - time.Second}},
			tokenExp:     lifetime,
			wantRequests: 1,
		},
		{
			name:         "refetched once only the margin remains",
			calls:        []call{{}, {advance: lifetime - oidcTokenMinRemaining}},
			tokenExp:     lifetime,
			wantRequests: 2,
		},
		{
			name:         "refetched after expiry",
			calls:        []call{{}, {advance: lifetime + time.Minute}},
			tokenExp:     lifetime,
			wantRequests: 2,
		},
		{
			name:         "different audience is a different entry",
			calls:        []call{{}, {audience: "api://AzureADTokenExchange"}, {}, {audience: "api://AzureADTokenExchange"}},
			tokenExp:     lifetime,
			wantRequests: 2,
		},
		{
			name:         "different request token is a different entry",
			calls:        []call{{}, {requestToken: "other-request-token"}},
			tokenExp:     lifetime,
			wantRequests: 2,
		},
		{
			name:         "token without exp is never cached",
			calls:        []call{{}, {}},
			wantRequests: 2,
		},
		{
			name:         "token issued inside the margin is never cached",
			calls:        []call{{}, {}},
			tokenExp:     oidcTokenMinRemaining - time.Second,
			wantRequests: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			isolateJWTCache(t)
			clock := newFakeClock()
			ts := newTokenServer(t, func(w http.ResponseWriter, r *http.Request) {
				token := "header.payload-without-exp"
				if tt.tokenExp > 0 {
					token = makeJWT(t, clock.Now().Add(tt.tokenExp), r.URL.Query().Get("audience"))
				}
				respondToken(token)(w, r)
			})
			p := newTestOIDCProvider(ts, clock, 5*time.Second)

			for i, c := range tt.calls {
				clock.advance(c.advance)
				requestToken, audience := testRequestToken, testAudience
				if c.requestToken != "" {
					requestToken = c.requestToken
				}
				if c.audience != "" {
					audience = c.audience
				}
				token, err := p.getOIDCToken(context.Background(), ts.URL, requestToken, audience)
				require.NoError(t, err, "call %d", i)
				require.NotEmpty(t, token, "call %d", i)
			}
			assert.Equal(t, tt.wantRequests, ts.requests.Load(), "token endpoint request count")
		})
	}
}

func TestOIDCProvider_getOIDCToken_ErrorIsNotCached(t *testing.T) {
	isolateJWTCache(t)
	clock := newFakeClock()
	jwt := makeJWT(t, clock.Now().Add(5*time.Minute), "s")
	ts := newTokenServer(t, respondStatus(401), respondToken(jwt))
	p := newTestOIDCProvider(ts, clock, 5*time.Second)

	_, err := p.getOIDCToken(context.Background(), ts.URL, testRequestToken, testAudience)
	require.ErrorIs(t, err, errUtils.ErrAuthenticationFailed)

	token, err := p.getOIDCToken(context.Background(), ts.URL, testRequestToken, testAudience)
	require.NoError(t, err)
	assert.Equal(t, jwt, token)
	assert.Equal(t, int32(2), ts.requests.Load())
}

func TestOIDCProvider_getOIDCToken_ConcurrentCallersShareOneFetch(t *testing.T) {
	isolateJWTCache(t)
	clock := newFakeClock()
	jwt := makeJWT(t, clock.Now().Add(5*time.Minute), "s")

	const callers = 16
	var ready sync.WaitGroup
	ready.Add(callers)
	ts := newTokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		// Hold the response until every caller has started, so they overlap the in-flight fetch.
		ready.Wait()
		time.Sleep(50 * time.Millisecond)
		respondToken(jwt)(w, r)
	})

	tokens := make([]string, callers)
	errs := make([]error, callers)
	var done sync.WaitGroup
	for i := range callers {
		done.Add(1)
		go func() {
			defer done.Done()
			// A separate provider per caller mirrors several identities rooted at one provider config.
			p := newTestOIDCProvider(ts, clock, 5*time.Second)
			ready.Done()
			tokens[i], errs[i] = p.getOIDCToken(context.Background(), ts.URL, testRequestToken, testAudience)
		}()
	}
	done.Wait()

	assert.Equal(t, int32(1), ts.requests.Load(), "token endpoint request count")
	for i := range callers {
		require.NoError(t, errs[i], "caller %d", i)
		assert.Equal(t, jwt, tokens[i], "caller %d", i)
	}
}

// TestOIDCProvider_Authenticate_ReusesTokenAcrossProviders covers the case that motivated the
// cache: one Atmos command authenticating several identities rooted at the same GitHub OIDC
// provider config must call the token endpoint once, not once per identity.
func TestOIDCProvider_Authenticate_ReusesTokenAcrossProviders(t *testing.T) {
	isolateJWTCache(t)
	jwt := makeJWT(t, time.Now().Add(5*time.Minute), "s")
	ts := newTokenServer(t, respondToken(jwt))
	t.Setenv("GITHUB_ACTIONS", "true")
	t.Setenv("ACTIONS_ID_TOKEN", "")
	t.Setenv("ACTIONS_ID_TOKEN_REQUEST_TOKEN", testRequestToken)
	t.Setenv("ACTIONS_ID_TOKEN_REQUEST_URL", ts.URL)

	for i := range 4 {
		provider, err := NewOIDCProvider("github-oidc", validOidcSpec())
		require.NoError(t, err)
		p, ok := provider.(*oidcProvider)
		require.True(t, ok)
		p.httpClient = ts.Client()

		creds, err := provider.Authenticate(context.Background())
		require.NoError(t, err, "identity %d", i)
		oidcCreds, ok := creds.(*types.OIDCCredentials)
		require.True(t, ok)
		assert.Equal(t, jwt, oidcCreds.Token)
	}
	assert.Equal(t, int32(1), ts.requests.Load(), "token endpoint request count")
}

// TestOIDCProvider_Authenticate_PresetIDTokenSkipsRequest pins the ACTIONS_ID_TOKEN short-circuit:
// a token supplied by the caller is used as is and the endpoint is never called.
func TestOIDCProvider_Authenticate_PresetIDTokenSkipsRequest(t *testing.T) {
	isolateJWTCache(t)
	ts := newTokenServer(t, respondStatus(500))
	t.Setenv("GITHUB_ACTIONS", "true")
	t.Setenv("ACTIONS_ID_TOKEN", "preset-jwt")
	t.Setenv("ACTIONS_ID_TOKEN_REQUEST_TOKEN", testRequestToken)
	t.Setenv("ACTIONS_ID_TOKEN_REQUEST_URL", ts.URL)

	provider, err := NewOIDCProvider("github-oidc", validOidcSpec())
	require.NoError(t, err)
	p, ok := provider.(*oidcProvider)
	require.True(t, ok)
	p.httpClient = ts.Client()

	creds, err := provider.Authenticate(context.Background())
	require.NoError(t, err)
	oidcCreds, ok := creds.(*types.OIDCCredentials)
	require.True(t, ok)
	assert.Equal(t, "preset-jwt", oidcCreds.Token)
	assert.Zero(t, ts.requests.Load())
}
