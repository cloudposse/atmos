package okta

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/auth/providers/aws/saml2aws/creds"
)

const (
	pathAuthn     = "/api/v1/authn"
	pathSessions  = "/api/v1/sessions"
	pathSessionMe = "/api/v1/sessions/me"
	pathApp       = "/app/sso"
)

// fakeOkta is a configurable fake Okta org used to drive the Authenticate flow end to end.
type fakeOkta struct {
	t   *testing.T
	ts  *httptest.Server
	log requestLog

	// authn returns the JSON body for POST /api/v1/authn given the decoded request body.
	authn func(body map[string]any) string
	// session returns the JSON body for POST /api/v1/sessions given the session token.
	session func(sessionToken string) string
	// sessionMe returns the status code and body for GET /api/v1/sessions/me given the sid cookie.
	sessionMe func(sid string) (int, string)
	// app returns the HTML for GET /app/sso given the per-path call number (1-based).
	app func(call int, r *http.Request) string
	// extra holds additional handlers keyed by path.
	extra map[string]http.HandlerFunc
}

func newFakeOkta(t *testing.T) *fakeOkta {
	t.Helper()

	f := &fakeOkta{t: t, extra: map[string]http.HandlerFunc{}}
	f.ts = httptest.NewTLSServer(http.HandlerFunc(f.serve))
	t.Cleanup(f.ts.Close)
	return f
}

func (f *fakeOkta) serve(w http.ResponseWriter, r *http.Request) {
	call := f.log.add(r.URL.Path)

	if h, ok := f.extra[r.URL.Path]; ok {
		h(w, r)
		return
	}

	switch {
	case r.URL.Path == pathAuthn && f.authn != nil:
		_, _ = fmt.Fprint(w, f.authn(decodeJSONBody(f.t, r)))
	case r.URL.Path == pathSessions && f.session != nil:
		body := decodeJSONBody(f.t, r)
		token, _ := body["sessionToken"].(string)
		_, _ = fmt.Fprint(w, f.session(token))
	case r.URL.Path == pathSessionMe && f.sessionMe != nil:
		sid := ""
		if c, err := r.Cookie("sid"); err == nil {
			sid = c.Value
		}
		code, body := f.sessionMe(sid)
		w.WriteHeader(code)
		_, _ = fmt.Fprint(w, body)
	case r.URL.Path == pathApp && f.app != nil:
		_, _ = fmt.Fprint(w, f.app(call, r))
	default:
		f.t.Errorf("unexpected request: %s %s", r.Method, r.URL.String())
		http.Error(w, "unexpected", http.StatusNotFound)
	}
}

// client returns an Okta client whose target is the fake org and login details pointing at the app URL.
func (f *fakeOkta) client(disableSessions bool) (*Client, *creds.LoginDetails) {
	oc, loginDetails := setupTestClient(f.t, f.ts, "Auto")
	oc.disableSessions = disableSessions
	loginDetails.URL = f.ts.URL + pathApp
	return oc, loginDetails
}

