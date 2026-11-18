package okta

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/marshallbrekka/go-u2fhost"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/auth/providers/aws/saml2aws/creds"
	"github.com/cloudposse/atmos/pkg/auth/providers/aws/saml2aws/mocks"
)

// reviewTestWait bounds how long a regression test waits for a call that must return promptly.
const reviewTestWait = 10 * time.Second

// followPage runs follow() against a single page served at the app path.
func followPage(t *testing.T, page, target string) (string, *routes, error) {
	t.Helper()

	handler := newRoutes(map[string]func(int, *http.Request) string{
		pathApp:   static(page),
		"/next":   static(samlTargetPage("https://aws.example.com/saml", encodedSAML())),
		"/wrong":  static(`<html>wrong form submitted</html>`),
		"/resume": static(samlTargetPage("https://aws.example.com/saml", encodedSAML())),
	})
	oc := newFaultClient(t, handler, nil)
	oc.targetURL = target
	req, err := http.NewRequest(http.MethodGet, faultAppURL, nil)
	require.NoError(t, err)
	got, err := oc.follow(context.Background(), req, faultLogin())
	return got, handler, err
}

// Regression: a page that has the target form but no SAMLResponse is "unknown"; the debug log must carry
// a form count, not the page, which can hold state tokens.
func TestFollow_UnknownDocumentDoesNotLogPageHTML(t *testing.T) {
	logs := captureDebugLogs(t)
	page := `<html><body><form action="https://aws.example.com/saml"><input name="state" value="secret-state-token"/></form></body></html>`

	_, _, err := followPage(t, page, "https://aws.example.com/saml")
	require.ErrorContains(t, err, "unknown document type")
	assert.Contains(t, logs.String(), "Unknown document type")
	assert.NotContains(t, logs.String(), "secret-state-token")
}

// Regression: the form that carries SAMLRequest is submitted, not the first form with an action.
func TestFollow_SubmitsTheFormThatTriggeredTheRedirect(t *testing.T) {
	page := `<html><body>` +
		`<form action="/wrong"><input name="unrelated" value="x"/></form>` +
		`<form action="/next"><input name="SAMLRequest" value="req"/></form>` +
		`</body></html>`

	got, handler, err := followPage(t, page, "https://aws.example.com/saml")
	require.NoError(t, err)
	assert.Equal(t, encodedSAML(), got)
	assert.Equal(t, 1, handler.calls("/next"))
	assert.Zero(t, handler.calls("/wrong"))
}

// Negative path: with a single form the same form is submitted, as before.
func TestFollow_SingleRedirectFormIsSubmitted(t *testing.T) {
	page := `<html><form action="/resume"><input name="RelayState" value="r"/></form></html>`

	got, handler, err := followPage(t, page, "https://aws.example.com/saml")
	require.NoError(t, err)
	assert.Equal(t, encodedSAML(), got)
	assert.Equal(t, 1, handler.calls("/resume"))
}

// Regression: the assertion is read from the form that posts to the target, not from an earlier form.
func TestFollow_ReadsAssertionFromTargetForm(t *testing.T) {
	intermediate := "aW50ZXJtZWRpYXRl" // "intermediate".
	page := `<html><body>` +
		fmt.Sprintf(`<form action="https://idp.example.com/other"><input name="SAMLResponse" value="%s"/></form>`, intermediate) +
		fmt.Sprintf(`<form action="https://aws.example.com/saml"><input name="SAMLResponse" value="%s"/></form>`, encodedSAML()) +
		`</body></html>`

	got, _, err := followPage(t, page, "https://aws.example.com/saml")
	require.NoError(t, err)
	assert.Equal(t, encodedSAML(), got)
}

