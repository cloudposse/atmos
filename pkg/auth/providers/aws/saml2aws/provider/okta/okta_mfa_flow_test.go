package okta

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/marshallbrekka/go-u2fhost"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/auth/providers/aws/saml2aws/creds"
	"github.com/cloudposse/atmos/pkg/auth/providers/aws/saml2aws/mocks"
)

// verifyServer is a fake Okta factor verification endpoint. Each factor id is served at
// /verify/<id>; the handler receives the decoded JSON body and the 1-based call number.
type verifyServer struct {
	ts      *httptest.Server
	log     requestLog
	mu      sync.Mutex
	handler func(id string, call int, body map[string]any) string
}

func newVerifyServer(t *testing.T, handler func(id string, call int, body map[string]any) string) *verifyServer {
	t.Helper()

	vs := &verifyServer{handler: handler}
	vs.ts = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/verify/") {
			t.Errorf("unexpected request %s", r.URL.Path)
			http.NotFound(w, r)
			return
		}
		assert.Equal(t, "true", r.URL.Query().Get("rememberDevice"))
		call := vs.log.add(r.URL.Path)
		body := decodeJSONBody(t, r)
		vs.mu.Lock()
		h := vs.handler
		vs.mu.Unlock()
		_, _ = fmt.Fprint(w, h(strings.TrimPrefix(r.URL.Path, "/verify/"), call, body))
	}))
	t.Cleanup(vs.ts.Close)
	return vs
}

func (vs *verifyServer) href(id string) string { return vs.ts.URL + "/verify/" + id }

func TestVerifyMfa_PassCodeFactors(t *testing.T) {
	tests := []struct {
		name       string
		provider   string
		factorType string
		mfaToken   string
		prompted   string
		response   string
		wantToken  string
		wantErr    string
	}{
		{name: "google totp with token", provider: "GOOGLE", factorType: "token:software:totp", mfaToken: "111111", response: `{"sessionToken":"st"}`, wantToken: "st"},
		{name: "okta totp prompts for code", provider: "OKTA", factorType: "token:software:totp", prompted: "222222", response: `{"sessionToken":"st"}`, wantToken: "st"},
		{name: "symantec token", provider: "SYMANTEC", factorType: "token", mfaToken: "333333", response: `{"sessionToken":"st"}`, wantToken: "st"},
		{name: "sms rejected code", provider: "OKTA", factorType: "sms", mfaToken: "444444", response: `{"status":"MFA_CHALLENGE"}`, wantErr: `received status is: "MFA_CHALLENGE"`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			pr := withMockPrompter(t)
			expectedCode := tc.mfaToken
			if tc.prompted != "" {
				pr.On("RequestSecurityCode", "000000").Return(tc.prompted).Once()
				expectedCode = tc.prompted
			}

			vs := newVerifyServer(t, func(_ string, call int, body map[string]any) string {
				assert.Equal(t, "state-1", body["stateToken"])
				assert.Equal(t, "true", body["rememberDevice"])
				if call == 1 {
					// The challenge request never carries a pass code.
					assert.NotContains(t, body, "passCode")
					return `{"status":"MFA_CHALLENGE"}`
				}
				assert.Equal(t, expectedCode, body["passCode"])
				return tc.response
			})

			oc, _ := offlineTestClient(t)
			resp := mfaRequiredResponse(t, "state-1", testFactor{id: "f1", provider: tc.provider, factorType: tc.factorType, href: vs.href("f1")})
			got, err := verifyMfa(oc, "okta.invalid", &creds.LoginDetails{MFAToken: tc.mfaToken}, resp)
			if tc.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.wantToken, got)
			assert.Equal(t, 2, vs.log.count("/verify/f1"))
		})
	}
}

func TestVerifyMfa_Yubikey(t *testing.T) {
	pr := withMockPrompter(t)
	pr.On("Password", "Press the button on your yubikey").Return("yubi-otp").Once()

	vs := newVerifyServer(t, func(_ string, _ int, body map[string]any) string {
		assert.Equal(t, "yubi-otp", body["passCode"])
		return `{"status":"SUCCESS","sessionToken":"yubi-session"}`
	})

	oc, _ := offlineTestClient(t)
	resp := mfaRequiredResponse(t, "state-1", testFactor{id: "y", provider: "YUBICO", factorType: "token:hardware", href: vs.href("y")})
	got, err := verifyMfa(oc, "okta.invalid", &creds.LoginDetails{}, resp)
	require.NoError(t, err)
	assert.Equal(t, "yubi-session", got)
	assert.Equal(t, 1, vs.log.count("/verify/y"))
}

