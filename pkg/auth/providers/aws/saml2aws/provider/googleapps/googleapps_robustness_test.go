package googleapps

import (
	"fmt"
	"net/url"
	"testing"

	u2fhost "github.com/marshallbrekka/go-u2fhost"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/auth/providers/aws/saml2aws/creds"
	"github.com/cloudposse/atmos/pkg/auth/providers/aws/saml2aws/mocks"
)

// failOnProcessExit turns a logrus Fatal (which calls os.Exit) into a panic for the duration of the
// test, so an exit path fails the test instead of terminating the test binary.
func failOnProcessExit(t *testing.T) {
	t.Helper()
	std := logrus.StandardLogger()
	orig := std.ExitFunc
	std.ExitFunc = func(code int) { panic(fmt.Sprintf("process exit with code %d", code)) }
	t.Cleanup(func() { std.ExitFunc = orig })
}

// Regression: a SAMLResponse input without a value attribute called logger.Fatal, which exits the
// whole Atmos process. It must return an error.
func TestAuthenticate_SAMLResponseWithoutValue(t *testing.T) {
	failOnProcessExit(t)
	page := `<html><body><form method="POST" action="https://signin.aws.amazon.com/saml">` +
		`<input type="hidden" name="SAMLResponse"></form></body></html>`
	f := newFakeGoogle(t, passwordFlowRoutes(t, static(page)))
	kc, loginDetails := f.client()

	got, err := kc.Authenticate(loginDetails)
	require.ErrorIs(t, err, errInputValueNotFound)
	assert.Contains(t, err.Error(), "SAMLResponse")
	assert.Empty(t, got)
}

// Regression: the captcha loop always used div[data-auto-init='CaptchaInput'] because a
// *goquery.Selection is never nil, so the legacy .captcha-img markup was ignored and indexing the
// missing div's children panicked.
func TestAuthenticate_LegacyCaptchaImage(t *testing.T) {
	t.Setenv("TERM_PROGRAM", "Apple_Terminal")
	pr := withMockPrompter(t)
	pr.On("String", "Captcha", "").Return("solved-legacy").Once()

	const legacyCaptchaPage = `<html><body><form id="gaia_loginform" method="POST" action="/legacy-captcha">` +
		`<input type="hidden" name="Passwd" value="">` +
		`<div class="captcha-img"><img src="/Captcha?legacy=1"></div>` +
		`<input type="text" id="logincaptcha" name="logincaptcha" value="">` +
		`</form></body></html>`
	routes := passwordFlowRoutes(t, static(legacyCaptchaPage))
	routes["/legacy-captcha"] = static(samlPage)
	f := newFakeGoogle(t, routes)
	kc, loginDetails := f.client()

	got, err := kc.Authenticate(loginDetails)
	require.NoError(t, err)
	assert.Equal(t, testSAMLResponse, got)

	forms := f.formsFor("/legacy-captcha")
	require.Len(t, forms, 1)
	assert.Equal(t, "solved-legacy", forms[0].Get("logincaptcha"))
	assert.Equal(t, "s3cret", forms[0].Get("Passwd"))
	assert.Empty(t, forms[0].Get("ca"), "the new-style captcha field must not be used for the legacy markup")
}

// Regression: a captcha container without an image child panicked (index out of range).
func TestAuthenticate_CaptchaWithoutImage(t *testing.T) {
	pages := map[string]string{
		"empty new-style container": `<html><body><form id="challenge" method="POST" action="/next">` +
			`<input type="text" id="identifier-captcha-input" name="ca" value="">` +
			`<div data-auto-init="CaptchaInput"></div></form></body></html>`,
		"empty legacy container": `<html><body><form id="challenge" method="POST" action="/next">` +
			`<input type="text" id="logincaptcha" name="logincaptcha" value="">` +
			`<div class="captcha-img"></div></form></body></html>`,
		"no container": `<html><body><form id="challenge" method="POST" action="/next">` +
			`<input type="text" id="logincaptcha" name="logincaptcha" value=""></form></body></html>`,
	}

	for name, page := range pages {
		t.Run(name, func(t *testing.T) {
			withMockPrompter(t) // No prompt is expected.
			f := newFakeGoogle(t, passwordFlowRoutes(t, static(page)))
			kc, loginDetails := f.client()

			var got string
			var err error
			require.NotPanics(t, func() { got, err = kc.Authenticate(loginDetails) })
			require.Error(t, err)
			assert.Contains(t, err.Error(), "captcha image not found but requested")
			assert.Empty(t, got)
		})
	}
}

