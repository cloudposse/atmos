package browser

import (
	"errors"
	"net/url"
	"path/filepath"
	"testing"

	"github.com/mxschmitt/playwright-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/cloudposse/atmos/pkg/auth/providers/aws/saml2aws/cfg"
	"github.com/cloudposse/atmos/pkg/auth/providers/aws/saml2aws/creds"
)

const (
	testIdPURL    = "https://idp.example.com/login"
	testSigninURL = "https://signin.aws.amazon.com/saml"
	testAssertion = "PHNhbWxwOlJlc3BvbnNlPjwvc2FtbHA6UmVzcG9uc2U+"
)

var errTest = errors.New("boom")

func samlPostBody(assertion string) string {
	params := url.Values{}
	params.Add("RelayState", "state")
	params.Add("SAMLResponse", assertion)
	return params.Encode()
}

func newTestClient(t *testing.T) *Client {
	t.Helper()
	client, err := New(&cfg.IDPAccount{Headless: true, Timeout: 100000})
	require.NoError(t, err)
	return client
}

func TestNew(t *testing.T) {
	client, err := New(&cfg.IDPAccount{
		Headless:              true,
		BrowserDriverDir:      "/drivers",
		BrowserType:           "Firefox",
		BrowserExecutablePath: "/usr/bin/firefox",
		Timeout:               42,
		BrowserAutoFill:       true,
	})
	require.NoError(t, err)
	assert.Equal(t, &Client{
		Headless:              true,
		BrowserDriverDir:      "/drivers",
		BrowserType:           "firefox",
		BrowserExecutablePath: "/usr/bin/firefox",
		Timeout:               42,
		BrowserAutoFill:       true,
	}, client)
}

func TestValidate(t *testing.T) {
	client := newTestClient(t)

	require.NoError(t, client.Validate(&creds.LoginDetails{URL: testIdPURL}))
	require.ErrorContains(t, client.Validate(&creds.LoginDetails{}), "empty URL")
}

func TestValidateBrowserType(t *testing.T) {
	for _, browserType := range append([]string{""}, validBrowserTypes...) {
		t.Run("valid "+browserType, func(t *testing.T) {
			require.NoError(t, (&Client{BrowserType: browserType}).validateBrowserType())
		})
	}

	err := (&Client{BrowserType: "invalid"}).validateBrowserType()
	require.ErrorContains(t, err, "invalid browser-type: 'invalid', only [chromium firefox webkit chrome chrome-beta chrome-dev chrome-canary msedge msedge-beta msedge-dev msedge-canary] are allowed")
}

// An invalid browser type must fail before Playwright is installed or started.
func TestAuthenticateRejectsInvalidBrowserTypeBeforeStartingPlaywright(t *testing.T) {
	client := &Client{BrowserType: "invalid", BrowserDriverDir: t.TempDir()}

	_, err := client.Authenticate(&creds.LoginDetails{URL: testIdPURL, DownloadBrowser: true})
	require.ErrorContains(t, err, "invalid browser-type")
}

// Without an installed driver and without download, Run fails with a clear message (no network needed).
func TestAuthenticateWithoutDriverFails(t *testing.T) {
	client := &Client{Headless: true, BrowserDriverDir: t.TempDir()}

	_, err := client.Authenticate(&creds.LoginDetails{URL: testIdPURL})
	require.ErrorContains(t, err, "please install the driver")
}

func TestSigninRegex(t *testing.T) {
	regex, err := signinRegex()
	require.NoError(t, err)

	for _, u := range []string{
		"https://signin.aws.amazon.com/saml",
		"https://us-east-1.signin.aws.amazon.com/saml",
		"https://signin.amazonaws-us-gov.com/saml",
		"https://signin.amazonaws.cn/saml",
	} {
		assert.True(t, regex.MatchString(u), u)
	}
	for _, u := range []string{"https://google.com/", "http://signin.aws.amazon.com/saml", "https://signin.aws.amazon.com/console"} {
		assert.False(t, regex.MatchString(u), u)
	}
}

func TestExpectRequestOptions(t *testing.T) {
	options := (&Client{Timeout: 100000}).expectRequestTimeout()
	assert.InDelta(t, float64(100000), *options.Timeout, 0)
}

func TestExpectRequestOptionsDefaultTimeout(t *testing.T) {
	options := (&Client{Timeout: 1000}).expectRequestTimeout()
	assert.InDelta(t, DEFAULT_TIMEOUT, *options.Timeout, 0)
}

