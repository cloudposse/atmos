package adfs

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/auth/providers/aws/saml2aws/mocks"
)

func TestBuildSignOnURL(t *testing.T) {
	urn := url.QueryEscape("urn:amazon:webservices")
	tests := []struct {
		name string
		base string
		want string
	}{
		{
			name: "base URL gets the sign-on endpoint",
			base: "https://adfs.example.com",
			want: "https://adfs.example.com/adfs/ls/IdpInitiatedSignOn.aspx?loginToRp=urn%3Aamazon%3Awebservices",
		},
		{
			name: "full sign-on URL is reused unchanged",
			base: "https://adfs.example.com/adfs/ls/IdpInitiatedSignOn.aspx?loginToRp=urn%3Aamazon%3Awebservices",
			want: "https://adfs.example.com/adfs/ls/IdpInitiatedSignOn.aspx?loginToRp=urn%3Aamazon%3Awebservices",
		},
		{
			name: "an existing loginToRp value is not changed",
			base: "https://adfs.example.com/adfs/ls/idpinitiatedsignon.aspx?loginToRp=urn%3Aother",
			want: "https://adfs.example.com/adfs/ls/idpinitiatedsignon.aspx?loginToRp=urn%3Aother",
		},
		{
			name: "sign-on URL without loginToRp gets it added",
			base: "https://adfs.example.com/adfs/ls/IdpInitiatedSignOn.aspx",
			want: "https://adfs.example.com/adfs/ls/IdpInitiatedSignOn.aspx?loginToRp=urn%3Aamazon%3Awebservices",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, buildSignOnURL(tt.base, urn))
		})
	}
}

// Regression: a configured full SSO URL had a second endpoint appended inside its query.
func TestAuthenticateFullSignOnURL(t *testing.T) {
	fake := &fakeADFS{
		loginHTML: loginPage("/adfs/ls/"),
		responses: []string{samlResponsePage},
	}
	ts := newTestServer(t, fake)

	details := loginDetails(ts.URL+"/adfs/ls/IdpInitiatedSignOn.aspx?loginToRp=urn%3Aamazon%3Awebservices", "")
	assertion, err := newTestClient(t).Authenticate(details)
	require.NoError(t, err)
	assert.Equal(t, testSAMLAssertion, assertion)

	getQuery, _ := fake.requests()
	assert.Equal(t, "urn:amazon:webservices", getQuery.Get("loginToRp"))
}

// Regression: an unrelated form after the OTP form supplied the action, and the OTP was sent there.
func TestAuthenticateMFAUsesTheChallengeForm(t *testing.T) {
	page := `<html><body>
<form method="post" action="/adfs/ls/mfa"><input name="VerificationCode" type="text" value="" />
<input name="Context" type="hidden" value="ctx-123" /></form>
<form method="post" action="/adfs/ls/unrelated"><input name="Other" type="hidden" value="unrelated-value" /></form>
</body></html>`
	fake := &fakeADFS{
		loginHTML: loginPage("/adfs/ls/"),
		responses: []string{page, samlResponsePage},
	}
	ts := newTestServer(t, fake)

	assertion, err := newTestClient(t).Authenticate(loginDetails(ts.URL, "654321"))
	require.NoError(t, err)
	assert.Equal(t, testSAMLAssertion, assertion)

	_, paths := fake.requests()
	assert.Equal(t, []string{"/adfs/ls/", "/adfs/ls/mfa"}, paths)
	posts := fake.postedForms()
	require.Len(t, posts, 2)
	assert.Equal(t, []string{"654321"}, posts[1]["VerificationCode"])
	assert.Equal(t, []string{"ctx-123"}, posts[1]["Context"])
	// Inputs of the unrelated form are not part of the submission.
	_, hasOther := posts[1]["Other"]
	assert.False(t, hasOther)
}

// Regression: an options form with its own AuthMethod input before the OTP form was submitted instead of
// the OTP form, so the code was never sent.
func TestAuthenticateMFAIgnoresEarlierOptionsForm(t *testing.T) {
	page := `<html><body>
<form method="post" action="/adfs/ls/options"><input name="AuthMethod" type="hidden" value="FormsAuthentication" /></form>
<form method="post" action="/adfs/ls/mfa"><input name="VerificationCode" type="text" value="" />
<input name="Context" type="hidden" value="ctx-123" /></form>
</body></html>`
	fake := &fakeADFS{
		loginHTML: loginPage("/adfs/ls/"),
		responses: []string{page, samlResponsePage},
	}
	ts := newTestServer(t, fake)

	assertion, err := newTestClient(t).Authenticate(loginDetails(ts.URL, "654321"))
	require.NoError(t, err)
	assert.Equal(t, testSAMLAssertion, assertion)

	_, paths := fake.requests()
	assert.Equal(t, []string{"/adfs/ls/", "/adfs/ls/mfa"}, paths)
	posts := fake.postedForms()
	require.Len(t, posts, 2)
	assert.Equal(t, []string{"654321"}, posts[1]["VerificationCode"])
	assert.Equal(t, []string{"ctx-123"}, posts[1]["Context"])
	// The options form's AuthMethod is not part of the submission.
	_, hasAuthMethod := posts[1]["AuthMethod"]
	assert.False(t, hasAuthMethod)
}

