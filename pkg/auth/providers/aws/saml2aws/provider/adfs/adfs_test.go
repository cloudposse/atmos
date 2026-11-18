package adfs

import (
	"bytes"
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/auth/providers/aws/saml2aws/cfg"
	"github.com/cloudposse/atmos/pkg/auth/providers/aws/saml2aws/creds"
	"github.com/cloudposse/atmos/pkg/auth/providers/aws/saml2aws/mocks"
	"github.com/cloudposse/atmos/pkg/auth/providers/aws/saml2aws/prompter"
)

const (
	testSAMLAssertion = "PHNhbWxwOlJlc3BvbnNlPjwvc2FtbHA6UmVzcG9uc2U+"
	testUsername      = "alice@example.com"
	testPassword      = "s3cr3t"
	// abortResponse instructs the fake server to drop the connection without replying.
	abortResponse = "<<abort>>"
)

// samlResponsePage mimics the auto-submitting page ADFS returns after a successful login.
var samlResponsePage = `<html><body>
<form method="POST" name="hiddenform" action="https://signin.aws.amazon.com:443/saml">
<input type="hidden" name="SAMLResponse" value="` + testSAMLAssertion + `" />
<noscript><p>Script is disabled. Click Submit to continue.</p><input type="submit" value="Submit" /></noscript>
</form></body></html>`

// loginPage mimics the ADFS forms-authentication page.
func loginPage(action string) string {
	actionAttr := ""
	if action != "" {
		actionAttr = fmt.Sprintf(` action=%q`, action)
	}
	return `<html><body>
<form id="loginForm" method="post"` + actionAttr + `>
<input id="userNameInput" name="UserName" type="email" value="" />
<input id="userNameHidden" name="UserNameHidden" type="hidden" value="should-not-be-sent" />
<input id="passwordInput" name="Password" type="password" value="" />
<input id="kmsiInput" name="Kmsi" type="checkbox" value="  true  " />
<input id="kmsiEmpty" name="Kmsi" type="hidden" value="   " />
<input id="optionForms" name="AuthMethod" type="hidden" value="FormsAuthentication" />
<input type="hidden" value="unnamed inputs are ignored" />
</form></body></html>`
}

// fakeADFSMaxPosts stops a fake server that repeats its last response from looping forever.
const fakeADFSMaxPosts = 1000

// fakeADFS is an httptest handler that serves the login page on GET and replays canned
// responses, in order, for each POST it receives.
type fakeADFS struct {
	t         *testing.T
	loginHTML string
	responses []string

	// repeatLast serves the last response for every POST after the canned ones run out,
	// up to fakeADFSMaxPosts, after which the connection is dropped.
	repeatLast bool

	mu       sync.Mutex
	getQuery url.Values
	posts    []url.Values
	paths    []string
}

func (f *fakeADFS) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if r.Method == http.MethodGet {
		f.getQuery = r.URL.Query()
		_, _ = w.Write([]byte(f.loginHTML))
		return
	}

	if err := r.ParseForm(); err != nil {
		f.t.Errorf("failed to parse posted form: %v", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	idx := len(f.posts)
	f.posts = append(f.posts, r.PostForm)
	f.paths = append(f.paths, r.URL.RequestURI())

	if f.repeatLast && idx >= len(f.responses) && len(f.responses) > 0 {
		if idx >= fakeADFSMaxPosts {
			panic(http.ErrAbortHandler)
		}
		idx = len(f.responses) - 1
	}
	if idx >= len(f.responses) {
		f.t.Errorf("unexpected POST #%d to %s", idx+1, r.URL)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	if f.responses[idx] == abortResponse {
		panic(http.ErrAbortHandler)
	}
	_, _ = w.Write([]byte(f.responses[idx]))
}

func (f *fakeADFS) postedForms() []url.Values {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]url.Values(nil), f.posts...)
}

// newTestServer starts a TLS server backed by the fake ADFS handler.
func newTestServer(t *testing.T, fake *fakeADFS) *httptest.Server {
	t.Helper()
	fake.t = t
	ts := httptest.NewTLSServer(fake)
	t.Cleanup(ts.Close)
	return ts
}