func TestAuthenticate_DisabledSessions(t *testing.T) {
	saml := encodedSAML()

	tests := []struct {
		name       string
		stateToken string
		authn      func(t *testing.T, ts string, body map[string]any) string
		mfaToken   string
		wantErr    string
	}{
		{
			name: "password success redirects with session token",
			authn: func(t *testing.T, _ string, body map[string]any) string {
				assert.Equal(t, "user@example.com", body["username"])
				assert.Equal(t, "test123", body["password"])
				assert.NotContains(t, body, "stateToken")
				return `{"status":"SUCCESS","sessionToken":"st-1"}`
			},
		},
		{
			name:       "state token replaces username and password",
			stateToken: "state-abc",
			authn: func(t *testing.T, _ string, body map[string]any) string {
				assert.Equal(t, "state-abc", body["stateToken"])
				assert.Empty(t, body["username"])
				assert.Empty(t, body["password"])
				return `{"status":"SUCCESS","sessionToken":"st-1"}`
			},
		},
		{
			name: "locked out account",
			authn: func(*testing.T, string, map[string]any) string {
				return `{"status":"LOCKED_OUT"}`
			},
			wantErr: "the account is locked",
		},
		{
			name:     "MFA required with TOTP token",
			mfaToken: "123456",
			authn: func(t *testing.T, ts string, _ map[string]any) string {
				return mfaRequiredResponse(t, "state-1", testFactor{
					id: "totp", provider: "GOOGLE", factorType: "token:software:totp", href: ts + "/verify/totp",
				})
			},
		},
		{
			name: "MFA verification without session token fails",
			authn: func(t *testing.T, ts string, _ map[string]any) string {
				return mfaRequiredResponse(t, "state-1", testFactor{
					id: "bad", provider: "GOOGLE", factorType: "token:software:totp", href: ts + "/verify/bad",
				})
			},
			mfaToken: "000000",
			wantErr:  "error verifying MFA",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f := newFakeOkta(t)
			f.authn = func(body map[string]any) string { return tc.authn(t, f.ts.URL, body) }
			f.extra["/verify/totp"] = func(w http.ResponseWriter, r *http.Request) {
				body := decodeJSONBody(t, r)
				if body["passCode"] != nil {
					assert.Equal(t, tc.mfaToken, body["passCode"])
					_, _ = fmt.Fprint(w, `{"status":"SUCCESS","sessionToken":"st-1"}`)
					return
				}
				_, _ = fmt.Fprint(w, `{"status":"MFA_CHALLENGE"}`)
			}
			f.extra["/verify/bad"] = func(w http.ResponseWriter, _ *http.Request) {
				_, _ = fmt.Fprint(w, `{"status":"MFA_CHALLENGE"}`)
			}
			f.extra["/login/sessionCookieRedirect"] = func(w http.ResponseWriter, r *http.Request) {
				q := r.URL.Query()
				assert.Equal(t, "st-1", q.Get("token"))
				assert.Equal(t, "true", q.Get("checkAccountSetupComplete"))
				assert.Equal(t, f.ts.URL+pathApp, q.Get("redirectUrl"))
				_, _ = fmt.Fprint(w, samlTargetPage(f.ts.URL, saml))
			}

			oc, loginDetails := f.client(true)
			loginDetails.StateToken = tc.stateToken
			loginDetails.MFAToken = tc.mfaToken

			got, err := oc.Authenticate(loginDetails)
			if tc.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.wantErr)
				assert.Zero(t, f.log.count("/login/sessionCookieRedirect"))
				return
			}
			require.NoError(t, err)
			assert.Equal(t, saml, got)
			assert.Equal(t, 1, f.log.count("/login/sessionCookieRedirect"))
			// Sessions are disabled, so the sessions API must never be used.
			assert.Zero(t, f.log.count(pathSessions))
		})
	}
}

