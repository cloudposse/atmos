package okta

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/auth/providers/aws/saml2aws/cfg"
)

func TestCreateSession(t *testing.T) {
	t.Run("nil input", func(t *testing.T) {
		oc, loginDetails := offlineTestClient(t)
		_, _, err := oc.createSession(nil, "token")
		require.Error(t, err)
		_, _, err = oc.createSession(loginDetails, "")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "nil input")
	})

	tests := []struct {
		name       string
		status     int
		body       string
		addErr     error
		wantErr    string
		wantCookie string
		wantToken  string
	}{
		{name: "active session", status: http.StatusOK, body: `{"id":"sid-1","status":"ACTIVE","sessionToken":"st"}`, wantCookie: "sid-1", wantToken: "st"},
		{name: "unauthorized", status: http.StatusUnauthorized, wantErr: "invalid sessionToken"},
		{name: "server error", status: http.StatusInternalServerError, wantErr: "HTTP Code: 500"},
		{name: "MFA enroll", status: http.StatusOK, body: `{"id":"sid-1","status":"MFA_ENROLL"}`, wantErr: "MFA_ENROLL"},
		{name: "credential store failure", status: http.StatusOK, body: `{"id":"sid-1","status":"ACTIVE"}`, addErr: errors.New("keychain locked"), wantErr: "keychain locked"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			withCredentialsHelper(t, &recordingHelper{addErr: tc.addErr})
			ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, pathSessions, r.URL.Path)
				w.WriteHeader(tc.status)
				_, _ = fmt.Fprint(w, tc.body)
			}))
			defer ts.Close()

			oc, loginDetails := setupTestClient(t, ts, "Auto")
			cookie, token, err := oc.createSession(loginDetails, "session-token")
			if tc.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.wantCookie, cookie)
			assert.Equal(t, tc.wantToken, token)
		})
	}

	t.Run("MFA required is verified", func(t *testing.T) {
		withCredentialsHelper(t, &recordingHelper{})
		ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case pathSessions:
				body := strings.Replace(mfaRequiredResponse(t, "state-1", testFactor{
					id: "sms", provider: "OKTA", factorType: "sms", href: "https://" + r.Host + "/verify/sms",
				}), `"status":"MFA_REQUIRED"`, `"status":"MFA_REQUIRED","id":"sid-mfa"`, 1)
				_, _ = fmt.Fprint(w, body)
			case "/verify/sms":
				_, _ = fmt.Fprint(w, `{"status":"SUCCESS","sessionToken":"st-after-mfa"}`)
			default:
				t.Errorf("unexpected request %s", r.URL.Path)
			}
		}))
		defer ts.Close()

		oc, loginDetails := setupTestClient(t, ts, "Auto")
		loginDetails.MFAToken = "111111"
		cookie, token, err := oc.createSession(loginDetails, "session-token")
		require.NoError(t, err)
		assert.Equal(t, "sid-mfa", cookie)
		assert.Equal(t, "st-after-mfa", token)
	})
}

func TestValidateSession(t *testing.T) {
	t.Run("nil input", func(t *testing.T) {
		oc, _ := offlineTestClient(t)
		require.Error(t, oc.validateSession(nil))
	})

	tests := []struct {
		name    string
		status  int
		body    string
		wantErr string
	}{
		{name: "active", status: http.StatusOK, body: `{"status":"ACTIVE"}`},
		{name: "unknown status is accepted", status: http.StatusOK, body: `{"status":"SOMETHING"}`},
		{name: "invalid session", status: http.StatusNotFound, wantErr: "invalid okta session"},
		{name: "MFA enroll", status: http.StatusOK, body: `{"status":"MFA_ENROLL"}`, wantErr: "MFA_ENROLL"},
		{name: "MFA required with unsupported factor", status: http.StatusOK, body: `{"status":"MFA_REQUIRED"}`, wantErr: "error verifying MFA"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, pathSessionMe, r.URL.Path)
				c, err := r.Cookie("sid")
				if assert.NoError(t, err) {
					assert.Equal(t, "sid-1", c.Value)
				}
				w.WriteHeader(tc.status)
				_, _ = fmt.Fprint(w, tc.body)
			}))
			defer ts.Close()

			oc, loginDetails := setupTestClient(t, ts, "Auto")
			loginDetails.OktaSessionCookie = "sid-1"
			err := oc.validateSession(loginDetails)
			if tc.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.wantErr)
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestGetDeviceTokenFromOkta(t *testing.T) {
	tests := []struct {
		name    string
		cookie  *http.Cookie
		want    string
		wantErr bool
	}{
		{name: "device token cookie present", cookie: &http.Cookie{Name: "DT", Value: "device-123"}, want: "device-123"},
		{name: "device token cookie absent", cookie: &http.Cookie{Name: "other", Value: "x"}, wantErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				http.SetCookie(w, tc.cookie)
			}))
			defer ts.Close()

			oc, loginDetails := setupTestClient(t, ts, "Auto")
			got, err := oc.getDeviceTokenFromOkta(loginDetails)
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestNew_ConfiguresClient(t *testing.T) {
	idpAccount := cfg.NewIDPAccount()
	idpAccount.URL = "https://idp.example.com/app"
	idpAccount.MFA = "PUSH"
	idpAccount.TargetURL = "https://target.example.com/saml"

	oc, err := New(idpAccount)
	require.NoError(t, err)
	assert.Equal(t, "PUSH", oc.mfa)
	assert.Equal(t, "https://target.example.com/saml", oc.targetURL)
	assert.NotNil(t, oc.client.CheckResponseStatus)
	assert.NotNil(t, oc.client.Jar)
}
