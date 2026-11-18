package googleapps

import (
	"encoding/base64"
	"net/http"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/auth/providers/aws/saml2aws/cfg"
	"github.com/cloudposse/atmos/pkg/auth/providers/aws/saml2aws/creds"
	"github.com/cloudposse/atmos/pkg/auth/providers/aws/saml2aws/provider"
)

const pathPassword = "/signin/challenge/pwd/1"

// passwordFlowRoutes returns routes for first page, email lookup and password submission.
func passwordFlowRoutes(t *testing.T, afterPassword pageHandler) map[string]pageHandler {
	return map[string]pageHandler{
		pathFirstPage: static(firstPage),
		pathLookup:    static(fixture(t, "form-password-challengeid-1.html")),
		pathPassword:  afterPassword,
	}
}

func TestAuthenticate_PasswordFlow(t *testing.T) {
	f := newFakeGoogle(t, passwordFlowRoutes(t, static(samlPage)))
	kc, loginDetails := f.client()

	got, err := kc.Authenticate(loginDetails)
	require.NoError(t, err)
	assert.Equal(t, testSAMLResponse, got)

	lookups := f.formsFor(pathLookup)
	require.Len(t, lookups, 1)
	assert.Equal(t, "user@example.com", lookups[0].Get("Email"))
	assert.Equal(t, "js_enabled", lookups[0].Get("bgresponse"))
	assert.Equal(t, "galx-token", lookups[0].Get("GALX"))

	passwords := f.formsFor(pathPassword)
	require.Len(t, passwords, 1)
	assert.Equal(t, "s3cret", passwords[0].Get("Passwd"))
	assert.Equal(t, "on", passwords[0].Get("TrustDevice"))
	assert.Equal(t, "1", passwords[0].Get("challengeId"))
}

func TestAuthenticate_IdentifierFallback(t *testing.T) {
	routes := passwordFlowRoutes(t, static(samlPage))
	passwordPage := routes[pathLookup](1, nil)
	routes[pathLookup] = func(call int, _ url.Values) string {
		if call == 1 {
			return `<html><body>no forms here</body></html>`
		}
		return passwordPage
	}
	f := newFakeGoogle(t, routes)
	kc, loginDetails := f.client()

	got, err := kc.Authenticate(loginDetails)
	require.NoError(t, err)
	assert.Equal(t, testSAMLResponse, got)

	lookups := f.formsFor(pathLookup)
	require.Len(t, lookups, 2)
	assert.Equal(t, "user@example.com", lookups[0].Get("Email"))
	assert.Empty(t, lookups[1].Get("Email"))
	assert.Equal(t, "user@example.com", lookups[1].Get("identifier"))
}

func TestAuthenticate_Errors(t *testing.T) {
	noForms := static(`<html><body>nothing</body></html>`)

	tests := []struct {
		name    string
		routes  func(t *testing.T) map[string]pageHandler
		wantErr string
	}{
		{
			name:    "first page without login form",
			routes:  func(*testing.T) map[string]pageHandler { return map[string]pageHandler{pathFirstPage: noForms} },
			wantErr: "error loading first page",
		},
		{
			name: "lookup rejected for email and identifier",
			routes: func(*testing.T) map[string]pageHandler {
				return map[string]pageHandler{pathFirstPage: static(firstPage), pathLookup: noForms}
			},
			wantErr: "error loading login page",
		},
		{
			name: "invalid credentials",
			routes: func(t *testing.T) map[string]pageHandler {
				return passwordFlowRoutes(t, static(`<html><body><span class="error-msg">Wrong password</span></body></html>`))
			},
			wantErr: "Invalid username or password",
		},
		{
			name: "password error marker",
			routes: func(t *testing.T) map[string]pageHandler {
				return passwordFlowRoutes(t, static(`<html><body><span id="passwordError">Wrong password. Try again.</span></body></html>`))
			},
			wantErr: "Password error",
		},
		{
			name: "2-step verification not set up",
			routes: func(t *testing.T) map[string]pageHandler {
				return passwordFlowRoutes(t, static(`<html><body><section class="aN1Vld "><p>Policy not met</p></section></body></html>`))
			},
			wantErr: "you must set-up 2-Step Verification",
		},
		{
			name: "no SAML assertion",
			routes: func(t *testing.T) map[string]pageHandler {
				return passwordFlowRoutes(t, static(`<html><body><p>Welcome</p></body></html>`))
			},
			wantErr: "page is missing saml assertion",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f := newFakeGoogle(t, tc.routes(t))
			kc, loginDetails := f.client()

			got, err := kc.Authenticate(loginDetails)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.wantErr)
			assert.Empty(t, got)
		})
	}
}