func newTestClient(t *testing.T) *Client {
	t.Helper()
	client, err := New(&cfg.IDPAccount{
		SkipVerify:           true,
		AmazonWebservicesURN: cfg.DefaultAmazonWebservicesURN,
	})
	require.NoError(t, err)
	return client
}

func loginDetails(serverURL, mfaToken string) *creds.LoginDetails {
	return &creds.LoginDetails{
		URL:      serverURL,
		Username: testUsername,
		Password: testPassword,
		MFAToken: mfaToken,
	}
}

func mfaPage(fields string) string {
	return `<html><body><form method="post" action="/adfs/ls/mfa">` + fields +
		`<input name="Context" type="hidden" value="ctx-123" /></form></body></html>`
}

func azurePage(authMethod, extra string) string {
	return `<html><body><form method="post" action="/adfs/ls/mfa">
<input name="AuthMethod" type="hidden" value="` + authMethod + `" />
<input name="Context" type="hidden" value="azure-ctx" />
</form>` + extra + `</body></html>`
}

// useFastAzurePolling shortens the Azure MFA poll interval for the duration of a test.
func useFastAzurePolling(t *testing.T) {
	t.Helper()
	orig := azureMFAPollInterval
	azureMFAPollInterval = time.Millisecond
	t.Cleanup(func() { azureMFAPollInterval = orig })
}

// usePrompter swaps the active prompter for the duration of a test.
func usePrompter(t *testing.T, p prompter.Prompter) {
	t.Helper()
	orig := prompter.ActivePrompter
	prompter.SetPrompter(p)
	t.Cleanup(func() { prompter.SetPrompter(orig) })
}

// captureLog redirects the standard logger for the duration of a test.
func captureLog(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	origOut, origFlags := log.Writer(), log.Flags()
	log.SetOutput(&buf)
	log.SetFlags(0)
	t.Cleanup(func() {
		log.SetOutput(origOut)
		log.SetFlags(origFlags)
	})
	return &buf
}

func TestAuthenticateSuccessRelativeAction(t *testing.T) {
	fake := &fakeADFS{
		loginHTML: loginPage("/adfs/ls/?client-request-id=abc"),
		responses: []string{samlResponsePage},
	}
	ts := newTestServer(t, fake)

	assertion, err := newTestClient(t).Authenticate(loginDetails(ts.URL, ""))
	require.NoError(t, err)
	assert.Equal(t, testSAMLAssertion, assertion)

	// The IdP-initiated sign-on request targets the AWS relying party.
	assert.Equal(t, cfg.DefaultAmazonWebservicesURN, fake.getQuery.Get("loginToRp"))

	// The relative action was resolved against the ADFS URL, preserving its query.
	require.Len(t, fake.paths, 1)
	assert.Equal(t, "/adfs/ls/?client-request-id=abc", fake.paths[0])

	posts := fake.postedForms()
	require.Len(t, posts, 1)
	form := posts[0]
	assert.Equal(t, []string{testUsername}, form["UserName"])
	assert.Equal(t, []string{testPassword}, form["Password"])
	assert.Equal(t, []string{"FormsAuthentication"}, form["AuthMethod"])
	// Kmsi values are trimmed and empty values discarded.
	assert.Equal(t, []string{"true"}, form["Kmsi"])
	// Hidden inputs that look like credential fields are never sent.
	_, hasHidden := form["UserNameHidden"]
	assert.False(t, hasHidden)
}

func TestAuthenticateSuccessAbsoluteAction(t *testing.T) {
	fake := &fakeADFS{responses: []string{samlResponsePage}}
	ts := newTestServer(t, fake)
	fake.loginHTML = loginPage(ts.URL + "/adfs/ls/absolute")

	assertion, err := newTestClient(t).Authenticate(loginDetails(ts.URL, ""))
	require.NoError(t, err)
	assert.Equal(t, testSAMLAssertion, assertion)
	require.Len(t, fake.paths, 1)
	assert.Equal(t, "/adfs/ls/absolute", fake.paths[0])
}