func TestStorageStatePath(t *testing.T) {
	path, err := storageStatePath()
	require.NoError(t, err)
	assert.True(t, filepath.IsAbs(path), path)
	assert.Equal(t, filepath.Join(".aws", "saml2aws", "storageState.json"), filepath.Join(filepath.Base(filepath.Dir(filepath.Dir(path))), filepath.Base(filepath.Dir(path)), filepath.Base(path)))
}

// The SAML response is captured by ExpectRequest when the callback has not fired yet.
func TestGetSAMLResponseViaExpectRequest(t *testing.T) {
	ctrl := gomock.NewController(t)
	page := NewMocksamlPage(ctrl)
	req := NewMockRequest(ctrl)
	client := newTestClient(t)

	page.EXPECT().OnRequest(gomock.Any())
	page.EXPECT().Goto(testIdPURL).Return(nil, nil)
	page.EXPECT().ExpectRequest(gomock.Any(), gomock.Any(), client.expectRequestTimeout()).Return(req, nil)
	req.EXPECT().PostData().Return(samlPostBody(testAssertion), nil)

	got, err := getSAMLResponse(page, &creds.LoginDetails{URL: testIdPURL}, client)
	require.NoError(t, err)
	assert.Equal(t, testAssertion, got)
}

// The SAML response is captured by the OnRequest callback, and ExpectRequest is not needed.
func TestGetSAMLResponseViaOnRequestCallback(t *testing.T) {
	ctrl := gomock.NewController(t)
	page := NewMocksamlPage(ctrl)
	req := NewMockRequest(ctrl)
	client := newTestClient(t)

	var handler func(playwright.Request)
	page.EXPECT().OnRequest(gomock.Any()).Do(func(fn func(playwright.Request)) { handler = fn })
	page.EXPECT().Goto(testIdPURL).DoAndReturn(func(string, ...playwright.PageGotoOptions) (playwright.Response, error) {
		handler(req)
		return nil, nil
	})
	req.EXPECT().URL().Return(testSigninURL)
	req.EXPECT().PostData().Return(samlPostBody(testAssertion), nil)

	got, err := getSAMLResponse(page, &creds.LoginDetails{URL: testIdPURL}, client)
	require.NoError(t, err)
	assert.Equal(t, testAssertion, got)
}

// Requests to other URLs are ignored by the callback.
func TestGetSAMLResponseCallbackIgnoresOtherURLs(t *testing.T) {
	ctrl := gomock.NewController(t)
	page := NewMocksamlPage(ctrl)
	other := NewMockRequest(ctrl)
	signin := NewMockRequest(ctrl)
	client := newTestClient(t)

	var handler func(playwright.Request)
	page.EXPECT().OnRequest(gomock.Any()).Do(func(fn func(playwright.Request)) { handler = fn })
	page.EXPECT().Goto(testIdPURL).DoAndReturn(func(string, ...playwright.PageGotoOptions) (playwright.Response, error) {
		handler(other)
		return nil, nil
	})
	other.EXPECT().URL().Return("https://idp.example.com/assets/app.js")
	page.EXPECT().ExpectRequest(gomock.Any(), gomock.Any(), gomock.Any()).Return(signin, nil)
	signin.EXPECT().PostData().Return(samlPostBody(testAssertion), nil)

	got, err := getSAMLResponse(page, &creds.LoginDetails{URL: testIdPURL}, client)
	require.NoError(t, err)
	assert.Equal(t, testAssertion, got)
}

// Regression: upstream dereferenced the nil request after an ExpectRequest timeout and panicked.
func TestGetSAMLResponseExpectRequestTimeoutDoesNotPanic(t *testing.T) {
	ctrl := gomock.NewController(t)
	page := NewMocksamlPage(ctrl)
	client := newTestClient(t)

	page.EXPECT().OnRequest(gomock.Any())
	page.EXPECT().Goto(testIdPURL).Return(nil, nil)
	page.EXPECT().ExpectRequest(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, errTest)

	var err error
	require.NotPanics(t, func() {
		_, err = getSAMLResponse(page, &creds.LoginDetails{URL: testIdPURL}, client)
	})
	require.ErrorIs(t, err, errTest)
	assert.ErrorContains(t, err, "waiting for the AWS sign-in request")
}

