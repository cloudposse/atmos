package googleapps

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"testing"

	"github.com/PuerkitoBio/goquery"
	"github.com/marshallbrekka/go-u2fhost"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/auth/providers/aws/saml2aws/creds"
	"github.com/cloudposse/atmos/pkg/auth/providers/aws/saml2aws/mocks"
	"github.com/cloudposse/atmos/pkg/auth/providers/aws/saml2aws/provider"
)

const pathStart = "/start"

// challengePage renders a 2-Step Verification page whose challenge form posts to action.
func challengePage(action, extra string) string {
	return `<html><body>` + twoStepHeader + `<form method="POST" id="challenge" action="` + action + `">` +
		`<input type="hidden" name="challengeId" value="7">` + extra + `</form>` +
		`<form method="POST" action="/signin/challenge/skip"><input type="hidden" name="challengeId" value="7"></form>` +
		`</body></html>`
}

// runChallenge serves startPage at /start, routes the rest, and runs loadChallengePage against it.
func runChallenge(t *testing.T, startPage string, routes map[string]pageHandler, loginDetails *creds.LoginDetails) (*fakeGoogle, string, error) {
	t.Helper()

	routes[pathStart] = static(startPage)
	f := newFakeGoogle(t, routes)
	kc := &Client{client: mustHTTPClient(t)}
	doc, err := kc.loadChallengePage(f.ts.URL+pathStart, f.ts.URL, url.Values{}, loginDetails)
	if err != nil {
		return f, "", err
	}
	assertion, err := findInputByName(doc, "SAMLResponse")
	return f, assertion, err
}

func TestChallenge_TOTPPromptsWhenNoToken(t *testing.T) {
	pr := withMockPrompter(t)
	pr.On("RequestSecurityCode", "000000").Return("246810").Once()

	f, got, err := runChallenge(t, fixture(t, "challenge-totp.html"), map[string]pageHandler{
		"/signin/challenge/totp/2": static(samlPage),
	}, &creds.LoginDetails{})
	require.NoError(t, err)
	assert.Equal(t, testSAMLResponse, got)

	forms := f.formsFor("/signin/challenge/totp/2")
	require.Len(t, forms, 1)
	assert.Equal(t, "246810", forms[0].Get("Pin"))
	assert.Equal(t, "on", forms[0].Get("TrustDevice"))
}

func TestChallenge_SMSAlreadySent(t *testing.T) {
	pr := withMockPrompter(t)
	pr.On("StringRequired", "Enter SMS token: G-").Return("135790").Once()

	f, got, err := runChallenge(t, challengePage("/signin/challenge/ipp/3", ""), map[string]pageHandler{
		"/signin/challenge/ipp/3": static(samlPage),
	}, &creds.LoginDetails{})
	require.NoError(t, err)
	assert.Equal(t, testSAMLResponse, got)

	forms := f.formsFor("/signin/challenge/ipp/3")
	require.Len(t, forms, 1)
	assert.Equal(t, "135790", forms[0].Get("Pin"))
	assert.Empty(t, forms[0].Get("SendMethod"))
}

func TestChallenge_SecurityKeyOTP(t *testing.T) {
	// Regression: skotp URLs also contain "challenge/sk", so they used to be routed to the U2F handler.
	pr := withMockPrompter(t)
	pr.On("RequestSecurityCode", "000 000").Return("112233").Once()
	withDeviceFinder(t, &scriptedDeviceFinder{err: errNoDeviceFound})

	f, got, err := runChallenge(t, challengePage("/signin/challenge/skotp/4", ""), map[string]pageHandler{
		"/signin/challenge/skotp/4": static(samlPage),
	}, &creds.LoginDetails{})
	require.NoError(t, err)
	assert.Equal(t, testSAMLResponse, got)

	forms := f.formsFor("/signin/challenge/skotp/4")
	require.Len(t, forms, 1)
	assert.Equal(t, "112233", forms[0].Get("Pin"))
}

