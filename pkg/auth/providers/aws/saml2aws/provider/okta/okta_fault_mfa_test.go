package okta

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"runtime"
	"testing"

	"github.com/marshallbrekka/go-u2fhost"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/cloudposse/atmos/pkg/auth/providers/aws/saml2aws/creds"
	"github.com/cloudposse/atmos/pkg/auth/providers/aws/saml2aws/mocks"
)

const faultVerifyBase = faultOrgURL + "/verify/"

func TestVerifyMfa_RequestFailures(t *testing.T) {
	errPrompt := errors.New("prompt failed")
	totp := testFactor{id: "totp", provider: "GOOGLE", factorType: "token:software:totp"}

	tests := []struct {
		name     string
		factor   testFactor
		href     string
		mfaToken string
		fault    func(*http.Request) faultKind
		prompt   func(pr *mocks.MockPrompter)
		wantErr  string
	}{
		{
			name:   "yubikey prompt failure stops authentication",
			factor: testFactor{id: "y", provider: "YUBICO", factorType: "token:hardware"},
			prompt: func(pr *mocks.MockPrompter) {
				pr.EXPECT().Password("Press the button on your yubikey").Return("", errPrompt).Times(1)
			},
			wantErr: "error reading the yubikey code",
		},
		{name: "unparseable verify URL", factor: totp, href: badURL, wantErr: "error building verify request"},
		{name: "challenge transport failure", factor: totp, fault: faultAt(faultTransport, "/verify/totp", 1), wantErr: "error retrieving verify response"},
		{name: "challenge unreadable body", factor: totp, fault: faultAt(faultBody, "/verify/totp", 1), wantErr: "error retrieving body from response"},
		{
			name:     "code submission transport failure",
			factor:   totp,
			mfaToken: "123456",
			fault:    faultAt(faultTransport, "/verify/totp", 2),
			wantErr:  "error retrieving token post response",
		},
		{
			name:     "code submission unreadable body",
			factor:   totp,
			mfaToken: "123456",
			fault:    faultAt(faultBody, "/verify/totp", 2),
			wantErr:  "error retrieving body from response",
		},
		{
			name:     "code submission without a session token reports the status",
			factor:   totp,
			mfaToken: "123456",
			wantErr:  "received status is: \"MFA_CHALLENGE\"",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if tc.prompt != nil {
				tc.prompt(withMockPrompter(t))
			}
			handler := newRoutes(map[string]func(int, *http.Request) string{
				"/verify/" + tc.factor.id: static(`{"status":"MFA_CHALLENGE"}`),
			})
			factor := tc.factor
			factor.href = faultVerifyBase + factor.id
			if tc.href != "" {
				factor.href = tc.href
			}

			oc := newFaultClient(t, handler, tc.fault)
			resp := mfaRequiredResponse(t, "state-1", factor)
			_, err := verifyMfa(oc, "okta.example.com", &creds.LoginDetails{MFAToken: tc.mfaToken}, resp)
			require.Error(t, err)
			assert.ErrorContains(t, err, tc.wantErr)
			if tc.prompt != nil {
				assert.ErrorIs(t, err, errPrompt)
			}
		})
	}
}

func TestVerifyMfa_MultipleMatchesPromptFailure(t *testing.T) {
	errPrompt := errors.New("interrupt")
	pr := withMockPrompter(t)
	pr.EXPECT().Choose("Multiple FIDO MFA options found. Select which MFA option to use", gomock.Any()).Return(0, errPrompt).Times(1)

	handler := newRoutes(nil)
	oc := newFaultClient(t, handler, nil)
	oc.mfa = "FIDO"
	resp := mfaRequiredResponse(t, "state-1",
		testFactor{id: "a", provider: "FIDO", factorType: "webauthn", href: faultVerifyBase + "a"},
		testFactor{id: "b", provider: "FIDO", factorType: "webauthn", href: faultVerifyBase + "b"},
	)

	_, err := verifyMfa(oc, "okta.example.com", &creds.LoginDetails{}, resp)
	require.ErrorIs(t, err, errPrompt)
	assert.ErrorContains(t, err, "error selecting the MFA option")
	assert.Zero(t, handler.calls("/verify/a")+handler.calls("/verify/b"), "no factor is challenged after a failed selection")
}

func TestVerifyMfa_PushPollRequestFailure(t *testing.T) {
	withFastPolling(t)
	handler := newRoutes(map[string]func(int, *http.Request) string{
		"/verify/push": static(`{"status":"MFA_CHALLENGE","factorResult":"WAITING"}`),
	})
	// The first request starts the challenge, the second is the status poll.
	oc := newFaultClient(t, handler, faultAt(faultTransport, "/verify/push", 2))
	resp := mfaRequiredResponse(t, "state-1", testFactor{id: "push", provider: "OKTA", factorType: "push", href: faultVerifyBase + "push"})

	_, err := verifyMfa(oc, "okta.example.com", &creds.LoginDetails{}, resp)
	require.Error(t, err)
	assert.ErrorContains(t, err, "error retrieving verify response")
	assert.Equal(t, 1, handler.calls("/verify/push"), "the failed poll never reaches the server")
}

