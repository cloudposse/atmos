package okta

import (
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/cloudposse/atmos/pkg/auth/providers/aws/saml2aws/creds"
	"github.com/cloudposse/atmos/pkg/auth/providers/aws/saml2aws/helper/credentials"
	"github.com/cloudposse/atmos/pkg/auth/providers/aws/saml2aws/mocks"
	"github.com/cloudposse/atmos/pkg/auth/providers/aws/saml2aws/prompter"
	"github.com/cloudposse/atmos/pkg/auth/providers/aws/saml2aws/provider"
)

// withFastPolling shortens the package-level polling delays for the duration of a test.
func withFastPolling(t *testing.T) {
	t.Helper()

	origPush, origDuo, origSettle := pushPollInterval, duoPollInterval, endpointHealthSettleDelay
	pushPollInterval, duoPollInterval, endpointHealthSettleDelay = time.Millisecond, time.Millisecond, time.Millisecond
	t.Cleanup(func() {
		pushPollInterval, duoPollInterval, endpointHealthSettleDelay = origPush, origDuo, origSettle
	})
}

// withMockPrompter installs a mock prompter and restores the previous one when the test ends.
// A mock without expectations fails the test on any call, which makes unexpected prompts fail the test.
// The controller verifies the expectations when the test ends.
func withMockPrompter(t *testing.T) *mocks.MockPrompter {
	t.Helper()

	orig := prompter.ActivePrompter
	pr := mocks.NewMockPrompter(gomock.NewController(t))
	prompter.SetPrompter(pr)
	t.Cleanup(func() { prompter.SetPrompter(orig) })
	return pr
}

// withMockDevice makes the Okta and Duo U2F flows use the given mock device instead of real hardware.
// The flows may look the device up any number of times; the device mock's own expectations verify its use.
func withMockDevice(t *testing.T, device *mocks.MockU2FDevice) {
	t.Helper()

	withDeviceFinder(t, anyTimesFinderFor(gomock.NewController(t), device))
}

// withCredentialsHelperMock installs a credentials.Helper mock for the duration of a test. The Okta provider
// only stores the session cookie through it (Add); any other call fails the test because it has no expectation.
func withCredentialsHelperMock(t *testing.T) *mocks.MockHelper {
	t.Helper()

	h := mocks.NewMockHelper(gomock.NewController(t))
	withCredentialsHelper(t, h)
	return h
}

// withCredentialsHelper swaps the global credentials helper for the duration of a test.
func withCredentialsHelper(t *testing.T, h credentials.Helper) {
	t.Helper()

	orig := credentials.CurrentHelper
	credentials.CurrentHelper = h
	t.Cleanup(func() { credentials.CurrentHelper = orig })
}

// requestLog records the paths requested from a fake IdP in a goroutine-safe way.
type requestLog struct {
	mu    sync.Mutex
	paths []string
}

func (l *requestLog) add(path string) int {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.paths = append(l.paths, path)
	n := 0
	for _, p := range l.paths {
		if p == path {
			n++
		}
	}
	return n
}

func (l *requestLog) count(path string) int {
	l.mu.Lock()
	defer l.mu.Unlock()
	n := 0
	for _, p := range l.paths {
		if p == path {
			n++
		}
	}
	return n
}

// closedServerURL returns the URL of a TLS server that is no longer listening.
func closedServerURL(t *testing.T) string {
	t.Helper()

	ts := httptest.NewTLSServer(http.NotFoundHandler())
	u := ts.URL
	ts.Close()
	return u
}

// samlTargetPage renders an auto-submit form posting a SAML response to the given target.
func samlTargetPage(target, samlResponse string) string {
	return fmt.Sprintf(`<html><body><form method="POST" action="%s">`+
		`<input type="hidden" name="SAMLResponse" value="%s"/></form></body></html>`, target, samlResponse)
}

// encodedSAML returns a base64-encoded fake SAML assertion.
func encodedSAML() string {
	return base64.StdEncoding.EncodeToString([]byte("<saml:Assertion>fake</saml:Assertion>"))
}

// testFactor describes one entry of the Okta `_embedded.factors` array.
type testFactor struct {
	id         string
	provider   string
	factorType string
	href       string
	authName   string
}

// mfaRequiredResponse builds an Okta MFA_REQUIRED authn response with the given factors.
func mfaRequiredResponse(t *testing.T, stateToken string, factors ...testFactor) string {
	t.Helper()

	list := make([]map[string]any, 0, len(factors))
	for _, f := range factors {
		entry := map[string]any{
			"id":         f.id,
			"provider":   f.provider,
			"factorType": f.factorType,
			"_links":     map[string]any{"verify": map[string]any{"href": f.href}},
		}
		if f.authName != "" {
			entry["profile"] = map[string]any{"authenticatorName": f.authName}
		}
		list = append(list, entry)
	}

	body, err := json.Marshal(map[string]any{
		"status":     "MFA_REQUIRED",
		"stateToken": stateToken,
		"_embedded":  map[string]any{"factors": list},
	})
	require.NoError(t, err)
	return string(body)
}

// decodeJSONBody decodes a JSON request body into a generic map.
func decodeJSONBody(t *testing.T, r *http.Request) map[string]any {
	t.Helper()

	var body map[string]any
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		t.Errorf("failed to decode request body: %v", err)
	}
	return body
}

// offlineTestClient returns a test client (trusting the httptest TLS certificates) and login
// details that point at an unresolvable org, for tests that supply absolute URLs themselves.
func offlineTestClient(t *testing.T) (*Client, *creds.LoginDetails) {
	t.Helper()

	tr := http.DefaultTransport.(*http.Transport).Clone()
	tr.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
	opts := &provider.HTTPClientOptions{IsWithRetries: false}
	client, err := provider.NewHTTPClient(tr, opts)
	require.NoError(t, err)
	oc := &Client{client: client, mfa: "Auto", rememberDevice: true}
	return oc, &creds.LoginDetails{URL: "https://okta.invalid/app", Username: "user@example.com", Password: "test123"}
}

// mustDoc parses an HTML string into a goquery document.
func mustDoc(t *testing.T, html string) *goquery.Document {
	t.Helper()

	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	require.NoError(t, err)
	return doc
}
