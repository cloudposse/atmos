package okta

import (
	"context"
	"net/http"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/auth/providers/aws/saml2aws/creds"
)

const (
	faultOrgURL = "https://okta.example.com"
	faultAppURL = faultOrgURL + pathApp
)

func faultLogin() *creds.LoginDetails {
	return &creds.LoginDetails{URL: faultAppURL, Username: "user@example.com", Password: "test123"} // gitleaks:allow -- test fixture, not a credential.
}

// sessionAPI answers the Okta session endpoints with an active session.
func sessionAPI(extra map[string]func(int, *http.Request) string) *routes {
	h := map[string]func(int, *http.Request) string{
		pathSessions:  static(`{"id":"sid-new","status":"ACTIVE"}`),
		pathSessionMe: static(`{"status":"ACTIVE"}`),
	}
	for k, v := range extra {
		h[k] = v
	}
	return newRoutes(h)
}

func TestSessionAPIs_FailureModes(t *testing.T) {
	withCredentialsHelper(t, &recordingHelper{})

	tests := []struct {
		name    string
		url     string
		fault   func(*http.Request) faultKind
		body    string
		call    func(oc *Client, ld *creds.LoginDetails) error
		wantErr string
	}{
		{
			name: "createSession rejects an unparseable URL",
			url:  badURL,
			call: func(oc *Client, ld *creds.LoginDetails) error {
				_, _, err := oc.createSession(ld, "session-token-1") // gitleaks:allow -- test fixture, not a credential.
				return err
			},
			wantErr: "error building okta url",
		},
		{
			name:  "createSession reports a transport failure",
			fault: faultAt(faultTransport, pathSessions, 0),
			call: func(oc *Client, ld *creds.LoginDetails) error {
				_, _, err := oc.createSession(ld, "session-token-1") // gitleaks:allow -- test fixture, not a credential.
				return err
			},
			wantErr: "error retrieving session response",
		},
		{
			name:  "createSession reports an unreadable body",
			fault: faultAt(faultBody, pathSessions, 0),
			call: func(oc *Client, ld *creds.LoginDetails) error {
				_, _, err := oc.createSession(ld, "session-token-1") // gitleaks:allow -- test fixture, not a credential.
				return err
			},
			wantErr: "error retrieving body from response",
		},
		{
			name: "createSession surfaces an MFA failure for MFA_REQUIRED",
			body: `{"id":"sid-new","status":"MFA_REQUIRED"}`,
			call: func(oc *Client, ld *creds.LoginDetails) error {
				_, _, err := oc.createSession(ld, "session-token-1") // gitleaks:allow -- test fixture, not a credential.
				return err
			},
			wantErr: "error verifying MFA",
		},
		{
			name:    "validateSession rejects an unparseable URL",
			url:     badURL,
			call:    func(oc *Client, ld *creds.LoginDetails) error { return oc.validateSession(ld) },
			wantErr: "error building oktaURL",
		},
		{
			name:    "validateSession reports a transport failure",
			fault:   faultAt(faultTransport, pathSessionMe, 0),
			call:    func(oc *Client, ld *creds.LoginDetails) error { return oc.validateSession(ld) },
			wantErr: "error retrieving session response",
		},
		{
			name:    "validateSession reports an unreadable body",
			fault:   faultAt(faultBody, pathSessionMe, 0),
			call:    func(oc *Client, ld *creds.LoginDetails) error { return oc.validateSession(ld) },
			wantErr: "error retrieving body from response",
		},
		{
			name:    "getDeviceTokenFromOkta rejects an unparseable URL",
			url:     badURL,
			call:    func(oc *Client, ld *creds.LoginDetails) error { _, err := oc.getDeviceTokenFromOkta(ld); return err },
			wantErr: "error building device token request",
		},
		{
			name:    "getDeviceTokenFromOkta reports a transport failure",
			fault:   faultAt(faultTransport, pathApp, 0),
			call:    func(oc *Client, ld *creds.LoginDetails) error { _, err := oc.getDeviceTokenFromOkta(ld); return err },
			wantErr: "error retrieving device token",
		},
		{
			name:    "primaryAuth rejects an unparseable URL",
			url:     badURL,
			call:    func(oc *Client, ld *creds.LoginDetails) error { _, _, _, err := oc.primaryAuth(ld); return err },
			wantErr: "error building oktaURL",
		},
		{
			name:    "primaryAuth reports an unreadable body",
			fault:   faultAt(faultBody, pathAuthn, 0),
			call:    func(oc *Client, ld *creds.LoginDetails) error { _, _, _, err := oc.primaryAuth(ld); return err },
			wantErr: "error retrieving body from response",
		},
		{
			name:  "authWithSession reports a transport failure on the app request",
			fault: faultAt(faultTransport, pathApp, 0),
			call: func(oc *Client, ld *creds.LoginDetails) error {
				_, err := oc.authWithSession(ld)
				return err
			},
			wantErr: "error authing with session",
		},
		{
			name:  "authWithSession reports an unreadable app page",
			fault: faultAt(faultBody, pathApp, 0),
			call: func(oc *Client, ld *creds.LoginDetails) error {
				_, err := oc.authWithSession(ld)
				return err
			},
			wantErr: "failed to build document from response",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			sessionBody := tc.body
			handler := sessionAPI(map[string]func(int, *http.Request) string{
				pathApp:   static(`<html></html>`),
				pathAuthn: static(`{"status":"SUCCESS"}`),
			})
			if sessionBody != "" {
				handler.h[pathSessions] = static(sessionBody)
			}
			oc := newFaultClient(t, handler, tc.fault)
			ld := faultLogin()
			if tc.url != "" {
				ld.URL = tc.url
			}
			ld.OktaSessionCookie = "sid-1"

			err := tc.call(oc, ld)
			require.Error(t, err)
			assert.ErrorContains(t, err, tc.wantErr)
		})
	}
}