func TestChallenge_PhonePrompt(t *testing.T) {
	const awaitPath = "/cryptauth/v1/authzen/awaittx"

	newClient := func(t *testing.T, awaitTarget string) (*Client, *rewriteTransport) {
		target, err := url.Parse(awaitTarget)
		require.NoError(t, err)
		rt := &rewriteTransport{host: "content.googleapis.com", target: target}
		client, err := provider.NewHTTPClient(rt, &provider.HTTPClientOptions{})
		require.NoError(t, err)
		return &Client{client: client}, rt
	}

	t.Run("waits for the Google app approval", func(t *testing.T) {
		f := newFakeGoogle(t, map[string]pageHandler{
			pathStart:                static(fixture(t, "challenge-prompt.html")),
			awaitPath:                static(`{}`),
			"/signin/challenge/az/5": static(samlPage),
		})
		kc, rt := newClient(t, f.ts.URL)

		doc, err := kc.loadChallengePage(f.ts.URL+pathStart, f.ts.URL, url.Values{}, &creds.LoginDetails{})
		require.NoError(t, err)
		assertion, err := findInputByName(doc, "SAMLResponse")
		require.NoError(t, err)
		assert.Equal(t, testSAMLResponse, assertion)
		assert.Equal(t, 1, rt.hitCount())
		assert.Equal(t, []map[string]string{{"txId": "XXXXXX-XXXXXX-XXXXXX="}}, f.jsonFor(awaitPath))

		forms := f.formsFor("/signin/challenge/az/5")
		require.Len(t, forms, 1)
		assert.Equal(t, "on", forms[0].Get("TrustDevice"))
	})

	t.Run("await call failure", func(t *testing.T) {
		f := newFakeGoogle(t, map[string]pageHandler{pathStart: static(fixture(t, "challenge-prompt.html"))})
		kc, rt := newClient(t, closedServerURL(t))

		_, err := kc.loadChallengePage(f.ts.URL+pathStart, f.ts.URL, url.Values{}, &creds.LoginDetails{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unable to extract post wait tx form")
		assert.Equal(t, 1, rt.hitCount())
		assert.Zero(t, f.count("/signin/challenge/az/5"))
	})
}

func TestChallenge_DevicePush(t *testing.T) {
	t.Run("continues after ENTER", func(t *testing.T) {
		withStdin(t, "\n")
		f, got, err := runChallenge(t, fixture(t, "challenge-extra-number.html"), map[string]pageHandler{
			"/signin/challenge/dp/6": static(samlPage),
		}, &creds.LoginDetails{})
		require.NoError(t, err)
		assert.Equal(t, testSAMLResponse, got)
		require.Len(t, f.formsFor("/signin/challenge/dp/6"), 1)
		assert.Equal(t, "on", f.formsFor("/signin/challenge/dp/6")[0].Get("TrustDevice"))
	})

	t.Run("without extra number", func(t *testing.T) {
		withStdin(t, "\n")
		_, got, err := runChallenge(t, challengePage("/signin/challenge/dp/8", ""), map[string]pageHandler{
			"/signin/challenge/dp/8": static(samlPage),
		}, &creds.LoginDetails{})
		require.NoError(t, err)
		assert.Equal(t, testSAMLResponse, got)
	})

	t.Run("stdin closed before ENTER", func(t *testing.T) {
		withStdin(t, "")
		f, _, err := runChallenge(t, fixture(t, "challenge-extra-number.html"), map[string]pageHandler{}, &creds.LoginDetails{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "error reading new line")
		assert.Zero(t, f.count("/signin/challenge/dp/6"))
	})
}

const (
	testChallenge = "Y2hhbGxlbmdl"
	testKeyHandle = "a2V5aGFuZGxl"
)

// securityKeyPage renders a U2F challenge page, optionally with one key handle and an app id.
func securityKeyPage(withKeyHandle bool) string {
	ui := `{"1":[["` + testKeyHandle + `","{\"appid\":\"https://www.gstatic.com/securitykey/origins.json\"}"]]}`
	if !withKeyHandle {
		ui = `{"1":[["not base64!"]]}`
	}
	return challengePage("/signin/challenge/sk/5",
		`<input type="hidden" name="id-challenge" value="`+testChallenge+`">`) +
		`<div jsname="C0oDBd" data-challenge-ui='` + ui + `'></div>`
}

func TestChallenge_SecurityKey(t *testing.T) {
	t.Run("assertion is posted", func(t *testing.T) {
		device := &mocks.U2FDevice{}
		device.On("Close").Return().Once()
		device.On("Authenticate", mock.MatchedBy(func(r *u2fhost.AuthenticateRequest) bool {
			return r.Challenge == "Y2hhbGxlbmdl" && r.KeyHandle == "a2V5aGFuZGxl" &&
				r.AppId == "https://www.gstatic.com/securitykey/origins.json" && strings.HasPrefix(r.Facet, "http://127.0.0.1:")
		})).Return(&u2fhost.AuthenticateResponse{ClientData: "cd", SignatureData: "sig"}, nil).Once()
		withDeviceFinder(t, &scriptedDeviceFinder{device: device})

		f, got, err := runChallenge(t, securityKeyPage(true), map[string]pageHandler{
			"/signin/challenge/sk/5": static(samlPage),
		}, &creds.LoginDetails{})
		require.NoError(t, err)
		assert.Equal(t, testSAMLResponse, got)
		device.AssertExpectations(t)

		forms := f.formsFor("/signin/challenge/sk/5")
		require.Len(t, forms, 1)
		var assertion u2fhost.AuthenticateResponse
		require.NoError(t, json.Unmarshal([]byte(forms[0].Get("id-assertion")), &assertion))
		assert.Equal(t, "cd", assertion.ClientData)
		assert.Equal(t, "sig", assertion.SignatureData)
	})

	t.Run("device failure falls back to another challenge", func(t *testing.T) {
		device := &mocks.U2FDevice{}
		device.On("Close").Return().Once()
		device.On("Authenticate", mock.Anything).Return(nil, fmt.Errorf("device unplugged")).Once()
		withDeviceFinder(t, &scriptedDeviceFinder{device: device})

		chooser := `<form action="/signin/challenge/totp/9" data-challengeentry="9"><input name="challengeId" value="9"></form>`
		f, got, err := runChallenge(t, securityKeyPage(true), map[string]pageHandler{
			"/signin/challenge/skip":   static(chooser),
			"/signin/challenge/totp/9": static(samlPage),
		}, &creds.LoginDetails{})
		require.NoError(t, err)
		assert.Equal(t, testSAMLResponse, got)
		assert.Equal(t, 1, f.count("/signin/challenge/skip"))
		assert.Zero(t, f.count("/signin/challenge/sk/5"))
	})

	t.Run("no device plugged in", func(t *testing.T) {
		withDeviceFinder(t, &scriptedDeviceFinder{err: errNoDeviceFound})
		_, _, err := runChallenge(t, securityKeyPage(true), map[string]pageHandler{}, &creds.LoginDetails{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "Failed to prompt for second factor")
	})

	t.Run("page without key handles", func(t *testing.T) {
		// Regression: this used to panic with an index out of range.
		withDeviceFinder(t, &scriptedDeviceFinder{err: errNoDeviceFound})
		require.NotPanics(t, func() {
			_, _, err := runChallenge(t, securityKeyPage(false), map[string]pageHandler{}, &creds.LoginDetails{})
			require.Error(t, err)
			assert.Contains(t, err.Error(), "no U2F key handles")
		})
	})
}

func TestChallenge_UnsupportedSecondFactor(t *testing.T) {
	t.Run("skips to a supported challenge", func(t *testing.T) {
		chooser := `<h2>To sign in to your Google Account, choose a task from the list below.</h2>` +
			`<form action="/signin/challenge/totp/9" data-challengeentry="9"><input name="challengeId" value="9"></form>`
		f, got, err := runChallenge(t, challengePage("/signin/challenge/carrier-pigeon/1", ""), map[string]pageHandler{
			"/signin/challenge/skip":   static(chooser),
			"/signin/challenge/totp/9": static(samlPage),
		}, &creds.LoginDetails{MFAToken: "999999"})
		require.NoError(t, err)
		assert.Equal(t, testSAMLResponse, got)

		skips := f.formsFor("/signin/challenge/skip")
		require.Len(t, skips, 1)
		assert.Equal(t, "7", skips[0].Get("challengeId"))
		assert.Equal(t, "9", f.formsFor("/signin/challenge/totp/9")[0].Get("challengeId"))
	})

	t.Run("no skip form", func(t *testing.T) {
		page := `<html><body>` + twoStepHeader + `<form id="challenge" action="/signin/challenge/carrier-pigeon/1"></form></body></html>`
		_, _, err := runChallenge(t, page, map[string]pageHandler{}, &creds.LoginDetails{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unable to extract skip form")
	})
}

func TestExtractInputsByFormID(t *testing.T) {
	t.Run("selected form without action uses the document URL and only its own fields", func(t *testing.T) {
		// Regression: a matching form without an action attribute used to panic, and then
		// fell back to every form, merging their fields and posting to another form's action.
		doc := mustDoc(t, `<form id="challenge"><input name="a" value="1"></form>`+
			`<form action="/signin/next"><input name="b" value="2"></form>`, "https://accounts.google.com/page")
		var (
			form   url.Values
			action string
			err    error
		)
		require.NotPanics(t, func() { form, action, err = extractInputsByFormID(doc, "challenge") })
		require.NoError(t, err)
		assert.Equal(t, "https://accounts.google.com/page", action)
		assert.Equal(t, "1", form.Get("a"))
		assert.NotContains(t, form, "b")
	})

	t.Run("selected form keeps its explicit action", func(t *testing.T) {
		doc := mustDoc(t, `<form id="challenge" action="/signin/mine"><input name="a" value="1"></form>`+
			`<form action="/signin/next"><input name="b" value="2"></form>`, "https://accounts.google.com/page")
		form, action, err := extractInputsByFormID(doc, "challenge")
		require.NoError(t, err)
		assert.Equal(t, "https://accounts.google.com/signin/mine", action)
		assert.Equal(t, "1", form.Get("a"))
		assert.NotContains(t, form, "b")
	})

	t.Run("no form with the given id falls back to any form", func(t *testing.T) {
		doc := mustDoc(t, `<form action="/signin/next"><input name="b" value="2"></form>`, "https://accounts.google.com/page")
		form, action, err := extractInputsByFormID(doc, "challenge")
		require.NoError(t, err)
		assert.Equal(t, "https://accounts.google.com/signin/next", action)
		assert.Equal(t, "2", form.Get("b"))
	})

	t.Run("document without URL returns an error", func(t *testing.T) {
		doc, err := goquery.NewDocumentFromReader(strings.NewReader(`<form id="challenge"><input name="a" value="1"></form>`))
		require.NoError(t, err)
		require.NotPanics(t, func() { _, _, err = extractInputsByFormQuery(doc, "#challenge") })
		require.Error(t, err)
	})

	t.Run("no forms", func(t *testing.T) {
		doc := mustDoc(t, `<p>none</p>`, "https://accounts.google.com/page")
		_, _, err := extractInputsByFormID(doc, "challenge")
		require.Error(t, err)
	})
}

func TestGenerateFullURLIfRelative(t *testing.T) {
	tests := []struct {
		name        string
		destination string
		current     string
		want        string
	}{
		{name: "relative path", destination: "/signin/x", current: "https://accounts.google.com/a/b?c=d", want: "https://accounts.google.com/signin/x"},
		{name: "absolute URL", destination: "https://other.example.com/x", current: "https://accounts.google.com/", want: "https://other.example.com/x"},
		{name: "empty destination", destination: "", current: "https://accounts.google.com/", want: ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := generateFullURLIfRelative(tc.destination, tc.current)
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}

	_, err := generateFullURLIfRelative("/x", "://bad")
	require.Error(t, err)
}

func TestExtractKeyHandles(t *testing.T) {
	doc := mustDoc(t, `<div jsname="C0oDBd" data-challenge-ui='prefix {"1":[null,["`+testKeyHandle+`","`+testChallenge+`",`+
		`"{\"appid\":\"app-1\"}",42,[["b3RoZXI="]]]],"2":"scalar"} suffix'></div>`, "https://accounts.google.com/")

	appID, keyHandles := extractKeyHandles(doc, testChallenge)
	assert.Equal(t, "app-1", appID)
	// The challenge itself is valid base64 but must not be treated as a key handle.
	assert.Equal(t, []string{testKeyHandle, "b3RoZXI="}, keyHandles)
}

func TestIsAppID(t *testing.T) {
	assert.Equal(t, "app", isAppId(`{"appid":"app"}`))
	assert.Empty(t, isAppId(`{"appid":42}`))
	assert.Empty(t, isAppId(`not json`))
}
