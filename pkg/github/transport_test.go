package github

import (
	"crypto/tls"
	"crypto/x509"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// capturingRoundTripper is a stand-in "base" transport that records the request it received
// (in particular, the Authorization header scopedTokenTransport left on it) and returns a
// canned 200 response without making any real network call. Used where the scenario under
// test only needs to observe what scopedTokenTransport hands to the underlying transport
// (e.g. a same-host scheme downgrade, which cannot be reproduced with two real listeners
// sharing one TCP port).
type capturingRoundTripper struct {
	gotAuth string
	gotURL  string
}

func (c *capturingRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	c.gotAuth = req.Header.Get("Authorization")
	c.gotURL = req.URL.String()
	return &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader("")),
		Header:     make(http.Header),
	}, nil
}

// TestScopedTokenTransport_SameHostHTTPS pins that a request whose own URL is https and whose
// host matches the endpoint's server host carries the token, exercised through a real TLS
// server (not a stub) so the full RoundTrip path -- including the actual network call -- is
// covered.
func TestScopedTokenTransport_SameHostHTTPS(t *testing.T) {
	var gotAuth string
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	endpoints := Endpoints{ServerURL: server.URL, APIURL: server.URL, Host: hostOf(server.URL)}
	client := &http.Client{
		Transport: &scopedTokenTransport{
			base:    server.Client().Transport,
			token:   "secret-token",
			allowed: endpoints,
		},
	}

	resp, err := client.Get(server.URL + "/repos/owner/repo")
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, "Bearer secret-token", gotAuth)
}

// TestRequestAllowsToken_SchemeAwarePortHandling pins CodeRabbit thread PRRT_kwDOEW4XoM6h7p3L:
// normalizeHost strips both port 80 and port 443 unconditionally, so a request explicitly
// targeting "https://host:80" would normalize identically to the bare configured host (whose
// real default port for https is 443, not 80) and could wrongly receive a token scoped to that
// host. The function under test must instead use the scheme-aware IsHostForScheme/
// IsAPIHostForScheme/IsUploadHostForScheme, which only strip a port when it is the actual
// default for the request's own scheme.
func TestRequestAllowsToken_SchemeAwarePortHandling(t *testing.T) {
	allowed := Endpoints{Host: "host", APIURL: "https://host/api/v3", UploadURL: "https://host/api/uploads"}

	tests := []struct {
		name string
		url  string
		want bool
	}{
		{name: "https on the mismatched default http port is rejected", url: "https://host:80/repos/o/r", want: false},
		{name: "https on its own default port (implicit) is allowed", url: "https://host/repos/o/r", want: true},
		{name: "https explicitly on port 443 is allowed", url: "https://host:443/repos/o/r", want: true},
		{name: "plain http on its own default port is rejected (never https)", url: "http://host:80/repos/o/r", want: false},
		{name: "explicit non-default port must match exactly (mismatch)", url: "https://host:8443/repos/o/r", want: false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req, err := http.NewRequest(http.MethodGet, tc.url, nil)
			require.NoError(t, err)
			assert.Equal(t, tc.want, requestAllowsToken(req, allowed))
		})
	}
}

// TestScopedTokenTransport_CrossHostRedirectStripsToken pins that a redirect from the allowed
// host to an unrelated https host -- both real TLS servers, so this exercises net/http's
// actual redirect-following, not just a single RoundTrip call -- never carries the token to
// the redirect target, even though the target is also https.
func TestScopedTokenTransport_CrossHostRedirectStripsToken(t *testing.T) {
	var gotAuthB string
	serverB := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuthB = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
	}))
	defer serverB.Close()

	var gotAuthA string
	serverA := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuthA = r.Header.Get("Authorization")
		http.Redirect(w, r, serverB.URL+"/other", http.StatusFound)
	}))
	defer serverA.Close()

	// Trust both self-signed certificates: net/http must actually complete the TLS handshake
	// against serverB to prove the Authorization header was (or wasn't) sent to it.
	certPool := x509.NewCertPool()
	certPool.AddCert(serverA.Certificate())
	certPool.AddCert(serverB.Certificate())
	base := &http.Transport{TLSClientConfig: &tls.Config{RootCAs: certPool}}

	endpoints := Endpoints{ServerURL: serverA.URL, APIURL: serverA.URL, Host: hostOf(serverA.URL)}
	client := &http.Client{
		Transport: &scopedTokenTransport{
			base:    base,
			token:   "secret-token",
			allowed: endpoints,
		},
	}

	resp, err := client.Get(serverA.URL + "/repos/owner/repo")
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, "Bearer secret-token", gotAuthA, "the allowed host must still receive the token")
	assert.Empty(t, gotAuthB, "a redirect to an unrelated host must never receive the token")
}

