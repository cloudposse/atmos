package adfs

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

	"github.com/cloudposse/atmos/pkg/auth/providers/aws/saml2aws/mocks"
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

func TestGetAndSubmitReportUnreadableResponseBody(t *testing.T) {
	ac := newBrokenBodyClient(t)

	t.Run("get", func(t *testing.T) {
		doc, err := ac.get("https://idp.example.com/adfs/ls/")
		require.ErrorIs(t, err, errBodyRead)
		assert.Contains(t, err.Error(), "failed to build document from response")
		assert.Nil(t, doc)
	})

	t.Run("submit", func(t *testing.T) {
		doc, err := ac.submit("https://idp.example.com/adfs/ls/", url.Values{"a": {"b"}})
		require.ErrorIs(t, err, errBodyRead)
		assert.Contains(t, err.Error(), "failed to build document from response")
		assert.Nil(t, doc)
	})
}

func TestSubmitRejectsInvalidURL(t *testing.T) {
	// A control character in the host makes the URL unparsable, so no request is built.
	doc, err := newBrokenBodyClient(t).submit("https://idp.example.com\x7f/adfs/ls/", url.Values{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "error building request")
	assert.Nil(t, doc)
}

func TestAuthenticateRejectsUnparsableFormAction(t *testing.T) {
	// An unterminated IPv6 literal cannot be parsed as a URL.
	fake := &fakeADFS{loginHTML: loginPage("https://[::1/adfs/ls/")}
	ts := newTestServer(t, fake)

	assertion, err := newTestClient(t).Authenticate(loginDetails(ts.URL, ""))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse authSubmitURL fragment")
	assert.Empty(t, assertion)
	assert.Empty(t, fake.postedForms(), "no credentials may be posted when the action is invalid")
}

func TestAuthenticateMFAPromptFailure(t *testing.T) {
	errCancelled := errors.New("prompt cancelled")
	pr := &mocks.Prompter{}
	pr.On("RequestSecurityCode", "000000").Return("", errCancelled).Once()
	usePrompter(t, pr)

	fake := &fakeADFS{
		loginHTML: loginPage("/adfs/ls/"),
		responses: []string{mfaPage(`<input name="VerificationCode" type="text" value="" />`)},
	}
	ts := newTestServer(t, fake)

	assertion, err := newTestClient(t).Authenticate(loginDetails(ts.URL, ""))
	require.ErrorIs(t, err, errCancelled)
	assert.Contains(t, err.Error(), "error reading the MFA code")
	assert.Empty(t, assertion)
	// Only the login form was posted; no MFA form is submitted after the prompt fails.
	assert.Len(t, fake.postedForms(), 1)
	pr.AssertExpectations(t)
}

func TestChallengeSubmitURLKeepsFallbackForUnparsableAction(t *testing.T) {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(`<form action="%zz"></form>`))
	require.NoError(t, err)
	doc.Url, err = url.Parse("https://idp.example.com/adfs/ls/")
	require.NoError(t, err)

	assert.Equal(t, "https://fallback.example.com/login", challengeSubmitURL(doc, "https://fallback.example.com/login"))
}

func TestUpdatePassthroughFormData(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  url.Values
	}{
		{name: "name and value are copied", input: `<input name="Context" value="ctx" />`, want: url.Values{"Context": {"ctx"}}},
		{name: "empty value is kept", input: `<input name="Context" value="" />`, want: url.Values{"Context": {""}}},
		{name: "missing name is ignored", input: `<input value="orphan" />`, want: url.Values{}},
		{name: "missing value is ignored", input: `<input name="Context" />`, want: url.Values{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc, err := goquery.NewDocumentFromReader(strings.NewReader(tt.input))
			require.NoError(t, err)

			got := url.Values{}
			doc.Find("input").Each(func(_ int, s *goquery.Selection) {
				updatePassthroughFormData(got, s)
			})
			assert.Equal(t, tt.want, got)
		})
	}
}