func TestVerifyMfa_OptionSelection(t *testing.T) {
	successOn := func(wantID string) func(string, int, map[string]any) string {
		return func(id string, _ int, _ map[string]any) string {
			if id != wantID {
				return `{"status":"MFA_CHALLENGE"}`
			}
			return `{"sessionToken":"from-` + id + `"}`
		}
	}

	t.Run("auto with several factors prompts with labelled options", func(t *testing.T) {
		vs := newVerifyServer(t, successOn("totp"))
		pr := withMockPrompter(t)
		pr.On("Choose", "Select which MFA option to use", []string{
			"PUSH MFA authentication - push",
			"TOTP MFA authentication - Authenticator App (totp)",
			"UNSUPPORTED: ACME CARRIER-PIGEON",
		}).Return(1).Once()

		oc, _ := offlineTestClient(t)
		resp := mfaRequiredResponse(t, "state-1",
			testFactor{id: "push", provider: "OKTA", factorType: "push", href: vs.href("push")},
			testFactor{id: "totp", provider: "GOOGLE", factorType: "token:software:totp", href: vs.href("totp"), authName: "Authenticator App"},
			testFactor{id: "bird", provider: "ACME", factorType: "carrier-pigeon", href: vs.href("bird")},
		)
		got, err := verifyMfa(oc, "okta.invalid", &creds.LoginDetails{MFAToken: "123456"}, resp)
		require.NoError(t, err)
		assert.Equal(t, "from-totp", got)
		assert.Zero(t, vs.log.count("/verify/push"))
	})

	t.Run("configured factor type is selected without prompting", func(t *testing.T) {
		vs := newVerifyServer(t, successOn("totp"))
		withMockPrompter(t)

		oc, _ := offlineTestClient(t)
		oc.mfa = "TOTP"
		resp := mfaRequiredResponse(t, "state-1",
			testFactor{id: "push", provider: "OKTA", factorType: "push", href: vs.href("push")},
			testFactor{id: "totp", provider: "GOOGLE", factorType: "token:software:totp", href: vs.href("totp")},
		)
		got, err := verifyMfa(oc, "okta.invalid", &creds.LoginDetails{MFAToken: "123456"}, resp)
		require.NoError(t, err)
		assert.Equal(t, "from-totp", got)
		assert.Zero(t, vs.log.count("/verify/push"))
	})

	t.Run("several factors of the configured type prompt for one of them", func(t *testing.T) {
		vs := newVerifyServer(t, successOn("second"))
		pr := withMockPrompter(t)
		pr.On("Choose", "Multiple OKTA MFA options found. Select which MFA option to use", []string{
			"Okta MFA authentication - first",
			"Okta MFA authentication - second",
		}).Return(1).Once()

		oc, _ := offlineTestClient(t)
		oc.mfa = "OKTA"
		resp := mfaRequiredResponse(t, "state-1",
			testFactor{id: "first", provider: "OKTA", factorType: "token:software:totp", href: vs.href("first")},
			testFactor{id: "second", provider: "OKTA", factorType: "token:software:totp", href: vs.href("second")},
		)
		got, err := verifyMfa(oc, "okta.invalid", &creds.LoginDetails{MFAToken: "123456"}, resp)
		require.NoError(t, err)
		assert.Equal(t, "from-second", got)
		assert.Zero(t, vs.log.count("/verify/first"))
	})

	t.Run("only unsupported factors", func(t *testing.T) {
		oc, _ := offlineTestClient(t)
		resp := mfaRequiredResponse(t, "state-1", testFactor{id: "q", provider: "OKTA", factorType: "question", href: "https://okta.invalid/verify/q"})
		_, err := verifyMfa(oc, "okta.invalid", &creds.LoginDetails{}, resp)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unsupported mfa provider")
	})

	t.Run("unreachable verify endpoint", func(t *testing.T) {
		oc, _ := offlineTestClient(t)
		resp := mfaRequiredResponse(t, "state-1", testFactor{id: "p", provider: "OKTA", factorType: "push", href: closedServerURL(t) + "/verify/p"})
		_, err := verifyMfa(oc, "okta.invalid", &creds.LoginDetails{}, resp)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "error retrieving verify response")
	})
}