// The challenge form is the one holding the control that classified the page; pages without such a form keep
// using the first form with any challenge control.
func TestChallengeFormSelection(t *testing.T) {
	const options = `<form action="/adfs/ls/options"><input name="AuthMethod" type="hidden" value="FormsAuthentication" /></form>`
	base, err := url.Parse("https://idp.example.com/adfs/ls/")
	require.NoError(t, err)

	tests := []struct {
		name string
		html string
		want string
	}{
		{
			name: "code form after options form",
			html: options + `<form action="/adfs/ls/mfa"><input name="OathCode" value="" /></form>`,
			want: "https://idp.example.com/adfs/ls/mfa",
		},
		{
			name: "Azure wait form after options form",
			html: options + `<form action="/adfs/ls/azure"><input name="AuthMethod" type="hidden" value="AzureMfaAuthentication" /></form>`,
			want: "https://idp.example.com/adfs/ls/azure",
		},
		{
			name: "Azure server wait form after options form",
			html: options + `<form action="/adfs/ls/azure-server"><input name="AuthMethod" type="hidden" value="AzureMfaServerAuthentication" /></form>`,
			want: "https://idp.example.com/adfs/ls/azure-server",
		},
		{
			name: "VIP prompt form after options form",
			html: options + `<form action="/adfs/ls/vip"><input name="AuthMethod" type="hidden" value="VIPAuthenticationProviderUPN" /></form>`,
			want: "https://idp.example.com/adfs/ls/vip",
		},
		{
			name: "single challenge form",
			html: `<form action="/adfs/ls/mfa"><input name="VerificationCode" value="" /></form>`,
			want: "https://idp.example.com/adfs/ls/mfa",
		},
		{
			name: "unclassified page keeps the first form with a challenge control",
			html: `<form action="/adfs/ls/plain"><input name="Other" /></form>` + options,
			want: "https://idp.example.com/adfs/ls/options",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc, err := goquery.NewDocumentFromReader(strings.NewReader(tt.html))
			require.NoError(t, err)
			doc.Url = base
			assert.Equal(t, tt.want, challengeSubmitURL(doc, "https://fallback.example.com/login"))
		})
	}
}

// Regression: a SAMLResponse input without a value followed by an MFA input submitted an OTP.
func TestAuthenticateParseErrorStopsBeforeMFA(t *testing.T) {
	pr := &mocks.Prompter{}
	usePrompter(t, pr)

	fake := &fakeADFS{
		loginHTML: loginPage("/adfs/ls/"),
		responses: []string{`<form action="/adfs/ls/mfa"><input type="hidden" name="SAMLResponse" /><input name="OathCode" type="text" value="" /></form>`},
	}
	ts := newTestServer(t, fake)

	_, err := newTestClient(t).Authenticate(loginDetails(ts.URL, "123456"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unable to locate saml assertion value")
	// Only the login form was posted; no code was submitted or prompted for.
	assert.Len(t, fake.postedForms(), 1)
	pr.AssertNotCalled(t, "RequestSecurityCode", "000000")
}

// Regression: the poll count alone did not bound the wait when each poll request is slow.
func TestAuthenticateAzureMFAElapsedTimeLimit(t *testing.T) {
	useFastAzurePolling(t)
	orig := azureMFAWaitTimeout
	azureMFAWaitTimeout = 100 * time.Millisecond
	t.Cleanup(func() { azureMFAWaitTimeout = orig })

	var posts atomic.Int32
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			_, _ = w.Write([]byte(loginPage("/adfs/ls/")))
			return
		}
		if posts.Add(1) == 1 {
			_, _ = w.Write([]byte(azurePage("AzureMfaAuthentication", "")))
			return
		}
		// A pending poll that takes far longer than the whole approval window.
		select {
		case <-r.Context().Done():
		case <-time.After(10 * time.Second):
		}
	}))
	t.Cleanup(ts.Close)

	start := time.Now()
	assertion, err := newTestClient(t).Authenticate(loginDetails(ts.URL, ""))
	require.ErrorIs(t, err, ErrMFAAttemptsExceeded)
	assert.Empty(t, assertion)
	assert.Less(t, time.Since(start), 5*time.Second)
}
