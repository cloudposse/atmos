package googleapps

import (
	"fmt"
	"testing"

	u2fhost "github.com/marshallbrekka/go-u2fhost"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

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