func TestVerifyMfa_FidoFailureModes(t *testing.T) {
	fido := func(id string) testFactor {
		return testFactor{id: id, provider: "FIDO", factorType: "webauthn", href: faultVerifyBase + id}
	}
	signed := &u2fhost.AuthenticateResponse{ClientData: "cd", SignatureData: "sig", AuthenticatorData: "ad"}

	t.Run("no hardware key falls back to the system WebAuthn API", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skipf("the system WebAuthn API is available on %s and would prompt for a real authenticator", runtime.GOOS)
		}
		withDeviceFinder(t, failingFinder(gomock.NewController(t), errNoDeviceFound))
		oc := newFaultClient(t, newRoutes(map[string]func(int, *http.Request) string{
			"/verify/f": static(webauthnChallenge(faultOrgURL+"/finish", "cred-1")),
		}), nil)

		_, err := verifyMfa(oc, "okta.example.com", &creds.LoginDetails{}, mfaRequiredResponse(t, "state-1", fido("f")))
		require.Error(t, err)
		assert.ErrorContains(t, err, "not supported on this platform")
	})

	tests := []struct {
		name    string
		device  func(d *mocks.MockU2FDevice)
		next    string
		fault   func(*http.Request) faultKind
		extra   bool
		wantErr string
	}{
		{
			name:    "callback URL that cannot be built",
			device:  func(d *mocks.MockU2FDevice) { d.EXPECT().Authenticate(gomock.Any()).Return(signed, nil).Times(1) },
			next:    badURL,
			wantErr: "error building authentication request",
		},
		{
			name:    "callback transport failure",
			device:  func(d *mocks.MockU2FDevice) { d.EXPECT().Authenticate(gomock.Any()).Return(signed, nil).Times(1) },
			next:    faultOrgURL + "/finish",
			fault:   faultAt(faultTransport, "/finish", 0),
			wantErr: "error retrieving verify response",
		},
		{
			name:    "callback unreadable body",
			device:  func(d *mocks.MockU2FDevice) { d.EXPECT().Authenticate(gomock.Any()).Return(signed, nil).Times(1) },
			next:    faultOrgURL + "/finish",
			fault:   faultAt(faultBody, "/finish", 0),
			wantErr: "error retrieving body from response",
		},
		{
			name: "next FIDO option cannot be challenged",
			device: func(d *mocks.MockU2FDevice) {
				d.EXPECT().Authenticate(gomock.Any()).Return(nil, &u2fhost.BadKeyHandleError{}).Times(1)
			},
			next:    faultOrgURL + "/finish",
			fault:   faultAt(faultTransport, "/verify/second", 0),
			extra:   true,
			wantErr: "get mfa challenge failed for U2F device",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			device := mocks.NewMockU2FDevice(gomock.NewController(t))
			device.EXPECT().Close().MinTimes(1) // Unlimited, but at least one call (the former testify AssertExpectations contract).
			tc.device(device)
			withMockDevice(t, device)

			oc := newFaultClient(t, newRoutes(map[string]func(int, *http.Request) string{
				"/verify/first":  static(webauthnChallenge(tc.next, "cred-1")),
				"/verify/second": static(webauthnChallenge(tc.next, "cred-2")),
				"/finish":        static(`{"status":"SUCCESS","sessionToken":"fido-st"}`), // gitleaks:allow -- Fake session token in a test fixture.
			}), tc.fault)

			factors := []testFactor{fido("first")}
			if tc.extra {
				factors = append(factors, fido("second"))
				// Both options match, so the user is asked to pick; pick the first.
				oc.mfa = "FIDO"
				pr := withMockPrompter(t)
				pr.EXPECT().Choose(gomock.Any(), gomock.Any()).Return(0, nil).Times(1)
			}
			_, err := verifyMfa(oc, "okta.example.com", &creds.LoginDetails{}, mfaRequiredResponse(t, "state-1", factors...))
			require.Error(t, err)
			assert.ErrorContains(t, err, tc.wantErr)
		})
	}
}

func TestChallengeSystemWebAuthn_UnsupportedPlatform(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skipf("the system WebAuthn API is available on %s and would prompt for a real authenticator", runtime.GOOS)
	}
	got, err := ChallengeSystemWebAuthn("nonce", "okta.example.com", "state-1")
	require.Error(t, err)
	assert.Nil(t, got, "an unsupported platform must not yield an assertion")
}

func TestCertifierClient_StopsAfterTooManyRedirects(t *testing.T) {
	hits := 0
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		http.Redirect(w, r, r.URL.Path, http.StatusFound)
	}))
	defer ts.Close()

	oc := newFaultClient(t, newRoutes(nil), nil)
	req, err := http.NewRequest(http.MethodGet, ts.URL+"/loop", nil)
	require.NoError(t, err)

	res, err := newCertifierClient(oc.client, ts.URL).Do(req)
	if res != nil {
		_ = res.Body.Close()
	}
	require.Error(t, err)
	assert.ErrorContains(t, err, "too many certifier redirects")
	assert.Equal(t, maxCertifierRedirects, hits, "the redirect chain is bounded")
}
