// Package testkit provides local protocol fixtures for plugin conformance tests.
package testkit

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"time"

	"github.com/cloudposse/atmos/pkg/perf"
)

const fixtureClientIDField = "client_id"

const fixtureRSAKeyBits = 2048

// Issuer serves discovery, signed ID tokens, PKCE exchange, refresh, and revocation.
// It accepts no developer credentials and binds only to a local test listener.
type Issuer struct {
	Server                            *httptest.Server
	mu                                sync.Mutex
	key                               *rsa.PrivateKey
	codes                             map[string]authorization
	refresh                           map[string]authorization
	mutateClaims                      func(map[string]any)
	corruptSignature                  bool
	exchanges, refreshes, revocations int
}

type authorization struct{ nonce, challenge, redirect, client string }

// IssuerOption configures a deliberate protocol failure in a local test fixture.
type IssuerOption func(*Issuer)

// WithClaims changes signed claims to exercise relying-party validation.
//
//nolint:lintroller // Trivial accessor, pure lookup, factory option, or delegation; exempt under repository tracking policy.
func WithClaims(mutate func(map[string]any)) IssuerOption {
	return func(issuer *Issuer) { issuer.mutateClaims = mutate }
}

// WithInvalidSignature signs with a key absent from the discovery JWKS.
//
//nolint:lintroller // Trivial accessor, pure lookup, factory option, or delegation; exempt under repository tracking policy.
func WithInvalidSignature() IssuerOption {
	return func(issuer *Issuer) { issuer.corruptSignature = true }
}

// NewIssuer starts an isolated OIDC fixture.
func NewIssuer(options ...IssuerOption) (*Issuer, error) {
	defer perf.Track(nil, "testkit.NewIssuer")()

	key, err := rsa.GenerateKey(rand.Reader, fixtureRSAKeyBits)
	if err != nil {
		return nil, err
	}
	issuer := &Issuer{key: key, codes: make(map[string]authorization), refresh: make(map[string]authorization)}
	for _, option := range options {
		option(issuer)
	}
	issuer.Server = httptest.NewServer(http.HandlerFunc(issuer.serve))
	return issuer, nil
}

// Counts returns code-exchange, refresh, and revocation request counts.
//
//nolint:lintroller // Trivial accessor, pure lookup, factory option, or delegation; exempt under repository tracking policy.
func (i *Issuer) Counts() (int, int, int) {
	i.mu.Lock()
	defer i.mu.Unlock()
	return i.exchanges, i.refreshes, i.revocations
}

// Close stops the issuer.
func (i *Issuer) Close() {
	defer perf.Track(nil, "testkit.Issuer.Close")()

	i.Server.Close()
}

func (i *Issuer) serve(w http.ResponseWriter, r *http.Request) {
	i.mu.Lock()
	defer i.mu.Unlock()
	switch r.URL.Path {
	case "/.well-known/openid-configuration":
		writeJSON(w, map[string]any{"issuer": i.Server.URL, "authorization_endpoint": i.Server.URL + "/authorize", "token_endpoint": i.Server.URL + "/token", "jwks_uri": i.Server.URL + "/keys", "revocation_endpoint": i.Server.URL + "/revoke", "id_token_signing_alg_values_supported": []string{"RS256"}})
	case "/keys":
		writeJSON(w, map[string]any{"keys": []any{map[string]any{"kty": "RSA", "kid": "test-key", "use": "sig", "alg": "RS256", "n": base64.RawURLEncoding.EncodeToString(i.key.N.Bytes()), "e": "AQAB"}}})
	case "/authorize":
		i.authorize(w, r)
	case "/token":
		i.token(w, r)
	case "/revoke":
		if err := r.ParseForm(); err != nil {
			http.Error(w, "invalid form", http.StatusBadRequest)
			return
		}
		i.revocations++
		delete(i.refresh, r.Form.Get("token"))
		w.WriteHeader(http.StatusOK)
	default:
		http.NotFound(w, r)
	}
}