// TestScopedTokenTransport_SchemeDowngradeSameHostStripsToken pins that a request whose host
// matches the allowed endpoint but whose scheme is http (a same-host scheme downgrade, as a
// redirect from https could produce) never carries the token. A real listener cannot serve
// both http and https on one TCP port, so this drives scopedTokenTransport.RoundTrip directly
// with a capturing base transport rather than a live redirect chain -- RoundTrip's contract
// (re-validate the request actually handed to it) is exactly what's under test either way.
func TestScopedTokenTransport_SchemeDowngradeSameHostStripsToken(t *testing.T) {
	const host = "ghes.example.com:8443"
	base := &capturingRoundTripper{}
	transport := &scopedTokenTransport{
		base:    base,
		token:   "secret-token",
		allowed: Endpoints{ServerURL: "https://" + host, APIURL: "https://" + host, Host: host},
	}

	req, err := http.NewRequest(http.MethodGet, "http://"+host+"/repos/owner/repo", nil)
	require.NoError(t, err)

	resp, err := transport.RoundTrip(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Empty(t, base.gotAuth, "a same-host scheme downgrade to http must never carry the token")
}

// TestScopedTokenTransport_APIOnlyOverrideGetsToken pins that a request to the API-only
// override host (ATMOS_TOOLCHAIN_GITHUB_API_URL pointed at a different host than the web/clone
// URL) is recognized via IsAPIHost and receives the token, while a third, wholly unrelated
// https host does not.
func TestScopedTokenTransport_APIOnlyOverrideGetsToken(t *testing.T) {
	endpoints := Endpoints{
		ServerURL: "https://github.example.com",
		APIURL:    "https://api-mirror.example.com",
		Host:      "github.example.com",
	}
	base := &capturingRoundTripper{}
	transport := &scopedTokenTransport{base: base, token: "secret-token", allowed: endpoints}

	apiReq, err := http.NewRequest(http.MethodGet, "https://api-mirror.example.com/repos/owner/repo", nil)
	require.NoError(t, err)
	resp, err := transport.RoundTrip(apiReq)
	require.NoError(t, err)
	resp.Body.Close()
	assert.Equal(t, "Bearer secret-token", base.gotAuth, "the API-only override host must receive the token")

	unrelatedReq, err := http.NewRequest(http.MethodGet, "https://attacker.example.com/repos/owner/repo", nil)
	require.NoError(t, err)
	resp, err = transport.RoundTrip(unrelatedReq)
	require.NoError(t, err)
	resp.Body.Close()
	assert.Empty(t, base.gotAuth, "an unrelated third host must never receive the token")
}

// TestScopedTokenTransport_RemovesCallerSetHeader pins that a caller-set Authorization header
// is removed (not merely left unset) when the request is not allowed to carry a token, so a
// caller can never smuggle a token in ahead of scopedTokenTransport's own decision.
func TestScopedTokenTransport_RemovesCallerSetHeader(t *testing.T) {
	base := &capturingRoundTripper{}
	transport := &scopedTokenTransport{
		base:    base,
		token:   "",
		allowed: Endpoints{ServerURL: "https://github.example.com", APIURL: "https://github.example.com", Host: "github.example.com"},
	}

	req, err := http.NewRequest(http.MethodGet, "https://attacker.example.com/x", nil)
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer caller-supplied-token")

	resp, err := transport.RoundTrip(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Empty(t, base.gotAuth, "a caller-set Authorization header must be removed, not forwarded")
}

// TestIsApprovedGitHubDownloadHost pins that a download/redirect host is approved when it
// matches either RepoEndpoints' or ToolchainEndpoints' server, API, or upload host, and rejected
// otherwise -- e.g. the pre-signed S3-style blob host a GitHub artifact download redirects to.
func TestIsApprovedGitHubDownloadHost(t *testing.T) {
	clearGitHubEndpointEnv(t)
	t.Setenv("GITHUB_SERVER_URL", "https://ghes.example.com")
	t.Setenv("ATMOS_TOOLCHAIN_GITHUB_API_URL", "https://api-mirror.example.com")

	tests := []struct {
		name string
		host string
		want bool
	}{
		{name: "repo server host", host: "ghes.example.com", want: true},
		{name: "repo API host (derived /api/v3)", host: "ghes.example.com", want: true},
		{name: "toolchain default server host (public github.com)", host: "github.com", want: true},
		{name: "toolchain API-only override host", host: "api-mirror.example.com", want: true},
		{name: "toolchain default upload host", host: "uploads.github.com", want: true},
		{name: "case and port normalized", host: "GHES.Example.com:443", want: true},
		{name: "unrelated pre-signed blob host is rejected", host: "s3.amazonaws.com", want: false},
		{name: "unrelated host is rejected", host: "attacker.example.com", want: false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, IsApprovedGitHubDownloadHost(tc.host))
		})
	}
}

// TestNewScopedTokenHTTPClient_WithholdsTokenOverHTTP pins that NewScopedTokenHTTPClient never
// attaches a token when allowed's own API host is not https (TokenForEndpoints returns "").
func TestNewScopedTokenHTTPClient_WithholdsTokenOverHTTP(t *testing.T) {
	var gotAuth string
	var gotAuthSet bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth, gotAuthSet = r.Header.Get("Authorization"), r.Header.Get("Authorization") != ""
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	endpoints := Endpoints{ServerURL: server.URL, APIURL: server.URL, Host: hostOf(server.URL)}
	client := NewScopedTokenHTTPClient("leaked-token", endpoints, defaultHTTPTimeout)

	resp, err := client.Get(server.URL)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.False(t, gotAuthSet, "expected no Authorization header sent to a plain-http endpoint")
	assert.Empty(t, gotAuth)
}
