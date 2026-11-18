package googleapps

import (
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/auth/providers/aws/saml2aws/creds"
	"github.com/cloudposse/atmos/pkg/auth/providers/aws/saml2aws/provider"
)

func TestGenerateFullURLIfRelativeResolution(t *testing.T) {
	const page = "https://accounts.google.com/signin/step/page?x=1"
	tests := []struct {
		name        string
		destination string
		want        string
	}{
		{name: "empty stays empty", destination: "", want: ""},
		{name: "absolute is unchanged", destination: "https://other.example.com/a", want: "https://other.example.com/a"},
		{name: "root relative", destination: "/signin/next", want: "https://accounts.google.com/signin/next"},
		// Regression: a directory-relative action stayed without scheme and host.
		{name: "directory relative", destination: "next", want: "https://accounts.google.com/signin/step/next"},
		{name: "query only", destination: "?step=2", want: "https://accounts.google.com/signin/step/page?step=2"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := generateFullURLIfRelative(tt.destination, page)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}

	t.Run("root relative against an unparsable page URL is an error", func(t *testing.T) {
		_, err := generateFullURLIfRelative("/next", "https://idp.example.com\x7f/x")
		require.Error(t, err)
	})
}

// Regression: with no form id present, the fields of every form were merged and the last form's action was used.
func TestExtractInputsByFormIDSelectsOneUnnamedForm(t *testing.T) {
	doc := mustDoc(t, `<html><body>
<form action="/signin/password"><input name="Email" value="user@example.com"></form>
<form action="/signin/recovery"><input name="Recovery" value="phone"></form>
</body></html>`, "https://accounts.google.com/start")

	formData, actionURL, err := extractInputsByFormID(doc, "gaia_loginform", "challenge")
	require.NoError(t, err)
	assert.Equal(t, "https://accounts.google.com/signin/password", actionURL)
	assert.Equal(t, url.Values{"Email": {"user@example.com"}}, formData)

	t.Run("a single unnamed form without an action posts to the document URL", func(t *testing.T) {
		doc := mustDoc(t, `<form><input name="a" value="b"></form>`, "https://accounts.google.com/v3/page")
		formData, actionURL, err := extractInputsByFormID(doc, "challenge")
		require.NoError(t, err)
		assert.Equal(t, "https://accounts.google.com/v3/page", actionURL)
		assert.Equal(t, url.Values{"a": {"b"}}, formData)
	})

	t.Run("no forms", func(t *testing.T) {
		_, _, err := extractInputsByFormID(mustDoc(t, `<p>none</p>`, "https://accounts.google.com/"), "challenge")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "could not find any forms")
	})
}

// Regression: the chooser rejected a device push entry that the direct dispatcher handles.
func TestLoadChallengeEntryPageOffersDevicePush(t *testing.T) {
	withStdin(t, "\n")
	chooser := `<html><body><h2>To sign in to your Google Account, choose a task from the list below.</h2>` +
		`<form action="/signin/challenge/dp/6" data-challengeentry="6"><input name="challengeId" value="6"></form>` +
		`</body></html>`
	f, got, err := runChallenge(t, chooser, map[string]pageHandler{
		"/signin/challenge/dp/6": static(`<html><body>` + twoStepHeader +
			`<form id="challenge" action="/signin/challenge/dp/6/done"><input name="challengeId" value="6"></form></body></html>`),
		"/signin/challenge/dp/6/done": static(samlPage),
	}, &creds.LoginDetails{})
	require.NoError(t, err)
	assert.Equal(t, testSAMLResponse, got)
	assert.Equal(t, 1, f.count("/signin/challenge/dp/6/done"))
}

// Regression: a failed wait (such as HTTP 503) submitted the verification form as if the phone approved.
func TestChallenge_PhoneApprovalHTTPError(t *testing.T) {
	var waits atomic.Int32
	waitServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		waits.Add(1)
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
	}))
	t.Cleanup(waitServer.Close)
	target, err := url.Parse(waitServer.URL)
	require.NoError(t, err)

	f := newFakeGoogle(t, map[string]pageHandler{
		pathStart: static(fixture(t, "challenge-prompt.html")),
	})
	rt := &rewriteTransport{host: "content.googleapis.com", target: target}
	client, err := provider.NewHTTPClient(rt, &provider.HTTPClientOptions{})
	require.NoError(t, err)
	kc := &Client{client: client}

	_, err = kc.loadChallengePage(f.ts.URL+pathStart, f.ts.URL, url.Values{}, &creds.LoginDetails{})
	require.ErrorIs(t, err, errPhoneApprovalFailed)
	assert.EqualValues(t, 1, waits.Load())
	assert.Zero(t, f.count("/signin/challenge/az/5"))
}