func TestFindMfaOption(t *testing.T) {
	options := []string{"PUSH MFA authentication - a", "FIDO WebAuthn MFA authentication - b", "FIDO WebAuthn MFA authentication - c"}

	tests := []struct {
		name  string
		mfa   string
		start int
		want  int
	}{
		{name: "first match", mfa: "FIDO", start: 0, want: 1},
		{name: "start index is inclusive", mfa: "FIDO", start: 1, want: 1},
		{name: "match after start index", mfa: "FIDO", start: 2, want: 2},
		{name: "no match after start index", mfa: "FIDO", start: 3, want: 0},
		{name: "no match at all", mfa: "DUO", start: 0, want: 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, findMfaOption(tc.mfa, options, tc.start))
		})
	}
}

func TestVerifyMfa_Push(t *testing.T) {
	waiting := `{"status":"MFA_CHALLENGE","factorResult":"WAITING"}`

	tests := []struct {
		name      string
		responses []string
		wantToken string
		wantErr   string
	}{
		{name: "immediately approved", responses: []string{`{"status":"SUCCESS","sessionToken":"push-st"}`}, wantToken: "push-st"},
		{name: "approved after waiting", responses: []string{waiting, waiting, `{"status":"SUCCESS","sessionToken":"push-st"}`}, wantToken: "push-st"},
		{name: "timeout", responses: []string{waiting, `{"factorResult":"TIMEOUT"}`}, wantErr: "did not accept MFA in time"},
		{name: "rejected", responses: []string{`{"factorResult":"REJECTED"}`}, wantErr: "MFA rejected by user"},
		{name: "unexpected result", responses: []string{`{"factorResult":"CANCELLED"}`}, wantErr: "Unsupported response from Okta"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			withFastPolling(t)
			vs := newVerifyServer(t, func(_ string, call int, body map[string]any) string {
				assert.Equal(t, "state-1", body["stateToken"])
				if call > len(tc.responses) {
					t.Errorf("unexpected poll #%d", call)
					return `{}`
				}
				return tc.responses[call-1]
			})

			oc, _ := offlineTestClient(t)
			resp := mfaRequiredResponse(t, "state-1", testFactor{id: "push", provider: "OKTA", factorType: "push", href: vs.href("push")})
			got, err := verifyMfa(oc, "okta.invalid", &creds.LoginDetails{}, resp)
			assert.Equal(t, len(tc.responses), vs.log.count("/verify/push"))
			if tc.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.wantToken, got)
		})
	}
}

// webauthnChallenge renders an Okta WebAuthn challenge response for the given credential.
func webauthnChallenge(nextHref, credentialID string) string {
	return fmt.Sprintf(`{"status":"MFA_CHALLENGE","_embedded":{"factor":{"profile":{"credentialId":"%s","version":"U2F_V2"},`+
		`"_embedded":{"challenge":{"challenge":"nonce-%s"}}}},"_links":{"next":{"href":"%s"}}}`, credentialID, credentialID, nextHref)
}