func TestAuthenticate_WithSessions(t *testing.T) {
	saml := encodedSAML()

	t.Run("creates and stores a new session", func(t *testing.T) {
		helper := &recordingHelper{}
		withCredentialsHelper(t, helper)

		f := newFakeOkta(t)
		f.authn = func(map[string]any) string { return `{"status":"SUCCESS","sessionToken":"st-1"}` }
		f.session = func(token string) string {
			assert.Equal(t, "st-1", token)
			return `{"id":"sid-1","status":"ACTIVE","sessionToken":"st-2","expiresAt":"2030-01-01T00:00:00Z"}`
		}
		f.sessionMe = func(sid string) (int, string) {
			assert.Equal(t, "sid-1", sid)
			return http.StatusOK, `{"status":"ACTIVE"}`
		}
		f.app = func(int, *http.Request) string { return samlTargetPage(f.ts.URL, saml) }

		oc, loginDetails := f.client(false)
		got, err := oc.Authenticate(loginDetails)
		require.NoError(t, err)
		assert.Equal(t, saml, got)
		assert.Equal(t, "sid-1", loginDetails.OktaSessionCookie)

		stored := helper.stored()
		require.Len(t, stored, 1)
		assert.Equal(t, loginDetails.URL+"/sessionCookie", stored[0].ServerURL)
		assert.Equal(t, "user@example.com", stored[0].Username)
		assert.Equal(t, "sid-1", stored[0].Secret)
	})

	t.Run("reuses a valid existing session without primary auth", func(t *testing.T) {
		f := newFakeOkta(t)
		f.sessionMe = func(sid string) (int, string) {
			assert.Equal(t, "existing-sid", sid)
			return http.StatusOK, `{"status":"ACTIVE"}`
		}
		f.app = func(_ int, r *http.Request) string {
			c, err := r.Cookie("sid")
			if assert.NoError(t, err) {
				assert.Equal(t, "existing-sid", c.Value)
			}
			return samlTargetPage(f.ts.URL, saml)
		}

		oc, loginDetails := f.client(false)
		loginDetails.OktaSessionCookie = "existing-sid"
		got, err := oc.Authenticate(loginDetails)
		require.NoError(t, err)
		assert.Equal(t, saml, got)
		assert.Zero(t, f.log.count(pathAuthn))
		assert.Zero(t, f.log.count(pathSessions))
	})

	t.Run("stale session falls back to primary auth", func(t *testing.T) {
		f := newFakeOkta(t)
		f.authn = func(map[string]any) string { return `{"status":"SUCCESS","sessionToken":"st-1"}` }
		f.session = func(string) string { return `{"id":"fresh-sid","status":"ACTIVE"}` }
		f.sessionMe = func(sid string) (int, string) {
			if sid == "stale-sid" {
				return http.StatusUnauthorized, `{}`
			}
			return http.StatusOK, `{"status":"ACTIVE"}`
		}
		f.app = func(int, *http.Request) string { return samlTargetPage(f.ts.URL, saml) }

		oc, loginDetails := f.client(false)
		loginDetails.OktaSessionCookie = "stale-sid"
		got, err := oc.Authenticate(loginDetails)
		require.NoError(t, err)
		assert.Equal(t, saml, got)
		assert.Equal(t, "fresh-sid", loginDetails.OktaSessionCookie)
		assert.Equal(t, 1, f.log.count(pathAuthn))
	})

	t.Run("step-up page triggers re-authentication with the state token", func(t *testing.T) {
		f := newFakeOkta(t)
		f.authn = func(body map[string]any) string {
			assert.Equal(t, "step-up-token", body["stateToken"])
			return `{"status":"SUCCESS","sessionToken":"st-1"}`
		}
		f.sessionMe = func(string) (int, string) { return http.StatusOK, `{"status":"ACTIVE"}` }
		f.app = func(call int, _ *http.Request) string {
			if call == 1 {
				return `<html><script>var redirect = "/login/step-up/redirect"; var stateToken = 'step-up-token';</script></html>`
			}
			return samlTargetPage(f.ts.URL, saml)
		}

		oc, loginDetails := f.client(false)
		loginDetails.OktaSessionCookie = "sid-1"
		got, err := oc.Authenticate(loginDetails)
		require.NoError(t, err)
		assert.Equal(t, saml, got)
		assert.Equal(t, "step-up-token", loginDetails.StateToken)
		assert.Equal(t, 1, f.log.count(pathAuthn))
	})

	t.Run("step-up page without a state token fails", func(t *testing.T) {
		f := newFakeOkta(t)
		f.sessionMe = func(string) (int, string) { return http.StatusOK, `{"status":"ACTIVE"}` }
		f.app = func(int, *http.Request) string { return `<html>/login/step-up/ but no token</html>` }

		oc, loginDetails := f.client(false)
		loginDetails.OktaSessionCookie = "sid-1"
		_, err := oc.Authenticate(loginDetails)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "cannot find state token")
	})

	t.Run("session creation failure is returned", func(t *testing.T) {
		f := newFakeOkta(t)
		f.authn = func(map[string]any) string { return `{"status":"SUCCESS","sessionToken":"st-1"}` }
		f.extra[pathSessions] = func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusUnauthorized)
		}

		oc, loginDetails := f.client(false)
		_, err := oc.Authenticate(loginDetails)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid sessionToken")
	})

	t.Run("transport error while fetching the app returns an error instead of panicking", func(t *testing.T) {
		f := newFakeOkta(t)
		f.sessionMe = func(string) (int, string) { return http.StatusOK, `{"status":"ACTIVE"}` }
		f.extra[pathApp] = func(w http.ResponseWriter, _ *http.Request) {
			hj, ok := w.(http.Hijacker)
			if !assert.True(t, ok) {
				return
			}
			conn, _, err := hj.Hijack()
			if assert.NoError(t, err) {
				_ = conn.Close()
			}
		}

		oc, loginDetails := f.client(false)
		loginDetails.OktaSessionCookie = "sid-1"
		require.NotPanics(t, func() {
			_, err := oc.Authenticate(loginDetails)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "error authing with session")
		})
	})
}

