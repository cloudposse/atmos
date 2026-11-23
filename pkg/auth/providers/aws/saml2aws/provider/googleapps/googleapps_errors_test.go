package googleapps

import (
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"testing/iotest"

	"github.com/PuerkitoBio/goquery"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/cloudposse/atmos/pkg/auth/providers/aws/saml2aws/creds"
	"github.com/cloudposse/atmos/pkg/auth/providers/aws/saml2aws/provider"
)

var errBodyRead = errors.New("connection reset while reading the body")

// brokenBodyTransport answers every request with a 200 whose body fails while being read.
type brokenBodyTransport struct{}

func (brokenBodyTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	return &http.Response{
		StatusCode: http.StatusOK,
		Status:     "200 OK",
		Header:     http.Header{},
		Body:       io.NopCloser(iotest.ErrReader(errBodyRead)),
		Request:    req,
	}, nil
}

func newBrokenBodyClient(t *testing.T) *Client {
	t.Helper()

	httpClient, err := provider.NewHTTPClient(brokenBodyTransport{}, &provider.HTTPClientOptions{})
	require.NoError(t, err)
	return &Client{client: httpClient}
}

// invalidURL cannot be parsed, so building a request for it fails before anything is sent.
const invalidURL = "https://idp.example.com\x7f/signin"

// pageLoaders are the steps that request a page and parse its HTML. Each returns the error of the step.
var pageLoaders = []struct {
	name           string
	call           func(kc *Client, target string) error
	wantBuildErr   string
	wantRequestErr string
	wantParseErr   string
}{
	{
		name: "loadFirstPage",
		call: func(kc *Client, target string) error {
			_, _, err := kc.loadFirstPage(&creds.LoginDetails{URL: target})
			return err
		},
		wantBuildErr:   "error retrieving login form from idp",
		wantRequestErr: "failed to make request to login form",
		wantParseErr:   "error parsing first page html document",
	},
	{
		name: "loadLoginPage",
		call: func(kc *Client, target string) error {
			_, _, err := kc.loadLoginPage(target, "https://idp.example.com", url.Values{})
			return err
		},
		wantBuildErr:   "error retrieving login form",
		wantRequestErr: "failed to make request to login form",
		wantParseErr:   "error parsing login page html document",
	},
	{
		name: "loadChallengePage",
		call: func(kc *Client, target string) error {
			_, err := kc.loadChallengePage(target, "https://idp.example.com", url.Values{}, &creds.LoginDetails{})
			return err
		},
		wantBuildErr:   "error retrieving login form",
		wantRequestErr: "failed to make request to login form",
		wantParseErr:   "error parsing login page html document",
	},
	{
		name: "loadAlternateChallengePage",
		call: func(kc *Client, target string) error {
			_, err := kc.loadAlternateChallengePage(target, "https://idp.example.com", url.Values{}, &creds.LoginDetails{})
			return err
		},
		wantBuildErr:   "error retrieving login form",
		wantRequestErr: "failed to make request to login form",
		wantParseErr:   "error parsing login page html document",
	},
	{
		name: "loadResponsePage",
		call: func(kc *Client, target string) error {
			_, err := kc.loadResponsePage(target, "https://idp.example.com", url.Values{})
			return err
		},
		wantBuildErr:   "error retrieving response page",
		wantRequestErr: "failed to make request to login form",
		wantParseErr:   "error parsing login page html document",
	},
}

func TestPageLoadersReportRequestAndParseFailures(t *testing.T) {
	for _, loader := range pageLoaders {
		t.Run(loader.name+" invalid URL", func(t *testing.T) {
			err := loader.call(&Client{client: mustHTTPClient(t)}, invalidURL)
			require.Error(t, err)
			assert.Contains(t, err.Error(), loader.wantBuildErr)
		})

		t.Run(loader.name+" unreachable server", func(t *testing.T) {
			err := loader.call(&Client{client: mustHTTPClient(t)}, closedServerURL(t)+"/signin")
			require.Error(t, err)
			assert.Contains(t, err.Error(), loader.wantRequestErr)
		})

		t.Run(loader.name+" unreadable body", func(t *testing.T) {
			err := loader.call(newBrokenBodyClient(t), "https://idp.example.com/signin")
			require.ErrorIs(t, err, errBodyRead)
			assert.Contains(t, err.Error(), loader.wantParseErr)
		})
	}
}