func TestVerifyMfa_FidoWebAuthn(t *testing.T) {
	t.Run("signed assertion is posted to the next link", func(t *testing.T) {
		device := &mocks.U2FDevice{}
		withMockDevice(t, device)
		device.On("Close").Return()
		device.On("Authenticate", mock.MatchedBy(func(r *u2fhost.AuthenticateRequest) bool {
			return r.Challenge == "nonce-cred-1" && r.AppId == "okta.example.com" &&
				r.Facet == "https://okta.example.com" && r.KeyHandle == "cred-1" && r.WebAuthn
		})).Return(&u2fhost.AuthenticateResponse{ClientData: "cd", SignatureData: "sig", AuthenticatorData: "ad"}, nil).Once()

		ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/verify/fido":
				_, _ = fmt.Fprint(w, webauthnChallenge("https://"+r.Host+"/finish", "cred-1"))
			case "/finish":
				body := decodeJSONBody(t, r)
				assert.Equal(t, map[string]any{
					"stateToken": "state-1", "clientData": "cd", "signatureData": "sig", "authenticatorData": "ad",
				}, body)
				_, _ = fmt.Fprint(w, `{"status":"SUCCESS","sessionToken":"fido-st"}`)
			default:
				t.Errorf("unexpected request %s", r.URL.Path)
			}
		}))
		defer ts.Close()

		oc, _ := offlineTestClient(t)
		resp := mfaRequiredResponse(t, "state-1", testFactor{id: "fido", provider: "FIDO", factorType: "webauthn", href: ts.URL + "/verify/fido"})
		got, err := verifyMfa(oc, "okta.example.com", &creds.LoginDetails{}, resp)
		require.NoError(t, err)
		assert.Equal(t, "fido-st", got)
		device.AssertExpectations(t)
	})

	t.Run("bad key handle falls through to the next FIDO device", func(t *testing.T) {
		device := &mocks.U2FDevice{}
		withMockDevice(t, device)
		device.On("Close").Return()
		device.On("Authenticate", mock.MatchedBy(func(r *u2fhost.AuthenticateRequest) bool { return r.KeyHandle == "cred-a" })).
			Return(nil, &u2fhost.BadKeyHandleError{}).Once()
		device.On("Authenticate", mock.MatchedBy(func(r *u2fhost.AuthenticateRequest) bool { return r.KeyHandle == "cred-b" })).
			Return(&u2fhost.AuthenticateResponse{ClientData: "cd-b"}, nil).Once()

		pr := withMockPrompter(t)
		pr.On("Choose", "Multiple FIDO MFA options found. Select which MFA option to use", mock.Anything).Return(0).Once()

		ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/verify/a":
				_, _ = fmt.Fprint(w, webauthnChallenge("https://"+r.Host+"/finish-a", "cred-a"))
			case "/verify/b":
				_, _ = fmt.Fprint(w, webauthnChallenge("https://"+r.Host+"/finish-b", "cred-b"))
			case "/finish-b":
				assert.Equal(t, "cd-b", decodeJSONBody(t, r)["clientData"])
				_, _ = fmt.Fprint(w, `{"sessionToken":"fido-b"}`)
			default:
				t.Errorf("unexpected request %s", r.URL.Path)
			}
		}))
		defer ts.Close()

		oc, _ := offlineTestClient(t)
		oc.mfa = "FIDO"
		resp := mfaRequiredResponse(t, "state-1",
			testFactor{id: "a", provider: "FIDO", factorType: "webauthn", href: ts.URL + "/verify/a", authName: "Key A"},
			testFactor{id: "b", provider: "FIDO", factorType: "webauthn", href: ts.URL + "/verify/b", authName: "Key B"},
		)
		got, err := verifyMfa(oc, "okta.example.com", &creds.LoginDetails{}, resp)
		require.NoError(t, err)
		assert.Equal(t, "fido-b", got)
		device.AssertExpectations(t)
	})

	// Regression: with the default "Auto" setting the next FIDO device was never searched for.
	t.Run("bad key handle falls through to the next FIDO device with Auto", func(t *testing.T) {
		device := &mocks.U2FDevice{}
		withMockDevice(t, device)
		device.On("Close").Return()
		device.On("Authenticate", mock.MatchedBy(func(r *u2fhost.AuthenticateRequest) bool { return r.KeyHandle == "cred-a" })).
			Return(nil, &u2fhost.BadKeyHandleError{}).Once()
		device.On("Authenticate", mock.MatchedBy(func(r *u2fhost.AuthenticateRequest) bool { return r.KeyHandle == "cred-b" })).
			Return(&u2fhost.AuthenticateResponse{ClientData: "cd-b"}, nil).Once()

		pr := withMockPrompter(t)
		pr.On("Choose", "Select which MFA option to use", mock.Anything).Return(0).Once()

		ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/verify/a":
				_, _ = fmt.Fprint(w, webauthnChallenge("https://"+r.Host+"/finish-a", "cred-a"))
			case "/verify/b":
				_, _ = fmt.Fprint(w, webauthnChallenge("https://"+r.Host+"/finish-b", "cred-b"))
			case "/finish-b":
				_, _ = fmt.Fprint(w, `{"sessionToken":"fido-b"}`)
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
		require.NoError(t, err)
		assert.Equal(t, "fido-b", got)
	})

	t.Run("bad key handle on the only device fails", func(t *testing.T) {
		device := &mocks.U2FDevice{}
		withMockDevice(t, device)
		device.On("Close").Return()
		device.On("Authenticate", mock.Anything).Return(nil, &u2fhost.BadKeyHandleError{}).Once()

		ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = fmt.Fprint(w, webauthnChallenge("https://"+r.Host+"/finish", "cred-a"))
		}))
		defer ts.Close()

		oc, _ := offlineTestClient(t)
		oc.mfa = "FIDO"
		resp := mfaRequiredResponse(t, "state-1", testFactor{id: "a", provider: "FIDO", factorType: "webauthn", href: ts.URL + "/verify/a"})
		_, err := verifyMfa(oc, "okta.example.com", &creds.LoginDetails{}, resp)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "tried all MFA options")
	})

	t.Run("device error other than bad key handle fails", func(t *testing.T) {
		device := &mocks.U2FDevice{}
		withMockDevice(t, device)
		device.On("Close").Return()
		device.On("Authenticate", mock.Anything).Return(nil, fmt.Errorf("device unplugged")).Once()

		ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = fmt.Fprint(w, webauthnChallenge("https://"+r.Host+"/finish", "cred-a"))
		}))
		defer ts.Close()

		oc, _ := offlineTestClient(t)
		resp := mfaRequiredResponse(t, "state-1", testFactor{id: "a", provider: "FIDO", factorType: "webauthn", href: ts.URL + "/verify/a"})
		_, err := verifyMfa(oc, "okta.example.com", &creds.LoginDetails{}, resp)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to perform U2F challenge")
	})
}