func (i *Issuer) authorize(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	if query.Get("code_challenge_method") != "S256" || query.Get("nonce") == "" || query.Get("state") == "" || query.Get(fixtureClientIDField) == "" {
		http.Error(w, "PKCE, state, nonce and client required", http.StatusBadRequest)
		return
	}
	redirect, err := url.Parse(query.Get("redirect_uri"))
	if err != nil || redirect.Scheme != "http" || redirect.Hostname() != "127.0.0.1" || redirect.User != nil || redirect.Fragment != "" {
		http.Error(w, "loopback required", http.StatusBadRequest)
		return
	}
	code := randomToken()
	i.codes[code] = authorization{nonce: query.Get("nonce"), challenge: query.Get("code_challenge"), redirect: redirect.String(), client: query.Get(fixtureClientIDField)}
	values := redirect.Query()
	values.Set("code", code)
	values.Set("state", query.Get("state"))
	redirect.RawQuery = values.Encode()
	// The callback is an HTTP IPv4 loopback URL without userinfo or a fragment.
	//nolint:gosec // Deliberate local fixture redirect after loopback validation.
	http.Redirect(w, r, redirect.String(), http.StatusFound)
}

func (i *Issuer) token(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}
	var nonce, client string
	switch r.Form.Get("grant_type") {
	case "authorization_code":
		code := r.Form.Get("code")
		auth, exists := i.codes[code]
		delete(i.codes, code)
		challenge := sha256.Sum256([]byte(r.Form.Get("code_verifier")))
		if !exists || !matchesAuthorization(&auth, base64.RawURLEncoding.EncodeToString(challenge[:]), r) {
			i.invalidGrant(w)
			return
		}
		nonce, client = auth.nonce, auth.client
		i.exchanges++
	case "refresh_token":
		previous := r.Form.Get("refresh_token")
		previousAuth, exists := i.refresh[previous]
		client, nonce = previousAuth.client, previousAuth.nonce
		if !exists || client != r.Form.Get(fixtureClientIDField) {
			i.invalidGrant(w)
			return
		}
		delete(i.refresh, previous)
		i.refreshes++
	default:
		i.invalidGrant(w)
		return
	}
	i.issueTokens(w, client, nonce)
}

func (*Issuer) invalidGrant(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusBadRequest)
	_, _ = io.WriteString(w, `{"error":"invalid_grant"}`)
}

func (i *Issuer) sign(claims map[string]any) (string, error) {
	if i.mutateClaims != nil {
		i.mutateClaims(claims)
	}
	payload, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}
	encoded := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"RS256","kid":"test-key"}`)) + "." + base64.RawURLEncoding.EncodeToString(payload)
	digest := sha256.Sum256([]byte(encoded))
	key := i.key
	if i.corruptSignature {
		var err error
		key, err = rsa.GenerateKey(rand.Reader, fixtureRSAKeyBits)
		if err != nil {
			return "", err
		}
	}
	signature, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, digest[:])
	if err != nil {
		return "", err
	}
	return encoded + "." + base64.RawURLEncoding.EncodeToString(signature), nil
}

func writeJSON(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(value)
}

func randomToken() string {
	return rand.Text()
}

func (i *Issuer) issueTokens(w http.ResponseWriter, client, nonce string) {
	access, refresh := randomToken(), randomToken()
	i.refresh[refresh] = authorization{client: client, nonce: nonce}
	id, err := i.sign(map[string]any{"iss": i.Server.URL, "sub": "test-subject", "aud": client, "exp": time.Now().Add(time.Hour).Unix(), "iat": time.Now().Unix(), "nonce": nonce})
	if err != nil {
		http.Error(w, "signing failed", http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]any{"access_token": access, "refresh_token": refresh, "token_type": "Bearer", "expires_in": 3600, "id_token": id, "scope": "openid profile"})
}

func matchesAuthorization(auth *authorization, challenge string, r *http.Request) bool {
	return auth.challenge == challenge && auth.redirect == r.Form.Get("redirect_uri") && auth.client == r.Form.Get(fixtureClientIDField)
}