func TestPostJSONFailures(t *testing.T) {
	t.Run("invalid URL", func(t *testing.T) {
		res, err := (&Client{client: mustHTTPClient(t)}).postJSON(invalidURL, map[string]string{"txId": "1"}, "ref")
		if res != nil {
			_ = res.Body.Close()
		}
		require.Error(t, err)
		assert.Contains(t, err.Error(), "error retrieving login form")
	})

	t.Run("unreachable server", func(t *testing.T) {
		res, err := (&Client{client: mustHTTPClient(t)}).postJSON(closedServerURL(t)+"/await", map[string]string{"txId": "1"}, "ref")
		if res != nil {
			_ = res.Body.Close()
		}
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to post JSON")
	})
}

func TestDefaultU2FDeviceFinderIsHardwareBacked(t *testing.T) {
	assert.IsType(t, &U2FDeviceFinder{}, newU2FDeviceFinder())
}

// captchaFlow builds a password-flow server whose password step answers with page, and a client for it.
func captchaFlow(t *testing.T, page string, extra map[string]pageHandler) (*fakeGoogle, *Client, *creds.LoginDetails) {
	t.Helper()

	t.Setenv("TERM_PROGRAM", "Apple_Terminal")
	routes := passwordFlowRoutes(t, static(page))
	for path, h := range extra {
		routes[path] = h
	}
	f := newFakeGoogle(t, routes)
	kc, loginDetails := f.client()
	return f, kc, loginDetails
}

func TestAuthenticate_CaptchaFailures(t *testing.T) {
	const captchaInput = `<input type="text" id="logincaptcha" name="logincaptcha" value="">`

	t.Run("captcha image without src", func(t *testing.T) {
		withMockPrompter(t) // No prompt is expected.
		page := `<html><body><form id="challenge" action="/next">` + captchaInput +
			`<div class="captcha-img"><img alt="captcha"></div></form></body></html>`
		_, kc, loginDetails := captchaFlow(t, page, nil)

		_, err := kc.Authenticate(loginDetails)
		require.ErrorIs(t, err, errCaptchaImageNotFound)
	})

	t.Run("prompt failure stops the flow", func(t *testing.T) {
		errCancelled := errors.New("prompt cancelled")
		pr := withMockPrompter(t)
		pr.EXPECT().String("Captcha", "").Return("", errCancelled).Times(1)
		page := `<html><body><form id="challenge" action="/next">` + captchaInput +
			`<div class="captcha-img"><img src="/Captcha"></div></form></body></html>`
		f, kc, loginDetails := captchaFlow(t, page, nil)

		_, err := kc.Authenticate(loginDetails)
		require.ErrorIs(t, err, errCancelled)
		assert.Zero(t, f.count("/next"), "the captcha form must not be submitted after the prompt failed")
	})

	t.Run("captcha page without a form", func(t *testing.T) {
		pr := withMockPrompter(t)
		pr.EXPECT().String("Captcha", "").Return("answer", nil).Times(1)
		page := `<html><body>` + captchaInput + `<div class="captcha-img"><img src="/Captcha"></div></body></html>`
		_, kc, loginDetails := captchaFlow(t, page, nil)

		_, err := kc.Authenticate(loginDetails)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "error extracting captcha")
	})

	t.Run("captcha submission fails", func(t *testing.T) {
		pr := withMockPrompter(t)
		pr.EXPECT().String("Captcha", "").Return("answer", nil).Times(1)
		page := `<html><body><form id="challenge" action="` + closedServerURL(t) + `/next">` + captchaInput +
			`<div class="captcha-img"><img src="/Captcha"></div></form></body></html>`
		_, kc, loginDetails := captchaFlow(t, page, nil)

		_, err := kc.Authenticate(loginDetails)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "error loading challenge page")
	})
}

func TestAuthenticate_PasswordPageAfterCaptchaFailures(t *testing.T) {
	t.Run("page without a form", func(t *testing.T) {
		page := `<html><body><input id="password" name="Passwd"></body></html>`
		_, kc, loginDetails := captchaFlow(t, page, nil)

		_, err := kc.Authenticate(loginDetails)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "error parsing password page after captcha")
	})

	t.Run("password submission fails", func(t *testing.T) {
		page := `<html><body><form id="challenge" action="` + closedServerURL(t) + `/pwd">` +
			`<input id="password" name="Passwd" value=""></form></body></html>`
		_, kc, loginDetails := captchaFlow(t, page, nil)

		_, err := kc.Authenticate(loginDetails)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "error loading challenge page")
	})
}