// Regression: a push that stays in WAITING stops with a timeout error instead of polling forever.
func TestVerifyMfa_PushPollIsBounded(t *testing.T) {
	withFastPolling(t)
	withShortPollTimeout(t)
	vs := newVerifyServer(t, func(string, int, map[string]any) string {
		return `{"status":"MFA_CHALLENGE","factorResult":"WAITING"}`
	})

	oc, _ := offlineTestClient(t)
	resp := mfaRequiredResponse(t, "state-1", testFactor{id: "push", provider: "OKTA", factorType: "push", href: vs.href("push")})
	got, err := verifyMfa(oc, "okta.invalid", &creds.LoginDetails{}, resp)
	require.ErrorIs(t, err, errMFAPollTimeout)
	assert.Empty(t, got)
}

// Regression: a failed prompt (for example Ctrl+C) stops authentication instead of selecting the first
// MFA option or submitting an empty token.
func TestVerifyMfa_PromptFailureStopsAuthentication(t *testing.T) {
	errPrompt := errors.New("interrupt")

	t.Run("option selection", func(t *testing.T) {
		pr := withMockPrompter(t)
		pr.On("Choose", "Select which MFA option to use", mock.Anything).Return(0, errPrompt).Once()

		vs := newVerifyServer(t, func(string, int, map[string]any) string {
			t.Error("no verify request is expected after a failed prompt")
			return `{}`
		})
		oc, _ := offlineTestClient(t)
		resp := mfaRequiredResponse(t, "state-1",
			testFactor{id: "a", provider: "OKTA", factorType: "push", href: vs.href("a")},
			testFactor{id: "b", provider: "OKTA", factorType: "push", href: vs.href("b")},
		)
		_, err := verifyMfa(oc, "okta.invalid", &creds.LoginDetails{}, resp)
		require.ErrorIs(t, err, errPrompt)
	})

	t.Run("security code", func(t *testing.T) {
		pr := withMockPrompter(t)
		pr.On("RequestSecurityCode", "000000").Return("", errPrompt).Once()

		// Only the challenge request is made; the code is never submitted.
		vs := newVerifyServer(t, func(string, int, map[string]any) string {
			return `{"status":"MFA_CHALLENGE"}`
		})
		oc, _ := offlineTestClient(t)
		resp := mfaRequiredResponse(t, "state-1", testFactor{id: "totp", provider: "GOOGLE", factorType: "token:software:totp", href: vs.href("totp")})
		_, err := verifyMfa(oc, "okta.invalid", &creds.LoginDetails{}, resp)
		require.ErrorIs(t, err, errPrompt)
		assert.Equal(t, 1, vs.log.count("/verify/totp"))
	})
}