func TestAuthenticateErrors(t *testing.T) {
	// A server that is closed immediately yields a URL nothing listens on.
	closed := httptest.NewServer(http.NotFoundHandler())
	closedURL := closed.URL
	closed.Close()

	tests := []struct {
		name      string
		serverURL func(ts *httptest.Server) string
		loginHTML func(ts *httptest.Server) string
		responses []string
		wantErr   string
	}{
		{
			name:      "login page unreachable",
			serverURL: func(*httptest.Server) string { return closedURL },
			loginHTML: func(*httptest.Server) string { return "" },
			wantErr:   "failed to get adfs page",
		},
		{
			name:      "login page has no form action",
			loginHTML: func(*httptest.Server) string { return loginPage("") },
			wantErr:   "unable to locate IDP authentication form submit URL",
		},
		{
			name:      "auth form submission fails",
			loginHTML: func(*httptest.Server) string { return loginPage(closedURL + "/adfs/ls/") },
			wantErr:   "failed to submit adfs auth form",
		},
		{
			name:      "unclassifiable response",
			loginHTML: func(*httptest.Server) string { return loginPage("/adfs/ls/") },
			// A failed login re-renders the forms-authentication page.
			responses: []string{loginPage("/adfs/ls/")},
			wantErr:   "unable to classify response from auth server",
		},
		{
			name:      "SAMLResponse without value",
			loginHTML: func(*httptest.Server) string { return loginPage("/adfs/ls/") },
			responses: []string{`<form><input type="hidden" name="SAMLResponse" /></form>`},
			wantErr:   "unable to locate saml assertion value",
		},
		{
			name:      "mfa submission fails",
			loginHTML: func(*httptest.Server) string { return loginPage("/adfs/ls/") },
			responses: []string{mfaPage(`<input name="VerificationCode" type="text" value="" />`), abortResponse},
			wantErr:   "error retrieving mfa form results",
		},
		{
			name:      "azure mfa poll fails",
			loginHTML: func(*httptest.Server) string { return loginPage("/adfs/ls/") },
			responses: []string{azurePage("AzureMfaAuthentication", ""), abortResponse},
			wantErr:   "error retrieving mfa form results",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			useFastAzurePolling(t)
			fake := &fakeADFS{responses: tt.responses}
			ts := newTestServer(t, fake)
			fake.loginHTML = tt.loginHTML(ts)
			serverURL := ts.URL
			if tt.serverURL != nil {
				serverURL = tt.serverURL(ts)
			}

			assertion, err := newTestClient(t).Authenticate(loginDetails(serverURL, "123456"))
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
			assert.Empty(t, assertion)
		})
	}
}

func TestAuthenticateRejectsUntrustedCertificate(t *testing.T) {
	fake := &fakeADFS{loginHTML: loginPage("/adfs/ls/"), responses: []string{samlResponsePage}}
	ts := newTestServer(t, fake)

	client, err := New(&cfg.IDPAccount{SkipVerify: false})
	require.NoError(t, err)

	_, err = client.Authenticate(loginDetails(ts.URL, ""))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to get adfs page")
	assert.Empty(t, fake.postedForms())
}