func TestChallenge_PromptAndFormFailures(t *testing.T) {
	errCancelled := errors.New("prompt cancelled")
	chooserEntry := func(action string) string {
		return `<h2>To sign in to your Google Account, choose a task from the list below.</h2>` +
			`<form action="` + action + `" data-challengeentry="1"><input name="challengeId" value="1"></form>`
	}

	tests := []struct {
		name    string
		page    func(t *testing.T) string
		routes  map[string]pageHandler
		setup   func(t *testing.T)
		wantErr string
	}{
		{
			name:    "2-step page without any form",
			page:    func(*testing.T) string { return `<html><body>` + twoStepHeader + `</body></html>` },
			wantErr: "unable to extract challenge form",
		},
		{
			name: "TOTP prompt failure",
			page: func(*testing.T) string { return challengePage("/signin/challenge/totp/2", "") },
			setup: func(t *testing.T) {
				withMockPrompter(t).EXPECT().RequestSecurityCode("000000").Return("", errCancelled).Times(1)
			},
			wantErr: "failed to read the TOTP code",
		},
		{
			name: "SMS request fails",
			page: func(t *testing.T) string {
				return challengePage(closedServerURL(t)+"/signin/challenge/ipp/2", `<button>Send text message</button>`)
			},
			wantErr: "failed to post sms request form",
		},
		{
			name: "SMS request answered without a form",
			page: func(*testing.T) string {
				return challengePage("/signin/challenge/ipp/2", `<button>Send text message</button>`)
			},
			routes:  map[string]pageHandler{"/signin/challenge/ipp/2": static(`<html><body>sent</body></html>`)},
			wantErr: "unable to extract challenge form",
		},
		{
			name: "SMS token prompt failure",
			page: func(*testing.T) string { return challengePage("/signin/challenge/ipp/3", "") },
			setup: func(t *testing.T) {
				withMockPrompter(t).EXPECT().StringRequired("Enter SMS token: G-").Return("", errCancelled).Times(1)
			},
			wantErr: "failed to read the SMS token",
		},
		{
			name: "security key one-time code prompt failure",
			page: func(*testing.T) string { return challengePage("/signin/challenge/skotp/4", "") },
			setup: func(t *testing.T) {
				withMockPrompter(t).EXPECT().RequestSecurityCode("000 000").Return("", errCancelled).Times(1)
			},
			wantErr: "failed to read the one-time code",
		},
		{
			name:    "security key challenge with an unparsable action",
			page:    func(*testing.T) string { return challengePage("https://[::1/signin/challenge/sk/5", "") },
			wantErr: "unable to parse action URL for U2F challenge",
		},
		{
			name: "challenge chooser entry cannot be submitted",
			page: func(*testing.T) string { return chooserEntry("/signin/challenge/totp/1") },
			routes: map[string]pageHandler{
				"/signin/challenge/totp/1": static(`<html><body>` + twoStepHeader + `</body></html>`),
			},
			wantErr: "unable to extract challenge form",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.setup != nil {
				tt.setup(t)
			}
			routes := tt.routes
			if routes == nil {
				routes = map[string]pageHandler{}
			}
			_, _, err := runChallenge(t, tt.page(t), routes, &creds.LoginDetails{})
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func TestChallenge_SecurityKeyWithoutDeviceReportsFactorFailure(t *testing.T) {
	withDeviceFinder(t, failingFinder(gomock.NewController(t), errNoDeviceFound, 1))
	_, _, err := runChallenge(t, withoutSkipForm(securityKeyPage(true)), map[string]pageHandler{}, &creds.LoginDetails{})
	require.ErrorIs(t, err, errNoDeviceFound)
}

func TestLoadChallengeEntryPageWithoutDocumentURL(t *testing.T) {
	kc := &Client{client: mustHTTPClient(t)}
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(
		`<form action="/signin/challenge/totp/1" data-challengeentry="1"><input name="challengeId" value="1"></form>`))
	require.NoError(t, err)
	require.Nil(t, doc.Url)

	_, err = kc.loadChallengeEntryPage(doc, "https://idp.example.com/x", &creds.LoginDetails{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unable to extract challenge form")
	assert.Contains(t, err.Error(), "document has no URL")
}

func TestExtractInputsFormErrors(t *testing.T) {
	t.Run("form selected by id without a document URL", func(t *testing.T) {
		doc, err := goquery.NewDocumentFromReader(strings.NewReader(`<form id="challenge" action="/next"><input name="a" value="b"></form>`))
		require.NoError(t, err)

		_, _, err = extractInputsByFormID(doc, "challenge")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "document has no URL")
	})

	t.Run("relative action against an unparsable document URL", func(t *testing.T) {
		doc, err := goquery.NewDocumentFromReader(strings.NewReader(`<form id="challenge" action="/next"></form>`))
		require.NoError(t, err)
		// Built by hand: url.Parse would reject this host.
		doc.Url = &url.URL{Scheme: "https", Host: "idp.example.com\x7f", Path: "/signin"}

		formData, actionURL, err := extractInputsByFormQuery(doc, "#challenge")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "error getting action URL")
		assert.Empty(t, actionURL)
		assert.Empty(t, formData)
	})
}
