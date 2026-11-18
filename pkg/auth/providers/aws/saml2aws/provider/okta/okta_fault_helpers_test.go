package okta

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/auth/providers/aws/saml2aws/provider"
)

// errInjected is the failure produced by the fault-injecting transport below.
var errInjected = errors.New("injected failure")

// faultKind selects how the fault-injecting transport fails a request.
type faultKind int

const (
	// faultNone lets the request through to the handler.
	faultNone faultKind = iota
	// faultTransport fails the round trip itself, like a connection error.
	faultTransport
	// faultBody returns a 200 response whose body fails to read, like a connection reset mid-response.
	faultBody
)

// errBody is a response body that always fails to read.
type errBody struct{}

func (errBody) Read([]byte) (int, error) { return 0, errInjected }

func (errBody) Close() error { return nil }

// faultRoundTripper serves requests from an in-process handler and fails the ones selected by fault.
// It never touches the network, so tests can use arbitrary hostnames and loopback ports.
type faultRoundTripper struct {
	handler http.Handler
	fault   func(*http.Request) faultKind
}

func (f *faultRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	if f.fault != nil {
		switch f.fault(req) {
		case faultTransport:
			return nil, errInjected
		case faultBody:
			return &http.Response{
				StatusCode: http.StatusOK,
				Status:     "200 OK",
				Proto:      "HTTP/1.1",
				Header:     http.Header{},
				Body:       errBody{},
				Request:    req,
			}, nil
		case faultNone:
		}
	}

	rec := httptest.NewRecorder()
	f.handler.ServeHTTP(rec, req)
	res := rec.Result()
	res.Request = req
	return res, nil
}

// faultAt fails the nth request (1-based) to path with the given kind. An nth of 0 fails every request to path.
func faultAt(kind faultKind, path string, nth int) func(*http.Request) faultKind {
	var mu sync.Mutex
	seen := 0
	return func(req *http.Request) faultKind {
		if req.URL.Path != path {
			return faultNone
		}
		mu.Lock()
		defer mu.Unlock()
		seen++
		if nth == 0 || seen == nth {
			return kind
		}
		return faultNone
	}
}

// newFaultClient returns an Okta client whose HTTP traffic is served by handler, with fault applied on top.
// Like the production client from New, it rejects responses that are neither a success nor a redirect,
// so a request routed to an unknown path (404) fails instead of being treated as an empty page.
func newFaultClient(t *testing.T, handler http.Handler, fault func(*http.Request) faultKind) *Client {
	t.Helper()

	hc, err := provider.NewHTTPClient(&faultRoundTripper{handler: handler, fault: fault}, &provider.HTTPClientOptions{})
	require.NoError(t, err)
	hc.CheckResponseStatus = provider.SuccessOrRedirectResponseValidator
	return &Client{client: hc, mfa: "Auto", rememberDevice: true}
}

// routes is a path-keyed in-process handler. Each route receives the 1-based call number for its path
// and returns the response body; unknown paths answer 404.
type routes struct {
	mu     sync.Mutex
	counts map[string]int
	h      map[string]func(call int, r *http.Request) string
}

func newRoutes(h map[string]func(call int, r *http.Request) string) *routes {
	return &routes{counts: map[string]int{}, h: h}
}

func (rt *routes) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	rt.mu.Lock()
	rt.counts[r.URL.Path]++
	call := rt.counts[r.URL.Path]
	rt.mu.Unlock()

	f, ok := rt.h[r.URL.Path]
	if !ok {
		http.NotFound(w, r)
		return
	}
	_, _ = io.WriteString(w, f(call, r))
}

// calls returns how many requests the path received.
func (rt *routes) calls(path string) int {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	return rt.counts[path]
}

// static returns a route that always answers with body.
func static(body string) func(int, *http.Request) string {
	return func(int, *http.Request) string { return body }
}

// withDeviceFinder installs a U2F device finder for the duration of a test.
func withDeviceFinder(t *testing.T, finder DeviceFinder) {
	t.Helper()

	orig := newU2FDeviceFinder
	newU2FDeviceFinder = func() DeviceFinder { return finder }
	t.Cleanup(func() { newU2FDeviceFinder = orig })
}

// badURL is a URL that url.Parse and http.NewRequest reject.
const badURL = "http://[::1"

// duoChallengeBody builds the Okta factor-verify response that starts a Duo web challenge.
func duoChallengeBody(host, signature, completeHref string) string {
	return fmt.Sprintf(`{"status":"MFA_CHALLENGE","_embedded":{"factor":{"_embedded":{"verification":{`+
		`"host":"%s","signature":"%s","_links":{"complete":{"href":"%s"}}}}}}}`, host, signature, completeHref)
}