func TestAuthenticateMFAPrompt(t *testing.T) {
	tests := []struct {
		name       string
		fields     string
		tokenField string
	}{
		{
			name:       "OathCode",
			fields:     `<input name="OathCode" type="text" value="" />`,
			tokenField: "OathCode",
		},
		{
			name:       "VerificationCode",
			fields:     `<input name="VerificationCode" type="text" value="" />`,
			tokenField: "VerificationCode",
		},
		{
			name: "VIP account name",
			fields: `<input name="AuthMethod" type="hidden" value="VIPAuthenticationProviderWindowsAccountName" />
<input name="security_code" type="text" value="" />`,
			tokenField: "security_code",
		},
		{
			name: "VIP UPN",
			fields: `<input name="AuthMethod" type="hidden" value="VIPAuthenticationProviderUPN" />
<input name="security_code" type="text" value="" />`,
			tokenField: "security_code",
		},
		{
			name: "Defender",
			fields: `<input name="AuthMethod" type="hidden" value="Defender AD FS Adapter" />
<input name="ChallengeQuestionAnswer" type="password" value="" />`,
			tokenField: "ChallengeQuestionAnswer",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fake := &fakeADFS{
				loginHTML: loginPage("/adfs/ls/"),
				responses: []string{mfaPage(tt.fields), samlResponsePage},
			}
			ts := newTestServer(t, fake)

			assertion, err := newTestClient(t).Authenticate(loginDetails(ts.URL, "654321"))
			require.NoError(t, err)
			assert.Equal(t, testSAMLAssertion, assertion)

			posts := fake.postedForms()
			require.Len(t, posts, 2)
			mfaForm := posts[1]
			assert.Equal(t, []string{"654321"}, mfaForm[tt.tokenField])
			// Other inputs are passed through unchanged.
			assert.Equal(t, []string{"ctx-123"}, mfaForm["Context"])
			// Credentials are not re-sent with the OTP form.
			_, hasPassword := mfaForm["Password"]
			assert.False(t, hasPassword)
		})
	}
}

func TestAuthenticateMFAPromptsWhenTokenMissingOrConsumed(t *testing.T) {
	pr := &mocks.Prompter{}
	pr.On("RequestSecurityCode", "000000").Return("999999").Once()
	usePrompter(t, pr)

	verification := mfaPage(`<input name="VerificationCode" type="text" value="" />`)
	fake := &fakeADFS{
		loginHTML: loginPage("/adfs/ls/"),
		responses: []string{verification, verification, samlResponsePage},
	}
	ts := newTestServer(t, fake)

	// The configured token is used for the first challenge only; the second must be prompted for.
	assertion, err := newTestClient(t).Authenticate(loginDetails(ts.URL, "111111"))
	require.NoError(t, err)
	assert.Equal(t, testSAMLAssertion, assertion)

	posts := fake.postedForms()
	require.Len(t, posts, 3)
	assert.Equal(t, []string{"111111"}, posts[1]["VerificationCode"])
	assert.Equal(t, []string{"999999"}, posts[2]["VerificationCode"])
	pr.AssertExpectations(t)
}

func TestAuthenticateAzureMFAWait(t *testing.T) {
	useFastAzurePolling(t)
	logs := captureLog(t)

	waiting := azurePage("AzureMfaAuthentication", `<p id="validEntropyNumber">42</p>`)
	fake := &fakeADFS{
		loginHTML: loginPage("/adfs/ls/"),
		responses: []string{waiting, waiting, samlResponsePage},
	}
	ts := newTestServer(t, fake)

	assertion, err := newTestClient(t).Authenticate(loginDetails(ts.URL, ""))
	require.NoError(t, err)
	assert.Equal(t, testSAMLAssertion, assertion)

	posts := fake.postedForms()
	require.Len(t, posts, 3)
	for _, poll := range posts[1:] {
		assert.Equal(t, []string{"AzureMfaAuthentication"}, poll["AuthMethod"])
		assert.Equal(t, []string{"azure-ctx"}, poll["Context"])
	}
	// The number to tap is shown once even though the page was polled twice.
	assert.Equal(t, 1, strings.Count(logs.String(), "Microsoft Authenticator"))
	assert.Contains(t, logs.String(), "42")
}

func TestAuthenticateAzureMFAServerWait(t *testing.T) {
	waiting := azurePage("AzureMfaServerAuthentication", `<p id="instructions">Approve the sign-in request.</p>`)

	tests := []struct {
		name      string
		responses []string
		wantErr   string
	}{
		{
			name:      "approved",
			responses: []string{waiting, samlResponsePage},
		},
		{
			name:      "denied",
			responses: []string{waiting, `<html><body><label id="errorText">The request was denied.</label></body></html>`},
			wantErr:   "The request was denied.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			useFastAzurePolling(t)
			logs := captureLog(t)
			fake := &fakeADFS{loginHTML: loginPage("/adfs/ls/"), responses: tt.responses}
			ts := newTestServer(t, fake)

			assertion, err := newTestClient(t).Authenticate(loginDetails(ts.URL, ""))
			assert.Contains(t, logs.String(), "Approve the sign-in request.")
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Equal(t, tt.wantErr, err.Error())
				assert.Empty(t, assertion)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, testSAMLAssertion, assertion)
		})
	}
}