func TestAuthenticate_PasswordRequestedAgain(t *testing.T) {
	routes := passwordFlowRoutes(t, static(fixture(t, "form-password-challengeid-2.html")))
	routes["/signin/challenge/pwd/2"] = static(samlPage)
	f := newFakeGoogle(t, routes)
	kc, loginDetails := f.client()

	got, err := kc.Authenticate(loginDetails)
	require.NoError(t, err)
	assert.Equal(t, testSAMLResponse, got)

	second := f.formsFor("/signin/challenge/pwd/2")
	require.Len(t, second, 1)
	assert.Equal(t, "s3cret", second[0].Get("Passwd"))
	assert.Equal(t, "2", second[0].Get("challengeId"))
}

func TestAuthenticate_Captcha(t *testing.T) {
	const captchaImage = "fake-png-bytes"

	newCaptchaServer := func(t *testing.T) *fakeGoogle {
		routes := passwordFlowRoutes(t, static(fixture(t, "captcha.html")))
		routes["/v3/signin/identifier"] = static(samlPage)
		routes["/Captcha"] = static(captchaImage)
		return newFakeGoogle(t, routes)
	}

	assertCaptchaSubmitted := func(t *testing.T, f *fakeGoogle) {
		t.Helper()
		captchas := f.formsFor("/v3/signin/identifier")
		require.Len(t, captchas, 1)
		assert.Equal(t, "solved-captcha", captchas[0].Get("ca"))
		assert.Equal(t, "super_long_captcha_string", captchas[0].Get("ct"))
		assert.Equal(t, "js_enabled", captchas[0].Get("bgresponse"))
	}

	t.Run("plain terminal prints the captcha URL", func(t *testing.T) {
		t.Setenv("TERM_PROGRAM", "Apple_Terminal")
		pr := withMockPrompter(t)
		pr.On("String", "Captcha", "").Return("solved-captcha").Once()

		f := newCaptchaServer(t)
		kc, loginDetails := f.client()
		got, err := kc.Authenticate(loginDetails)
		require.NoError(t, err)
		assert.Equal(t, testSAMLResponse, got)
		assertCaptchaSubmitted(t, f)
		// The image is only fetched for inline terminal display.
		assert.Zero(t, f.count("/Captcha"))
	})

	for _, term := range []string{"xterm-256color", "screen"} {
		t.Run("iTerm displays the image inline with TERM="+term, func(t *testing.T) {
			t.Setenv("TERM_PROGRAM", "iTerm.app")
			t.Setenv("TERM", term)
			pr := withMockPrompter(t)
			pr.On("String", "Captcha", "").Return("solved-captcha").Once()

			f := newCaptchaServer(t)
			kc, loginDetails := f.client()
			var got string
			var err error
			stdout, stderr := captureOutput(t, func() { got, err = kc.Authenticate(loginDetails) })
			require.NoError(t, err)
			assert.Equal(t, testSAMLResponse, got)
			assertCaptchaSubmitted(t, f)
			assert.Equal(t, 1, f.count("/Captcha"))
			// Regression: the inline image went to stdout, the data channel, which corrupts
			// output such as `eval $(atmos auth env)`. It must go to stderr.
			assert.Empty(t, stdout)
			assert.Contains(t, stderr, "1337;File=")
			assert.Contains(t, stderr, base64.StdEncoding.EncodeToString([]byte(captchaImage)))
		})
	}

	t.Run("iTerm image download failure", func(t *testing.T) {
		kc := &Client{client: mustHTTPClient(t)}
		_, err := kc.iTermCaptchaPrompt(closedServerURL(t) + "/Captcha")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unable to fetch captcha image")
	})
}

// Regression: the new-style captcha input is `name="ca"` with no id. Upstream re-checked only
// `#ca` after submitting, so a rejected answer was never retried and the flow failed looking
// for a SAMLResponse on the captcha page.
func TestAuthenticate_CaptchaRetriedAfterRejectedAnswer(t *testing.T) {
	t.Setenv("TERM_PROGRAM", "Apple_Terminal")
	pr := withMockPrompter(t)
	pr.On("String", "Captcha", "").Return("wrong-answer").Once()
	pr.On("String", "Captcha", "").Return("solved-captcha").Once()

	captchaPage := fixture(t, "captcha.html")
	routes := passwordFlowRoutes(t, static(captchaPage))
	// The first answer is rejected (the captcha page comes back), the second is accepted.
	routes["/v3/signin/identifier"] = func(call int, _ url.Values) string {
		if call == 1 {
			return captchaPage
		}
		return samlPage
	}
	f := newFakeGoogle(t, routes)
	kc, loginDetails := f.client()

	got, err := kc.Authenticate(loginDetails)
	require.NoError(t, err)
	assert.Equal(t, testSAMLResponse, got)

	answers := f.formsFor("/v3/signin/identifier")
	require.Len(t, answers, 2)
	assert.Equal(t, "wrong-answer", answers[0].Get("ca"))
	assert.Equal(t, "solved-captcha", answers[1].Get("ca"))
	pr.AssertExpectations(t)
}