func TestGetSAMLResponseExpectRequestNilRequest(t *testing.T) {
	ctrl := gomock.NewController(t)
	page := NewMocksamlPage(ctrl)
	client := newTestClient(t)

	page.EXPECT().OnRequest(gomock.Any())
	page.EXPECT().Goto(testIdPURL).Return(nil, nil)
	page.EXPECT().ExpectRequest(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, nil)

	_, err := getSAMLResponse(page, &creds.LoginDetails{URL: testIdPURL}, client)
	require.ErrorIs(t, err, ErrNoSAMLResponse)
}

// Regression: upstream returned the wrong error variable, so a PostData failure was swallowed.
func TestGetSAMLResponsePostDataError(t *testing.T) {
	ctrl := gomock.NewController(t)
	page := NewMocksamlPage(ctrl)
	req := NewMockRequest(ctrl)
	client := newTestClient(t)

	page.EXPECT().OnRequest(gomock.Any())
	page.EXPECT().Goto(testIdPURL).Return(nil, nil)
	page.EXPECT().ExpectRequest(gomock.Any(), gomock.Any(), gomock.Any()).Return(req, nil)
	req.EXPECT().PostData().Return("", errTest)

	_, err := getSAMLResponse(page, &creds.LoginDetails{URL: testIdPURL}, client)
	require.ErrorIs(t, err, errTest)
}

func TestGetSAMLResponseGotoError(t *testing.T) {
	ctrl := gomock.NewController(t)
	page := NewMocksamlPage(ctrl)
	client := newTestClient(t)

	page.EXPECT().OnRequest(gomock.Any())
	page.EXPECT().Goto(testIdPURL).Return(nil, errTest)

	_, err := getSAMLResponse(page, &creds.LoginDetails{URL: testIdPURL}, client)
	require.ErrorIs(t, err, errTest)
}

func TestGetSAMLResponseMissingSAMLResponseField(t *testing.T) {
	ctrl := gomock.NewController(t)
	page := NewMocksamlPage(ctrl)
	req := NewMockRequest(ctrl)
	client := newTestClient(t)

	page.EXPECT().OnRequest(gomock.Any())
	page.EXPECT().Goto(testIdPURL).Return(nil, nil)
	page.EXPECT().ExpectRequest(gomock.Any(), gomock.Any(), gomock.Any()).Return(req, nil)
	req.EXPECT().PostData().Return("RelayState=state", nil)

	_, err := getSAMLResponse(page, &creds.LoginDetails{URL: testIdPURL}, client)
	require.ErrorIs(t, err, ErrNoSAMLResponse)
}

func TestGetSAMLResponseMalformedBody(t *testing.T) {
	ctrl := gomock.NewController(t)
	page := NewMocksamlPage(ctrl)
	req := NewMockRequest(ctrl)
	client := newTestClient(t)

	page.EXPECT().OnRequest(gomock.Any())
	page.EXPECT().Goto(testIdPURL).Return(nil, nil)
	page.EXPECT().ExpectRequest(gomock.Any(), gomock.Any(), gomock.Any()).Return(req, nil)
	req.EXPECT().PostData().Return("%zz", nil)

	_, err := getSAMLResponse(page, &creds.LoginDetails{URL: testIdPURL}, client)
	require.Error(t, err)
}

// Auto-fill only applies to a full playwright.Page; the narrow mock page is skipped without error.
func TestGetSAMLResponseAutoFillSkippedForNarrowPage(t *testing.T) {
	ctrl := gomock.NewController(t)
	page := NewMocksamlPage(ctrl)
	req := NewMockRequest(ctrl)
	client := newTestClient(t)
	client.BrowserAutoFill = true

	original := autoFill
	t.Cleanup(func() { autoFill = original })
	autoFill = func(playwright.Page, *creds.LoginDetails) error {
		t.Fatal("autoFill must not be called for a page that is not a playwright.Page")
		return nil
	}

	page.EXPECT().OnRequest(gomock.Any())
	page.EXPECT().Goto(testIdPURL).Return(nil, nil)
	page.EXPECT().ExpectRequest(gomock.Any(), gomock.Any(), gomock.Any()).Return(req, nil)
	req.EXPECT().PostData().Return(samlPostBody(testAssertion), nil)

	got, err := getSAMLResponse(page, &creds.LoginDetails{URL: testIdPURL}, client)
	require.NoError(t, err)
	assert.Equal(t, testAssertion, got)
}