func TestAuthenticate_InputErrors(t *testing.T) {
	t.Run("unparseable URL", func(t *testing.T) {
		oc, loginDetails := offlineTestClient(t)
		loginDetails.URL = "://missing-scheme"
		_, err := oc.Authenticate(loginDetails)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "error setting device token in cookie jar")
	})

	t.Run("unreachable org", func(t *testing.T) {
		oc, loginDetails := offlineTestClient(t)
		oc.disableSessions = true
		loginDetails.URL = closedServerURL(t) + pathApp
		_, err := oc.Authenticate(loginDetails)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "error retrieving auth response")
	})
}

func TestFollow(t *testing.T) {
	saml := encodedSAML()

	t.Run("follows SAMLRequest, resume and intermediate SAMLResponse forms", func(t *testing.T) {
		f := newFakeOkta(t)
		f.sessionMe = func(string) (int, string) { return http.StatusOK, `{"status":"ACTIVE"}` }
		f.app = func(int, *http.Request) string {
			return fmt.Sprintf(`<form method="post" action="%s/idp/saml"><input name="SAMLRequest" value="req"/></form>`, f.ts.URL)
		}
		f.extra["/idp/saml"] = func(w http.ResponseWriter, r *http.Request) {
			assert.NoError(t, r.ParseForm())
			assert.Equal(t, http.MethodPost, r.Method)
			assert.Equal(t, "req", r.PostForm.Get("SAMLRequest"))
			_, _ = fmt.Fprintf(w, `<form method="post" action="%s/idp/resume"><input name="RelayState" value="rs"/></form>`, f.ts.URL)
		}
		f.extra["/idp/resume"] = func(w http.ResponseWriter, r *http.Request) {
			assert.NoError(t, r.ParseForm())
			assert.Equal(t, "rs", r.PostForm.Get("RelayState"))
			_, _ = fmt.Fprintf(w, `<form method="post" action="%s/acs"><input name="SAMLResponse" value="intermediate"/></form>`, f.ts.URL)
		}
		f.extra["/acs"] = func(w http.ResponseWriter, r *http.Request) {
			assert.NoError(t, r.ParseForm())
			assert.Equal(t, "intermediate", r.PostForm.Get("SAMLResponse"))
			_, _ = fmt.Fprint(w, samlTargetPage(f.ts.URL, saml))
		}

		oc, loginDetails := f.client(false)
		loginDetails.OktaSessionCookie = "sid-1"
		got, err := oc.Authenticate(loginDetails)
		require.NoError(t, err)
		assert.Equal(t, saml, got)
		assert.Equal(t, 1, f.log.count("/acs"))
	})

	tests := []struct {
		name    string
		page    func(target string) string
		wantErr string
	}{
		{
			name:    "invalid base64 SAML response",
			page:    func(target string) string { return samlTargetPage(target, "%%%not-base64%%%") },
			wantErr: "failed to decode saml-response",
		},
		{
			name: "target form without SAML response is unknown",
			page: func(target string) string {
				return fmt.Sprintf(`<form method="post" action="%s"><input name="other" value="x"/></form>`, target)
			},
			wantErr: "Unknown document type",
		},
		{
			name:    "page without forms or state token",
			page:    func(string) string { return `<html><body>nothing to see</body></html>` },
			wantErr: "failed to getStateToken",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f := newFakeOkta(t)
			f.sessionMe = func(string) (int, string) { return http.StatusOK, `{"status":"ACTIVE"}` }
			f.app = func(int, *http.Request) string { return tc.page(f.ts.URL) }

			oc, loginDetails := f.client(false)
			loginDetails.OktaSessionCookie = "sid-1"
			_, err := oc.Authenticate(loginDetails)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.wantErr)
		})
	}
}

func TestDocIsFormRedirectToTarget_DefaultAWSTargets(t *testing.T) {
	for _, target := range []string{
		"https://signin.aws.amazon.com/saml",
		"https://signin.amazonaws-us-gov.com/saml",
		"https://signin.amazonaws.cn/saml",
	} {
		t.Run(target, func(t *testing.T) {
			doc := mustDoc(t, samlTargetPage(target, "x"))
			assert.True(t, docIsFormRedirectToTarget(doc, ""))
			assert.False(t, docIsFormRedirectToTarget(doc, "https://other.example.com/saml"))
		})
	}

	assert.False(t, docIsFormRedirectToTarget(mustDoc(t, samlTargetPage("https://evil.example.com/saml", "x")), ""))
}
