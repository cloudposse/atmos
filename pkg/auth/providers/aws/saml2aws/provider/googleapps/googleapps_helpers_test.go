package googleapps

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/PuerkitoBio/goquery"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/auth/providers/aws/saml2aws/cfg"
	"github.com/cloudposse/atmos/pkg/auth/providers/aws/saml2aws/creds"
	"github.com/cloudposse/atmos/pkg/auth/providers/aws/saml2aws/mocks"
	"github.com/cloudposse/atmos/pkg/auth/providers/aws/saml2aws/prompter"
)

const (
	testSAMLResponse = "PHNhbWw6QXNzZXJ0aW9uPmZha2U8L3NhbWw6QXNzZXJ0aW9uPg=="
	samlPage         = `<html><body><form method="POST" action="https://signin.aws.amazon.com/saml">` +
		`<input type="hidden" name="SAMLResponse" value="` + testSAMLResponse + `"></form></body></html>`
	pathFirstPage = "/o/saml2/initsso"
	pathLookup    = "/signin/v1/lookup"
	firstPage     = `<html><body><form id="gaia_loginform" method="POST" action="` + pathLookup + `">` +
		`<input type="hidden" name="GALX" value="galx-token"></form></body></html>`
	twoStepHeader = `<h1>2-Step Verification</h1>`
)

// pageHandler returns the HTML for a request given the 1-based call number for its path and the posted form.
type pageHandler func(call int, form url.Values) string

// fakeGoogle is a fake Google sign-in server serving per-path page handlers.
type fakeGoogle struct {
	t      *testing.T
	ts     *httptest.Server
	routes map[string]pageHandler

	mu    sync.Mutex
	calls map[string]int
	forms map[string][]url.Values
	jsons map[string][]map[string]string
}

func newFakeGoogle(t *testing.T, routes map[string]pageHandler) *fakeGoogle {
	t.Helper()

	f := &fakeGoogle{
		t:      t,
		routes: routes,
		calls:  map[string]int{},
		forms:  map[string][]url.Values{},
		jsons:  map[string][]map[string]string{},
	}
	f.ts = httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(f.ts.Close)
	return f
}

func (f *fakeGoogle) serve(w http.ResponseWriter, r *http.Request) {
	var jsonBody map[string]string
	if r.Header.Get("Content-Type") == "application/json" {
		assert.NoError(f.t, json.NewDecoder(r.Body).Decode(&jsonBody))
	}
	assert.NoError(f.t, r.ParseForm())

	f.mu.Lock()
	if jsonBody != nil {
		f.jsons[r.URL.Path] = append(f.jsons[r.URL.Path], jsonBody)
	}
	f.calls[r.URL.Path]++
	call := f.calls[r.URL.Path]
	f.forms[r.URL.Path] = append(f.forms[r.URL.Path], r.PostForm)
	h, ok := f.routes[r.URL.Path]
	f.mu.Unlock()

	if !ok {
		f.t.Errorf("unexpected request %s %s", r.Method, r.URL.String())
		http.NotFound(w, r)
		return
	}
	_, _ = fmt.Fprint(w, h(call, r.PostForm))
}

func (f *fakeGoogle) formsFor(path string) []url.Values {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.forms[path]
}

func (f *fakeGoogle) jsonFor(path string) []map[string]string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.jsons[path]
}

func (f *fakeGoogle) count(path string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls[path]
}

// client builds a Google Apps client through New, so the production HTTP client wiring is exercised.
func (f *fakeGoogle) client() (*Client, *creds.LoginDetails) {
	f.t.Helper()

	idpAccount := cfg.NewIDPAccount()
	idpAccount.URL = f.ts.URL + pathFirstPage + "?idpid=abc&spid=def"
	kc, err := New(idpAccount)
	require.NoError(f.t, err)
	return kc, &creds.LoginDetails{URL: idpAccount.URL, Username: "user@example.com", Password: "s3cret"}
}

// static returns a handler that always serves the same page.
func static(page string) pageHandler {
	return func(int, url.Values) string { return page }
}

// fixture loads an HTML fixture from the example directory.
func fixture(t *testing.T, name string) string {
	t.Helper()

	data, err := os.ReadFile(filepath.Join("example", name))
	require.NoError(t, err)
	return string(data)
}

// withMockPrompter installs a mock prompter and restores the previous one when the test ends.
// A mock without expectations panics on any call, which makes unexpected prompts fail the test.
func withMockPrompter(t *testing.T) *mocks.Prompter {
	t.Helper()

	orig := prompter.ActivePrompter
	pr := &mocks.Prompter{}
	prompter.SetPrompter(pr)
	t.Cleanup(func() {
		prompter.SetPrompter(orig)
		pr.AssertExpectations(t)
	})
	return pr
}

// withStdin replaces os.Stdin with a pipe containing the given input.
func withStdin(t *testing.T, input string) {
	t.Helper()

	r, w, err := os.Pipe()
	require.NoError(t, err)
	_, err = w.WriteString(input)
	require.NoError(t, err)
	require.NoError(t, w.Close())

	orig := os.Stdin
	os.Stdin = r
	t.Cleanup(func() {
		os.Stdin = orig
		_ = r.Close()
	})
}

// readPipe drains r in the background and returns a channel that yields everything read.
func readPipe(r *os.File) <-chan string {
	done := make(chan string, 1)
	go func() {
		var sb strings.Builder
		buf := make([]byte, 4096)
		for {
			n, readErr := r.Read(buf)
			sb.Write(buf[:n])
			if readErr != nil {
				break
			}
		}
		done <- sb.String()
	}()
	return done
}

// captureOutput redirects os.Stdout and os.Stderr while fn runs and returns what was written to each.
func captureOutput(t *testing.T, fn func()) (string, string) {
	t.Helper()

	outR, outW, err := os.Pipe()
	require.NoError(t, err)
	errR, errW, err := os.Pipe()
	require.NoError(t, err)
	origOut, origErr := os.Stdout, os.Stderr
	os.Stdout, os.Stderr = outW, errW
	outDone, errDone := readPipe(outR), readPipe(errR)

	func() {
		defer func() { os.Stdout, os.Stderr = origOut, origErr }()
		fn()
	}()
	require.NoError(t, outW.Close())
	require.NoError(t, errW.Close())
	stdout, stderr := <-outDone, <-errDone
	_ = outR.Close()
	_ = errR.Close()
	return stdout, stderr
}

// rewriteTransport redirects requests for one host to the target server and passes everything else through.
type rewriteTransport struct {
	host   string
	target *url.URL
	mu     sync.Mutex
	hits   int
}

func (rt *rewriteTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.URL.Host != rt.host {
		return http.DefaultTransport.RoundTrip(req)
	}

	rt.mu.Lock()
	rt.hits++
	rt.mu.Unlock()

	clone := req.Clone(req.Context())
	clone.URL.Scheme = rt.target.Scheme
	clone.URL.Host = rt.target.Host
	return http.DefaultTransport.RoundTrip(clone)
}

func (rt *rewriteTransport) hitCount() int {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	return rt.hits
}

// mustDoc parses an HTML string into a goquery document with the given URL.
func mustDoc(t *testing.T, html, rawURL string) *goquery.Document {
	t.Helper()

	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	require.NoError(t, err)
	doc.Url, err = url.Parse(rawURL)
	require.NoError(t, err)
	return doc
}

// closedServerURL returns the URL of a server that is no longer listening.
func closedServerURL(t *testing.T) string {
	t.Helper()

	ts := httptest.NewServer(http.NotFoundHandler())
	u := ts.URL
	ts.Close()
	return u
}