func TestContains(t *testing.T) {
	assert.True(t, contains([]string{"a", "b"}, "b"))
	assert.False(t, contains([]string{"a", "b"}, "c"))
	assert.False(t, contains(nil, "a"))
}

// Regression: the sign-in request can reach the OnRequest callback after the "already
// captured" check but before ExpectRequest starts listening. ExpectRequest then never sees
// it and times out; upstream failed (or panicked) although the SAML response was captured.
func TestGetSAMLResponseUsesCallbackCaptureWhenExpectRequestMissesIt(t *testing.T) {
	ctrl := gomock.NewController(t)
	page := NewMocksamlPage(ctrl)
	req := NewMockRequest(ctrl)
	client := newTestClient(t)

	var handler func(playwright.Request)
	page.EXPECT().OnRequest(gomock.Any()).Do(func(fn func(playwright.Request)) { handler = fn })
	page.EXPECT().Goto(testIdPURL).Return(nil, nil)
	page.EXPECT().ExpectRequest(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
		func(any, func() error, ...playwright.PageExpectRequestOptions) (playwright.Request, error) {
			handler(req)        // Delivered to the OnRequest callback only.
			return nil, errTest // The waiter never saw it and timed out.
		})
	req.EXPECT().URL().Return(testSigninURL)
	req.EXPECT().PostData().Return(samlPostBody(testAssertion), nil)

	got, err := getSAMLResponse(page, &creds.LoginDetails{URL: testIdPURL}, client)
	require.NoError(t, err)
	assert.Equal(t, testAssertion, got)
}

// Regression: a later request to the sign-in URL without a SAMLResponse (for example a GET) must not
// replace an assertion that was already captured.
func TestGetSAMLResponseKeepsCapturedAssertionOnLaterEmptyRequest(t *testing.T) {
	ctrl := gomock.NewController(t)
	page := NewMocksamlPage(ctrl)
	post := NewMockRequest(ctrl)
	get := NewMockRequest(ctrl)
	client := newTestClient(t)

	var handler func(playwright.Request)
	page.EXPECT().OnRequest(gomock.Any()).Do(func(fn func(playwright.Request)) { handler = fn })
	page.EXPECT().Goto(testIdPURL).DoAndReturn(func(string, ...playwright.PageGotoOptions) (playwright.Response, error) {
		handler(post)
		handler(get)
		return nil, nil
	})
	post.EXPECT().URL().Return(testSigninURL)
	post.EXPECT().PostData().Return(samlPostBody(testAssertion), nil)
	get.EXPECT().URL().Return(testSigninURL)
	get.EXPECT().PostData().Return("", nil)

	got, err := getSAMLResponse(page, &creds.LoginDetails{URL: testIdPURL}, client)
	require.NoError(t, err)
	assert.Equal(t, testAssertion, got)
}

// The callback passed to ExpectRequest runs once the waiter listens. It lets the wait continue
// while nothing was captured, and ends it as soon as the OnRequest callback captured the request.
func TestGetSAMLResponseExpectRequestCallbackEndsWaitWhenCaptured(t *testing.T) {
	ctrl := gomock.NewController(t)
	page := NewMocksamlPage(ctrl)
	req := NewMockRequest(ctrl)
	client := newTestClient(t)

	var handler func(playwright.Request)
	page.EXPECT().OnRequest(gomock.Any()).Do(func(fn func(playwright.Request)) { handler = fn })
	page.EXPECT().Goto(testIdPURL).Return(nil, nil)
	page.EXPECT().ExpectRequest(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ any, cb func() error, _ ...playwright.PageExpectRequestOptions) (playwright.Request, error) {
			require.NotNil(t, cb)
			require.NoError(t, cb(), "nothing captured yet, keep waiting")
			handler(req)
			err := cb()
			require.ErrorIs(t, err, errSAMLRequestCaptured)
			return nil, err // playwright-go's RunAndWait returns the callback error.
		})
	req.EXPECT().URL().Return(testSigninURL)
	req.EXPECT().PostData().Return(samlPostBody(testAssertion), nil)

	got, err := getSAMLResponse(page, &creds.LoginDetails{URL: testIdPURL}, client)
	require.NoError(t, err)
	assert.Equal(t, testAssertion, got)
}