// A page that keeps asking for a captcha must not loop forever.
func TestAuthenticate_CaptchaAttemptsExceeded(t *testing.T) {
	t.Setenv("TERM_PROGRAM", "Apple_Terminal")
	pr := withMockPrompter(t)
	pr.On("String", "Captcha", "").Return("wrong-answer")

	captchaPage := fixture(t, "captcha.html")
	routes := passwordFlowRoutes(t, static(captchaPage))
	routes["/v3/signin/identifier"] = static(captchaPage)
	f := newFakeGoogle(t, routes)
	kc, loginDetails := f.client()

	_, err := kc.Authenticate(loginDetails)
	require.ErrorIs(t, err, errCaptchaAttemptsExceeded)
	assert.Len(t, f.formsFor("/v3/signin/identifier"), maxCaptchaAttempts)
}

func TestFindCaptcha(t *testing.T) {
	tests := []struct {
		name      string
		html      string
		wantField string
		wantFound bool
	}{
		{name: "legacy login captcha", html: `<input id="logincaptcha">`, wantField: "logincaptcha", wantFound: true},
		{name: "identifier captcha", html: `<input id="identifier-captcha-input">`, wantField: "identifier-captcha-input", wantFound: true},
		{name: "captcha image", html: `<img id="captchaimg">`, wantField: "captchaimg", wantFound: true},
		{name: "new-style input by name only", html: `<input type="text" name="ca">`, wantField: "ca", wantFound: true},
		{name: "no captcha", html: `<input name="Passwd">`, wantFound: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			found, field := findCaptcha(mustDoc(t, "<html><body>"+tt.html+"</body></html>", "https://accounts.google.com/"))
			assert.Equal(t, tt.wantFound, found.Length() > 0)
			assert.Equal(t, tt.wantField, field)
		})
	}
}

func TestAuthenticate_UnreachableIdP(t *testing.T) {
	kc := &Client{client: mustHTTPClient(t)}
	_, err := kc.Authenticate(&creds.LoginDetails{URL: closedServerURL(t) + "/sso?x=1"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to make request to login form")
}

func TestLoadChallengeEntryPage(t *testing.T) {
	chooser := `<html><body><h2>To sign in to your Google Account, choose a task from the list below.</h2>` +
		`<form action="/signin/challenge/sk/1" data-challengeentry="1"><input name="challengeId" value="1"></form>` +
		`<form action="/signin/challenge/totp/2" data-challengeentry="2"><input name="challengeId" value="2"></form>` +
		`</body></html>`

	t.Run("selects the first supported challenge", func(t *testing.T) {
		f := newFakeGoogle(t, map[string]pageHandler{
			"/start": static(chooser),
			"/signin/challenge/totp/2": func(call int, form url.Values) string {
				if call == 1 {
					assert.Equal(t, "2", form.Get("challengeId"))
					return fixture(t, "challenge-totp.html")
				}
				assert.Equal(t, "123456", form.Get("Pin"))
				return samlPage
			},
		})
		kc := &Client{client: mustHTTPClient(t)}
		doc, err := kc.loadChallengePage(f.ts.URL+"/start", f.ts.URL, url.Values{}, &creds.LoginDetails{MFAToken: "123456"})
		require.NoError(t, err)
		assertion, err := findInputByName(doc, "SAMLResponse")
		require.NoError(t, err)
		assert.Equal(t, testSAMLResponse, assertion)
		assert.Equal(t, 2, f.count("/signin/challenge/totp/2"))
		assert.Zero(t, f.count("/signin/challenge/sk/1"))
	})

	t.Run("no supported challenge", func(t *testing.T) {
		kc := &Client{client: mustHTTPClient(t)}
		doc := mustDoc(t, `<form action="/signin/challenge/sk/1" data-challengeentry="1"></form><form data-challengeentry="9"></form>`, "https://accounts.google.com/x")
		_, err := kc.loadChallengeEntryPage(doc, "https://accounts.google.com/x", &creds.LoginDetails{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unable to find supported second factor")
	})
}

func TestNew_BuildsClient(t *testing.T) {
	idpAccount := cfg.NewIDPAccount()
	idpAccount.URL = "https://accounts.google.com/o/saml2/initsso?idpid=abc"

	kc, err := New(idpAccount)
	require.NoError(t, err)
	require.NotNil(t, kc.client)
	assert.Nil(t, kc.client.CheckResponseStatus)
}

// mustHTTPClient returns a provider HTTP client built the same way New does.
func mustHTTPClient(t *testing.T) *provider.HTTPClient {
	t.Helper()

	client, err := provider.NewHTTPClient(http.DefaultTransport, &provider.HTTPClientOptions{IsWithRetries: false})
	require.NoError(t, err)
	return client
}