// Regression: when several forms post to the target, the one carrying the assertion is used, and an
// assertion in a form that posts elsewhere is still never returned.
func TestFollow_FindsAssertionAmongTargetForms(t *testing.T) {
	const target = "https://aws.example.com/saml"
	tests := []struct {
		name    string
		page    string
		want    string
		wantErr string
	}{
		{
			name: "auxiliary target form before the assertion form",
			page: fmt.Sprintf(`<html><body><form action="%[1]s"><input name="other" value="x"/></form>`+
				`<form action="%[1]s"><input name="SAMLResponse" value="%[2]s"/></form></body></html>`, target, encodedSAML()),
			want: encodedSAML(),
		},
		{
			name: "assertion only in a form that posts elsewhere",
			page: fmt.Sprintf(`<html><body><form action="%s"><input name="other" value="x"/></form>`+
				`<form action="https://idp.example.com/other"><input name="SAMLResponse" value="%s"/></form></body></html>`,
				target, encodedSAML()),
			wantErr: "unknown document type",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, _, err := followPage(t, tc.page, target)
			if tc.wantErr != "" {
				require.ErrorContains(t, err, tc.wantErr)
				assert.Empty(t, got)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

// Regression: a SAMLRequest form without an action is submitted to the page URL, the form the dispatch
// chose, instead of a different form that carries RelayState.
func TestFollow_SubmitsSAMLRequestFormWithoutAction(t *testing.T) {
	page := `<html><body>` +
		`<form method="post"><input name="SAMLRequest" value="req"/></form>` +
		`<form action="/wrong"><input name="RelayState" value="r"/></form>` +
		`</body></html>`
	var submitted string
	handler := newRoutes(map[string]func(int, *http.Request) string{
		pathApp: func(call int, r *http.Request) string {
			if call == 1 {
				return page
			}
			assert.Equal(t, http.MethodPost, r.Method)
			assert.NoError(t, r.ParseForm())
			submitted = r.PostForm.Get("SAMLRequest")
			return samlTargetPage("https://aws.example.com/saml", encodedSAML())
		},
		"/wrong": static(`<html>wrong form submitted</html>`),
	})
	oc := newFaultClient(t, handler, nil)
	oc.targetURL = "https://aws.example.com/saml"
	req, err := http.NewRequest(http.MethodGet, faultAppURL, nil)
	require.NoError(t, err)

	got, err := oc.follow(context.Background(), req, faultLogin())
	require.NoError(t, err)
	assert.Equal(t, encodedSAML(), got)
	assert.Equal(t, "req", submitted)
	assert.Equal(t, 2, handler.calls(pathApp))
	assert.Zero(t, handler.calls("/wrong"))
}

// Regression: authWithSession handles the app response it already read instead of requesting the app again.
func TestAuthWithSession_DoesNotRequestTheAppPageTwice(t *testing.T) {
	handler := sessionAPI(map[string]func(int, *http.Request) string{
		pathApp: func(call int, _ *http.Request) string {
			if call > 1 {
				return `<html>the login transaction cannot be reused</html>`
			}
			return samlTargetPage("https://aws.example.com/saml", encodedSAML())
		},
	})
	oc := newFaultClient(t, handler, nil)
	oc.targetURL = "https://aws.example.com/saml"
	ld := faultLogin()
	ld.OktaSessionCookie = "sid-1"

	got, err := oc.authWithSession(ld)
	require.NoError(t, err)
	assert.Equal(t, encodedSAML(), got)
	assert.Equal(t, 1, handler.calls(pathApp))
}

// hangingVerifyServer answers the first verify request with WAITING and then blocks until the client gives up.
func hangingVerifyServer(t *testing.T) *httptest.Server {
	t.Helper()

	release := make(chan struct{})
	var mu sync.Mutex
	calls := 0
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		calls++
		call := calls
		mu.Unlock()
		if call == 1 {
			_, _ = fmt.Fprint(w, `{"status":"MFA_CHALLENGE","factorResult":"WAITING"}`)
			return
		}
		select {
		case <-r.Context().Done():
		case <-release:
		}
	}))
	// Cleanups run last-in first-out: release the handlers before the server waits for them.
	t.Cleanup(ts.Close)
	t.Cleanup(func() { close(release) })
	return ts
}

// verifyReturnsWithin runs fn and fails the test when it does not return within reviewTestWait.
func verifyReturnsWithin(t *testing.T, fn func() (string, error)) (string, error) {
	t.Helper()

	type result struct {
		got string
		err error
	}
	done := make(chan result, 1)
	go func() {
		got, err := fn()
		done <- result{got, err}
	}()
	select {
	case r := <-done:
		return r.got, r.err
	case <-time.After(reviewTestWait):
		t.Fatal("the MFA polling loop did not stop at its deadline")
		return "", nil
	}
}

// Regression: the push deadline also bounds an in-flight verify request, not only the waits between polls.
func TestVerifyMfa_PushDeadlineBoundsHungVerifyRequest(t *testing.T) {
	withFastPolling(t)
	orig := mfaPollTimeout
	mfaPollTimeout = 100 * time.Millisecond
	t.Cleanup(func() { mfaPollTimeout = orig })
	ts := hangingVerifyServer(t)

	oc, _ := offlineTestClient(t)
	resp := mfaRequiredResponse(t, "state-1", testFactor{id: "push", provider: "OKTA", factorType: "push", href: ts.URL + "/verify/push"})
	got, err := verifyReturnsWithin(t, func() (string, error) {
		return verifyMfa(oc, "okta.invalid", &creds.LoginDetails{}, resp)
	})
	require.ErrorIs(t, err, errMFAPollTimeout)
	assert.Empty(t, got)
}

// Regression: the Duo deadline also bounds an in-flight status request.
func TestVerifyMfa_DuoDeadlineBoundsHungStatusRequest(t *testing.T) {
	orig := mfaPollTimeout
	mfaPollTimeout = 100 * time.Millisecond
	t.Cleanup(func() { mfaPollTimeout = orig })
	release := make(chan struct{})
	s := &duoScenario{
		authPages:      []string{duoAuthPage},
		prompts:        []string{duoPromptOK},
		statuses:       []string{duoPushed},
		hangStatusFrom: 2,
		release:        release,
	}
	// runDuo registers the server's Close as a cleanup, and Close waits for the blocked handler.
	// Deferred calls run before any cleanup, even when verifyReturnsWithin calls t.Fatal, so the
	// handler is released before the server shuts down.
	defer close(release)

	got, err := verifyReturnsWithin(t, func() (string, error) { return runDuo(t, s, "Duo Push") })
	require.ErrorIs(t, err, errMFAPollTimeout)
	assert.Empty(t, got)
}

// Regression: a FAILURE in the first Duo status response ends the flow without polling again.
func TestVerifyMfa_DuoFirstStatusFailureStopsImmediately(t *testing.T) {
	s := &duoScenario{
		authPages: []string{duoAuthPage},
		prompts:   []string{duoPromptOK},
		statuses:  []string{`{"stat":"OK","response":{"result":"FAILURE","status":"Login request denied"}}`, duoSuccess},
	}

	got, err := runDuo(t, s, "Duo Push")
	require.ErrorContains(t, err, "failed to authenticate device")
	assert.Empty(t, got)
	assert.Equal(t, 1, s.log.count("/frame/status"))
}

// Regression: a sid rotated by a pending status response is sent on the next status poll.
func TestVerifyMfa_DuoPollSendsLatestSID(t *testing.T) {
	s := &duoScenario{
		authPages: []string{duoAuthPage},
		prompts:   []string{duoPromptOK},
		statuses: []string{
			`{"stat":"OK","response":{"status_code":"pushed","sid":"sid-2"}}`,
			`{"stat":"OK","response":{"status_code":"pushed","sid":"sid-3"}}`,
			duoSuccess,
		},
	}

	_, err := runDuo(t, s, "Duo Push")
	require.NoError(t, err)
	forms := s.formsFor("/frame/status")
	require.Len(t, forms, 3)
	assert.Equal(t, "sid-1", forms[0].Get("sid"))
	assert.Equal(t, "sid-2", forms[1].Get("sid"))
	assert.Equal(t, "sid-3", forms[2].Get("sid"))
}

// Regression: the unverified Duo certifier connection never uses a proxy and only dials loopback addresses.
func TestNewCertifierClient_LoopbackBypassesProxyAndPinsLoopback(t *testing.T) {
	oc, _ := offlineTestClient(t)

	for _, certifier := range []string{"https://localhost:15310/c", "https://LocalHost:15310/c", "https://127.0.0.1:15310/c"} {
		transport, ok := newCertifierClient(oc.client, certifier).Transport.(*http.Transport)
		require.True(t, ok)
		assert.Nil(t, transport.Proxy, certifier)
		assert.NotNil(t, transport.DialContext, certifier)
		assert.True(t, transport.TLSClientConfig.InsecureSkipVerify, certifier)
	}

	// Negative path: a remote certifier keeps verification and the default proxy handling.
	transport, ok := newCertifierClient(oc.client, "https://certifier.example.com/c").Transport.(*http.Transport)
	require.True(t, ok)
	assert.NotNil(t, transport.Proxy)
	assert.False(t, transport.TLSClientConfig.InsecureSkipVerify)
}

func TestRequireLoopbackDial(t *testing.T) {
	assert.NoError(t, requireLoopbackDial("tcp", "127.0.0.1:15310", nil))
	assert.NoError(t, requireLoopbackDial("tcp6", "[::1]:15310", nil))
	assert.ErrorIs(t, requireLoopbackDial("tcp", "10.0.0.1:15310", nil), errCertifierNonLoopbackDial)
	assert.ErrorIs(t, requireLoopbackDial("tcp", "[2001:db8::1]:443", nil), errCertifierNonLoopbackDial)
	assert.Error(t, requireLoopbackDial("tcp", "no-port", nil))
}

// Regression: a Duo U2F device whose read never returns must not block the flow past the timeout, and the
// device must not be closed while that read is still in flight.
func TestDuoU2FChallenge_HungDeviceTimesOut(t *testing.T) {
	origTimeout, origInterval := u2fAuthTimeout, u2fPollInterval                      // gitleaks:allow -- Test timing variables, not secrets.
	u2fAuthTimeout, u2fPollInterval = 100*time.Millisecond, 5*time.Millisecond        // gitleaks:allow -- Test timing variables, not secrets.
	t.Cleanup(func() { u2fAuthTimeout, u2fPollInterval = origTimeout, origInterval }) // gitleaks:allow -- Test timing variables, not secrets.

	release := make(chan struct{})
	closed := make(chan struct{})
	device := &mocks.U2FDevice{}
	device.On("Authenticate", mock.Anything).Run(func(mock.Arguments) { <-release }).Return(nil, fmt.Errorf("device unplugged")).Once()
	device.On("Close").Run(func(mock.Arguments) { close(closed) }).Return().Once()
	t.Cleanup(func() {
		select {
		case <-release:
		default:
			close(release)
		}
	})

	type result struct {
		resp *ResponseData
		err  error
	}
	done := make(chan result, 1)
	go func() {
		resp, err := (&DUOU2FClient{Device: device}).ChallengeU2F()
		done <- result{resp, err}
	}()
	select {
	case r := <-done:
		require.ErrorIs(t, r.err, ErrU2FAuthTimeout)
		assert.Nil(t, r.resp)
	case <-time.After(reviewTestWait):
		t.Fatal("ChallengeU2F stayed blocked on the device read")
	}

	select {
	case <-closed:
		t.Fatal("device was closed while its Authenticate read was still in flight")
	case <-time.After(50 * time.Millisecond):
	}
	close(release)
	select {
	case <-closed:
	case <-time.After(reviewTestWait):
		t.Fatal("device was never closed after the blocked read returned")
	}
	device.AssertExpectations(t)
}

// Negative path: a Duo U2F device that keeps asking for a touch times out between polls with the same sentinel.
func TestDuoU2FChallenge_PresenceTimeoutBetweenPolls(t *testing.T) {
	origTimeout, origInterval := u2fAuthTimeout, u2fPollInterval                      // gitleaks:allow -- Test timing variables, not secrets.
	u2fAuthTimeout, u2fPollInterval = 50*time.Millisecond, 5*time.Millisecond         // gitleaks:allow -- Test timing variables, not secrets.
	t.Cleanup(func() { u2fAuthTimeout, u2fPollInterval = origTimeout, origInterval }) // gitleaks:allow -- Test timing variables, not secrets.

	closed := make(chan struct{})
	device := &mocks.U2FDevice{}
	device.On("Authenticate", mock.Anything).Return(nil, &u2fhost.TestOfUserPresenceRequiredError{})
	device.On("Close").Run(func(mock.Arguments) { close(closed) }).Return().Once()

	got, err := (&DUOU2FClient{Device: device}).ChallengeU2F()
	require.ErrorIs(t, err, ErrU2FAuthTimeout)
	assert.Nil(t, got)
	select {
	case <-closed:
	case <-time.After(reviewTestWait):
		t.Fatal("device was never closed")
	}
}