// Regression: b64Safe panicked on a challenge nonce or key handle that is not valid base64.
func TestChallengeU2F_InvalidBase64(t *testing.T) {
	tests := map[string]struct {
		nonce     string
		keyHandle string
	}{
		"invalid challenge nonce": {nonce: "not base64!", keyHandle: "dGVzdAo="},
		"invalid key handle":      {nonce: "dGVzdAo=", keyHandle: "%%%"},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			device := &mocks.U2FDevice{}
			device.On("Close").Return().Once()
			client := &U2FClient{ChallengeNonce: tt.nonce, KeyHandle: tt.keyHandle, AppID: "appID", Facet: "facet", Device: device}

			var got string
			var err error
			require.NotPanics(t, func() { got, err = client.ChallengeU2F() })
			require.Error(t, err)
			assert.Empty(t, got)
			// The device is closed and never asked to authenticate.
			device.AssertExpectations(t)
			device.AssertNotCalled(t, "Authenticate", (*u2fhost.AuthenticateRequest)(nil))
		})
	}
}

func TestB64Safe(t *testing.T) {
	got, err := b64Safe("dGVzdAo=")
	require.NoError(t, err)
	assert.Equal(t, "dGVzdAo", got)

	_, err = b64Safe("not base64!")
	require.Error(t, err)
}

// Regression: the locale parameters are added to the query instead of being appended to the value of
// the last existing parameter.
func TestWithLocale(t *testing.T) {
	tests := []struct{ in, want string }{
		{"https://idp.example.com/pw", "https://idp.example.com/pw?hl=en&loc=US"},
		{"https://idp.example.com/pw?a=1&b=2", "https://idp.example.com/pw?a=1&b=2&hl=en&loc=US"},
		{"https://idp.example.com/pw?TL=abc%2Bdef", "https://idp.example.com/pw?TL=abc%2Bdef&hl=en&loc=US"},
		{"https://idp.example.com/pw?hl=fr&a=1&loc=CA", "https://idp.example.com/pw?a=1&hl=en&loc=US"},
		{"https://idp.example.com/pw?a=1#frag", "https://idp.example.com/pw?a=1&hl=en&loc=US#frag"},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.want, withLocale(tt.in), tt.in)
	}
}

// Regression: a challenge selection page that keeps returning the same selection page must end with an
// error instead of recursing forever.
func TestLoadChallengeEntryPageTransitionsAreBounded(t *testing.T) {
	chooser := `<html><body><h2>To sign in to your Google Account, choose a task from the list below.</h2>` +
		`<form action="/signin/challenge/totp/2" data-challengeentry="2"><input name="challengeId" value="2"></form>` +
		`</body></html>`
	f := newFakeGoogle(t, map[string]pageHandler{
		"/start":                   static(chooser),
		"/signin/challenge/totp/2": static(chooser),
	})
	kc := &Client{client: mustHTTPClient(t)}

	_, err := kc.loadChallengePage(f.ts.URL+"/start", f.ts.URL, url.Values{}, &creds.LoginDetails{})
	require.ErrorIs(t, err, errMFATransitionsExceeded)
	assert.Equal(t, maxMFATransitions, f.count("/signin/challenge/totp/2"))
}

// Regression: the MFA response document needs a URL, or a captcha on it panics in extractInputsByFormQuery.
func TestLoadResponsePageSetsDocumentURL(t *testing.T) {
	f := newFakeGoogle(t, map[string]pageHandler{"/resp": static(`<html><body><form id="f" action="/next"></form></body></html>`)})
	kc := &Client{client: mustHTTPClient(t)}

	doc, err := kc.loadResponsePage(f.ts.URL+"/resp", f.ts.URL, url.Values{})
	require.NoError(t, err)
	require.NotNil(t, doc.Url)
	assert.Equal(t, f.ts.URL+"/resp", doc.Url.String())

	_, actionURL, err := extractInputsByFormQuery(doc, "#f")
	require.NoError(t, err)
	assert.Equal(t, f.ts.URL+"/next", actionURL)
}

// Regression: headings wrapped in whitespace (as in challenge-sms.html) still match.
func TestExtractNodeTextNormalizesWhitespace(t *testing.T) {
	doc := mustDoc(t, "<h2>\n    Send   text\n  message\n</h2><h1>Inline</h1>", "https://accounts.google.com/")
	assert.Equal(t, "Send text message", extractNodeText(doc, "h2", "Send text message"))
	assert.Equal(t, "Inline", extractNodeText(doc, "h1", "Inline"))
	assert.Empty(t, extractNodeText(doc, "h2", "Other"))
}

// Regression: a data-challenge-ui value without a JSON object must not panic.
func TestExtractKeyHandlesMalformedChallengeUI(t *testing.T) {
	for _, value := range []string{"[]", "", "}{", "no braces"} {
		doc := mustDoc(t, `<div jsname="C0oDBd" data-challenge-ui='`+value+`'></div>`, "https://accounts.google.com/")
		appID, handles := extractKeyHandles(doc, testChallenge)
		assert.Empty(t, appID, value)
		assert.Empty(t, handles, value)
	}
}