// Regression: a captcha answered after the password page was requested again was never handled.
func TestAuthenticate_CaptchaAfterPasswordResubmission(t *testing.T) {
	t.Setenv("TERM_PROGRAM", "Apple_Terminal")
	pr := withMockPrompter(t)
	pr.EXPECT().String("Captcha", "").Return("solved-captcha", nil).Times(1)

	routes := passwordFlowRoutes(t, static(fixture(t, "form-password-challengeid-2.html")))
	routes["/signin/challenge/pwd/2"] = static(fixture(t, "captcha.html"))
	routes["/v3/signin/identifier"] = static(samlPage)
	f := newFakeGoogle(t, routes)
	kc, loginDetails := f.client()

	got, err := kc.Authenticate(loginDetails)
	require.NoError(t, err)
	assert.Equal(t, testSAMLResponse, got)

	answers := f.formsFor("/v3/signin/identifier")
	require.Len(t, answers, 1)
	assert.Equal(t, "solved-captcha", answers[0].Get("ca"))
}

// The attempt budget is shared by the captchas before and after the password resubmission.
func TestAuthenticate_CaptchaBudgetSharedAcrossPasswordResubmission(t *testing.T) {
	t.Setenv("TERM_PROGRAM", "Apple_Terminal")
	pr := withMockPrompter(t)
	pr.EXPECT().String("Captcha", "").Return("answer", nil).MinTimes(1) // Unlimited, but at least one call (the former testify AssertExpectations contract).

	routes := passwordFlowRoutes(t, static(fixture(t, "form-password-challengeid-2.html")))
	routes["/signin/challenge/pwd/2"] = static(fixture(t, "captcha.html"))
	routes["/v3/signin/identifier"] = static(fixture(t, "captcha.html"))
	f := newFakeGoogle(t, routes)
	kc, loginDetails := f.client()

	_, err := kc.Authenticate(loginDetails)
	require.ErrorIs(t, err, errCaptchaAttemptsExceeded)
	assert.Len(t, f.formsFor("/v3/signin/identifier"), maxCaptchaAttempts)
}

// Regression: the captcha image was resolved against the password page, not the page that contains it.
func TestSolveCaptchasResolvesImageAgainstThePageThatContainsIt(t *testing.T) {
	t.Setenv("TERM_PROGRAM", "iTerm.app")
	t.Setenv("TERM", "xterm-256color")
	pr := withMockPrompter(t)
	pr.EXPECT().String("Captcha", "").Return("solved-captcha", nil).Times(1)

	other := newFakeGoogle(t, map[string]pageHandler{
		"/Captcha": static("png"),
		"/answer":  static(samlPage),
	})
	page := `<html><body><div class="captcha-img"><img src="/Captcha"></div>` +
		`<form id="challenge" action="/answer"><input name="ca" value=""></form></body></html>`
	doc := mustDoc(t, page, other.ts.URL+"/captcha-page")

	kc := &Client{client: mustHTTPClient(t)}
	attempts := 0
	var got string
	var err error
	_, _ = captureOutput(t, func() {
		doc, err = kc.solveCaptchas(doc, &creds.LoginDetails{}, &attempts)
		if err == nil {
			got, err = findInputByName(doc, "SAMLResponse")
		}
	})
	require.NoError(t, err)
	assert.Equal(t, testSAMLResponse, got)
	assert.Equal(t, 1, other.count("/Captcha"))
	assert.Equal(t, 1, attempts)
}

func TestITermCaptchaPrompt(t *testing.T) {
	t.Run("the challenge URL is not logged", func(t *testing.T) {
		pr := withMockPrompter(t)
		pr.EXPECT().String("Captcha", "").Return("answer", nil).Times(1)
		f := newFakeGoogle(t, map[string]pageHandler{"/Captcha": static("png")})
		logs := captureStdLog(t)

		kc := &Client{client: mustHTTPClient(t)}
		_, _ = captureOutput(t, func() {
			_, err := kc.iTermCaptchaPrompt(f.ts.URL + "/Captcha?ctoken=secret-token")
			require.NoError(t, err)
		})
		assert.NotContains(t, logs.String(), "secret-token")
		assert.NotContains(t, logs.String(), f.ts.URL)
	})

	// Regression: an interrupted download printed the partial image and prompted for a code.
	t.Run("an interrupted download is an error and does not prompt", func(t *testing.T) {
		withMockPrompter(t) // Any prompt fails the test.
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Length", "100")
			_, _ = io.WriteString(w, "partial")
		}))
		t.Cleanup(ts.Close)

		kc := &Client{client: mustHTTPClient(t)}
		var err error
		_, stderr := captureOutput(t, func() { _, err = kc.iTermCaptchaPrompt(ts.URL + "/Captcha") })
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unable to read captcha image")
		assert.NotContains(t, stderr, "1337;File=")
	})
}

// captureStdLog redirects the standard logger and returns what it wrote.
func captureStdLog(t *testing.T) *strings.Builder {
	t.Helper()
	var sb strings.Builder
	origOut, origFlags := log.Writer(), log.Flags()
	log.SetOutput(&sb)
	log.SetFlags(0)
	t.Cleanup(func() {
		log.SetOutput(origOut)
		log.SetFlags(origFlags)
	})
	return &sb
}
