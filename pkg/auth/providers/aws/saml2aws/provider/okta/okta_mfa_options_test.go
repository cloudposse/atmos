package okta

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/marshallbrekka/go-u2fhost"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/auth/providers/aws/saml2aws/creds"
	"github.com/cloudposse/atmos/pkg/auth/providers/aws/saml2aws/mocks"
)

// Regression: when remembering the device is turned off, a factor verify link that already carries
// rememberDevice=true is sent with rememberDevice=false, and its other query parameters are kept.
func TestGetMfaChallengeContext_RememberDeviceQuery(t *testing.T) {
	tests := []struct {
		name     string
		query    string
		remember bool
		want     url.Values
	}{
		{
			name:  "remembering off overrides rememberDevice=true in the link",
			query: "?p=1&rememberDevice=true",
			want:  url.Values{"p": {"1"}, "rememberDevice": {"false"}},
		},
		{
			name:  "remembering off overrides a repeated rememberDevice parameter",
			query: "?rememberDevice=true&rememberDevice=false",
			want:  url.Values{"rememberDevice": {"false"}},
		},
		{
			name:  "remembering off keeps rememberDevice=false in the link",
			query: "?p=1&rememberDevice=false",
			want:  url.Values{"p": {"1"}, "rememberDevice": {"false"}},
		},
		{
			name:  "remembering off is appended to a link without rememberDevice",
			query: "?p=1",
			want:  url.Values{"p": {"1"}, "rememberDevice": {"false"}},
		},
		{
			name:     "remembering on keeps the value already in the link",
			query:    "?rememberDevice=false",
			remember: true,
			want:     url.Values{"rememberDevice": {"false"}},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			sent := make(chan url.Values, 1)
			ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				sent <- r.URL.Query()
				_, _ = fmt.Fprint(w, `{"status":"MFA_CHALLENGE"}`)
			}))
			defer ts.Close()

			oc, _ := offlineTestClient(t)
			oc.rememberDevice = tc.remember
			resp := mfaRequiredResponse(t, "state-1", testFactor{id: "push", provider: "OKTA", factorType: "push", href: ts.URL + "/verify" + tc.query})

			challenge, err := getMfaChallengeContext(oc, 0, resp)
			require.NoError(t, err)
			select {
			case query := <-sent:
				assert.Equal(t, tc.want, query)
			case <-time.After(reviewTestWait):
				t.Fatal("the verify request never reached the server")
			}
			link, err := url.Parse(challenge.oktaVerify)
			require.NoError(t, err)
			assert.Equal(t, "/verify", link.Path)
			assert.Equal(t, tc.want, link.Query())
		})
	}
}

// Regression: after a bad key handle on the selected FIDO factor, every other FIDO factor is tried
// once, including factors listed before the selected one.
func TestVerifyMfa_FidoBadKeyTriesEveryOtherFactorOnce(t *testing.T) {
	tests := []struct {
		name      string
		badKeys   []string
		wantToken string
		wantErr   string
	}{
		{name: "earlier factor accepts the key", badKeys: []string{"cred-b"}, wantToken: "fido-a"},
		{name: "every factor rejects the key", badKeys: []string{"cred-b", "cred-a"}, wantErr: "tried all MFA options"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			device := &mocks.U2FDevice{}
			withMockDevice(t, device)
			device.On("Close").Return()
			for _, key := range tc.badKeys {
				device.On("Authenticate", mock.MatchedBy(func(r *u2fhost.AuthenticateRequest) bool { return r.KeyHandle == key })).
					Return(nil, &u2fhost.BadKeyHandleError{}).Once()
			}
			if tc.wantToken != "" {
				device.On("Authenticate", mock.MatchedBy(func(r *u2fhost.AuthenticateRequest) bool { return r.KeyHandle == "cred-a" })).
					Return(&u2fhost.AuthenticateResponse{ClientData: "cd-a"}, nil).Once()
			}

			// The user selects the second factor.
			pr := withMockPrompter(t)
			pr.On("Choose", "Select which MFA option to use", mock.Anything).Return(1).Once()

			var log requestLog
			ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				log.add(r.URL.Path)
				switch r.URL.Path {
				case "/verify/a":
					_, _ = fmt.Fprint(w, webauthnChallenge("https://"+r.Host+"/finish-a", "cred-a"))
				case "/verify/b":
					_, _ = fmt.Fprint(w, webauthnChallenge("https://"+r.Host+"/finish-b", "cred-b"))
				case "/finish-a":
					_, _ = fmt.Fprint(w, `{"sessionToken":"fido-a"}`)
				default:
					t.Errorf("unexpected request %s", r.URL.Path)
				}
			}))
			defer ts.Close()

			oc, _ := offlineTestClient(t)
			oc.mfa = "Auto"
			resp := mfaRequiredResponse(t, "state-1",
				testFactor{id: "a", provider: "FIDO", factorType: "webauthn", href: ts.URL + "/verify/a", authName: "Key A"},
				testFactor{id: "b", provider: "FIDO", factorType: "webauthn", href: ts.URL + "/verify/b", authName: "Key B"},
			)
			got, err := verifyMfa(oc, "okta.example.com", &creds.LoginDetails{}, resp)
			if tc.wantErr != "" {
				require.ErrorContains(t, err, tc.wantErr)
				assert.Empty(t, got)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tc.wantToken, got)
			}
			// The selected factor is tried first, and no factor is tried twice.
			log.mu.Lock()
			paths := append([]string(nil), log.paths...)
			log.mu.Unlock()
			require.GreaterOrEqual(t, len(paths), 2)
			assert.Equal(t, []string{"/verify/b", "/verify/a"}, paths[:2])
			assert.Equal(t, 1, log.count("/verify/a"))
			assert.Equal(t, 1, log.count("/verify/b"))
			device.AssertExpectations(t)
		})
	}
}