// Recovery path: when only the first app page is unreadable, authWithSession requests the app again and
// returns the assertion from the second response.
func TestAuthWithSession_RecoversFromOneUnreadableAppPage(t *testing.T) {
	var appRequests atomic.Int32
	failFirst := faultAt(faultBody, pathApp, 1)
	fault := func(r *http.Request) faultKind {
		if r.URL.Path == pathApp {
			appRequests.Add(1)
		}
		return failFirst(r)
	}
	handler := sessionAPI(map[string]func(int, *http.Request) string{
		pathApp: static(samlTargetPage("https://aws.example.com/saml", encodedSAML())),
	})
	oc := newFaultClient(t, handler, fault)
	oc.targetURL = "https://aws.example.com/saml"
	ld := faultLogin()
	ld.OktaSessionCookie = "sid-1"

	got, err := oc.authWithSession(ld)
	require.NoError(t, err)
	assert.Equal(t, encodedSAML(), got)
	assert.Equal(t, int32(2), appRequests.Load())
}

func TestFollow_FailureModes(t *testing.T) {
	tests := []struct {
		name    string
		page    string
		fault   func(*http.Request) faultKind
		wantErr string
	}{
		{name: "transport failure", fault: faultAt(faultTransport, pathApp, 0), wantErr: "error following"},
		{name: "unreadable page", fault: faultAt(faultBody, pathApp, 0), wantErr: "failed to build document from response"},
		{
			name:    "SAMLRequest outside any form cannot be redirected",
			page:    `<html><input name="SAMLRequest" value="req"/></html>`,
			wantErr: "error extracting redirect form",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			oc := newFaultClient(t, newRoutes(map[string]func(int, *http.Request) string{pathApp: static(tc.page)}), tc.fault)
			req, err := http.NewRequest(http.MethodGet, faultAppURL, nil)
			require.NoError(t, err)

			_, err = oc.follow(context.Background(), req, faultLogin())
			require.Error(t, err)
			assert.ErrorContains(t, err, tc.wantErr)
		})
	}
}