func TestCheckResponse(t *testing.T) {
	tests := []struct {
		name          string
		html          string
		wantType      AuthResponseType
		wantAssertion string
		wantErr       bool
	}{
		{name: "empty page", html: `<html></html>`, wantType: UNKNOWN},
		{name: "saml response", html: samlResponsePage, wantType: SAML_RESPONSE, wantAssertion: testSAMLAssertion},
		{
			// Regression: upstream terminated the process via log.Fatalf on unnamed inputs.
			name:          "saml response with unnamed submit input",
			html:          `<form><input type="hidden" name="SAMLResponse" value="abc" /><input type="submit" value="Continue" /></form>`,
			wantType:      SAML_RESPONSE,
			wantAssertion: "abc",
		},
		{
			// Regression: upstream terminated the process via log.Fatalf when the value was missing.
			name:     "saml response without value",
			html:     `<form><input type="hidden" name="SAMLResponse" /></form>`,
			wantType: SAML_RESPONSE,
			wantErr:  true,
		},
		{name: "oath code", html: `<input name="OathCode" />`, wantType: MFA_PROMPT},
		{name: "verification code", html: `<input name="VerificationCode" />`, wantType: MFA_PROMPT},
		{name: "azure", html: `<input name="AuthMethod" value="AzureMfaAuthentication" />`, wantType: AZURE_MFA_WAIT},
		{name: "azure server", html: `<input name="AuthMethod" value="AzureMfaServerAuthentication" />`, wantType: AZURE_MFA_SERVER_WAIT},
		{name: "forms auth", html: `<input name="AuthMethod" value="FormsAuthentication" />`, wantType: UNKNOWN},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc, err := goquery.NewDocumentFromReader(strings.NewReader(tt.html))
			require.NoError(t, err)

			gotType, gotAssertion, err := checkResponse(doc)
			assert.Equal(t, tt.wantType, gotType)
			assert.Equal(t, tt.wantAssertion, gotAssertion)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestUpdateFormData(t *testing.T) {
	user := &creds.LoginDetails{Username: "bob", Password: "pw"}

	tests := []struct {
		name  string
		input string
		want  url.Values
	}{
		{name: "username", input: `<input name="UserName" type="text" />`, want: url.Values{"UserName": {"bob"}}},
		{name: "email", input: `<input name="EmailAddress" type="email" />`, want: url.Values{"EmailAddress": {"bob"}}},
		{name: "password", input: `<input name="Password" type="password" />`, want: url.Values{"Password": {"pw"}}},
		{name: "hidden username skipped", input: `<input name="UserName" type="hidden" value="x" />`, want: url.Values{}},
		{name: "hidden email skipped", input: `<input name="Email" type="hidden" value="x" />`, want: url.Values{}},
		{name: "hidden password skipped", input: `<input name="Password" type="hidden" value="x" />`, want: url.Values{}},
		{name: "passthrough", input: `<input name="AuthMethod" type="hidden" value="Forms" />`, want: url.Values{"AuthMethod": {"Forms"}}},
		{name: "passthrough without value", input: `<input name="Other" type="hidden" />`, want: url.Values{}},
		{name: "unnamed", input: `<input type="submit" value="Sign in" />`, want: url.Values{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc, err := goquery.NewDocumentFromReader(strings.NewReader(tt.input))
			require.NoError(t, err)

			got := url.Values{}
			doc.Find("input").Each(func(_ int, s *goquery.Selection) {
				updateFormData(got, s, user)
			})
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestUpdateOTPFormDataIgnoresUnnamedInputs(t *testing.T) {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(`<input type="submit" value="Verify" />`))
	require.NoError(t, err)

	got := url.Values{}
	doc.Find("input").Each(func(_ int, s *goquery.Selection) {
		updateOTPFormData(got, s, "123456")
	})
	assert.Empty(t, got)
}
