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
	// An external driver configured in the developer's environment would otherwise be used instead.
	t.Setenv("PLAYWRIGHT_CLI_PATH", "")
	t.Setenv("PLAYWRIGHT_NODEJS_PATH", "")
	client := &Client{Headless: true, BrowserDriverDir: t.TempDir()}

	_, err := client.Authenticate(&creds.LoginDetails{URL: testIdPURL})
	require.ErrorContains(t, err, "please install the driver")
}

func TestIsSigninURL(t *testing.T) {
	for _, u := range []string{
		"https://signin.aws.amazon.com/saml",
		"https://us-east-1.signin.aws.amazon.com/saml",
		"https://signin.amazonaws-us-gov.com/saml",
		"https://us-gov-west-1.signin.amazonaws-us-gov.com/saml",
		"https://signin.amazonaws.cn/saml",
		"https://signin.aws.amazon.com:443/saml",
		"https://signin.aws.amazon.com/saml?RelayState=x",
		// Backward compatibility: per-provider ACS endpoints used by IAM SAML providers.
		"https://signin.aws.amazon.com/saml/acs/SAMLSP4SH0A1EXAMPLE",
		"https://us-east-1.signin.aws.amazon.com/saml/acs/SAMLSP4SH0A1EXAMPLE",
	} {
		assert.True(t, isSigninURL(u), u)
	}
	for _, u := range []string{
		"https://google.com/",
		"http://signin.aws.amazon.com/saml",
		"https://signin.aws.amazon.com/console",
		"https://signin.aws.amazon.com/samlx",
		"https://signin.aws.amazon.com:8443/saml",
		// Regression: AWS sign-in text inside another request must not match.
		"https://idp.example.com/track?next=https://signin.aws.amazon.com/saml",
		"https://idp.example.com/https://signin.aws.amazon.com/saml",
		"https://signin.aws.amazon.com.evil.example/saml",
		"https://evil.example/#https://signin.aws.amazon.com/saml",
		"https://signin.aws.amazon.com@evil.example/saml",
		"%zz",
	} {
		assert.False(t, isSigninURL(u), u)
	}
}

// Regression: the ExpectRequest predicate only accepts a sign-in request that carries a SAMLResponse.
func TestSigninRequestWithAssertion(t *testing.T) {
	tests := []struct {
		name     string
		url      string
		postData string
		postErr  error
		want     bool
	}{
		{name: "assertion", url: testSigninURL, postData: samlPostBody(testAssertion), want: true},
		{name: "relay state only", url: testSigninURL, postData: "RelayState=state"},
		{name: "empty body", url: testSigninURL},
		{name: "body read error ends the wait to report it", url: testSigninURL, postErr: errTest, want: true},
		{name: "other URL", url: "https://idp.example.com/saml"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			req := NewMockRequest(ctrl)
			req.EXPECT().URL().Return(tt.url)
			if tt.url == testSigninURL {
				req.EXPECT().PostData().Return(tt.postData, tt.postErr)
			}
			assert.Equal(t, tt.want, signinRequestWithAssertion(req))
		})
	}
}

// Regression: a sign-in request without a SAMLResponse recorded before the waiter starts must not count as
// captured, or the valid assertion that arrives later is never waited for.
func TestGetSAMLResponseWaitsPastUnusableCapture(t *testing.T) {
	ctrl := gomock.NewController(t)
	page := NewMocksamlPage(ctrl)
	early := NewMockRequest(ctrl)
	valid := NewMockRequest(ctrl)
	client := newTestClient(t)

	var handler func(playwright.Request)
	page.EXPECT().OnRequest(gomock.Any()).Do(func(fn func(playwright.Request)) { handler = fn })
	page.EXPECT().Goto(testIdPURL).DoAndReturn(func(string, ...playwright.PageGotoOptions) (playwright.Response, error) {
		handler(early)
		return nil, nil
	})
	early.EXPECT().URL().Return(testSigninURL)
	early.EXPECT().PostData().Return("RelayState=state", nil)
	page.EXPECT().ExpectRequest(gomock.Any(), gomock.Any(), gomock.Any()).Return(valid, nil)
	valid.EXPECT().PostData().Return(samlPostBody(testAssertion), nil)

	got, err := getSAMLResponse(page, &creds.LoginDetails{URL: testIdPURL}, client)
	require.NoError(t, err)
	assert.Equal(t, testAssertion, got)
}

// A body-read error recorded by the callback is reported when the wait fails, not hidden behind the wait error.
func TestGetSAMLResponseKeepsRecordedBodyErrorWhenWaitFails(t *testing.T) {
	ctrl := gomock.NewController(t)
	page := NewMocksamlPage(ctrl)
	early := NewMockRequest(ctrl)
	client := newTestClient(t)

	errWait := errors.New("wait timed out")
	var handler func(playwright.Request)
	page.EXPECT().OnRequest(gomock.Any()).Do(func(fn func(playwright.Request)) { handler = fn })
	page.EXPECT().Goto(testIdPURL).DoAndReturn(func(string, ...playwright.PageGotoOptions) (playwright.Response, error) {
		handler(early)
		return nil, nil
	})
	early.EXPECT().URL().Return(testSigninURL)
	early.EXPECT().PostData().Return("", errTest)
	page.EXPECT().ExpectRequest(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, errWait)

	_, err := getSAMLResponse(page, &creds.LoginDetails{URL: testIdPURL}, client)
	require.ErrorIs(t, err, errTest)
	require.ErrorIs(t, err, errWait)
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