func TestFollow_ReturnsSAMLResponseWhenCalledRecursively(t *testing.T) {
	saml := encodedSAML()
	oc := newFaultClient(t, newRoutes(map[string]func(int, *http.Request) string{
		pathApp: static(samlTargetPage("https://aws.example.com/saml", saml)),
	}), nil)
	oc.targetURL = "https://aws.example.com/saml"

	// The markers are only used for diagnostics; the SAML response must be returned unchanged.
	ctx := context.WithValue(context.Background(), ctxKey("follow"), true)
	ctx = context.WithValue(ctx, ctxKey("authWithSession"), true)
	req, err := http.NewRequest(http.MethodGet, faultAppURL, nil)
	require.NoError(t, err)

	got, err := oc.follow(ctx, req, faultLogin())
	require.NoError(t, err)
	assert.Equal(t, saml, got)
}

// A page without a form is the Okta step-up page: follow fetches the state token and authenticates again with it.
func TestFollow_StepUpPageAuthenticatesWithStateToken(t *testing.T) {
	saml := encodedSAML()
	handler := newRoutes(map[string]func(int, *http.Request) string{
		pathApp: func(call int, _ *http.Request) string {
			if call == 1 {
				return `<html><body>verification required</body></html>`
			}
			return `<script>var stateToken = 'st-step-up';</script>` // gitleaks:allow -- test fixture, not a credential.
		},
		"/login/sessionCookieRedirect": static(samlTargetPage("https://aws.example.com/saml", saml)),
	})
	handler.h[pathAuthn] = func(_ int, r *http.Request) string {
		body := decodeJSONBody(t, r)
		assert.Equal(t, "st-step-up", body["stateToken"])
		assert.Empty(t, body["password"], "the state token replaces the credentials")
		return `{"status":"SUCCESS","sessionToken":"st-session"}` // gitleaks:allow -- Fake session token in a test fixture.
	}

	oc := newFaultClient(t, handler, nil)
	oc.disableSessions = true
	oc.targetURL = "https://aws.example.com/saml"
	req, err := http.NewRequest(http.MethodGet, faultAppURL, nil)
	require.NoError(t, err)
	ld := faultLogin()

	got, err := oc.follow(context.Background(), req, ld)
	require.NoError(t, err)
	assert.Equal(t, saml, got)
	assert.Equal(t, "st-step-up", ld.StateToken)
	assert.Equal(t, 2, handler.calls(pathApp))
	assert.Equal(t, 1, handler.calls(pathAuthn))
}

func TestGetStateToken_FailureModes(t *testing.T) {
	tests := []struct {
		name    string
		url     string
		fault   func(*http.Request) faultKind
		wantErr string
	}{
		{name: "unparseable URL", url: badURL, wantErr: "error building app request"},
		{name: "transport failure", fault: faultAt(faultTransport, pathApp, 0), wantErr: "error retrieving app response"},
		{name: "unreadable body", fault: faultAt(faultBody, pathApp, 0), wantErr: "error retrieving body from response"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			oc := newFaultClient(t, newRoutes(map[string]func(int, *http.Request) string{pathApp: static(`<html></html>`)}), tc.fault)
			req, err := http.NewRequest(http.MethodGet, faultAppURL, nil)
			require.NoError(t, err)
			ld := faultLogin()
			if tc.url != "" {
				ld.URL = tc.url
			}

			_, err = oc.getStateToken(req, ld)
			require.Error(t, err)
			assert.ErrorContains(t, err, tc.wantErr)
		})
	}
}

func TestNewU2FDeviceFinder_DefaultsToHardwareFinder(t *testing.T) {
	assert.IsType(t, &U2FDeviceFinder{}, newU2FDeviceFinder())
}
