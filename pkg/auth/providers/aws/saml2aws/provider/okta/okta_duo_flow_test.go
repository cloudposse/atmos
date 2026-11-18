package okta

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
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

const (
	duoAuthPage = `<html><body><form><input type="hidden" name="sid" value="sid-1">` +
		`<select name="device"><option value="phone1"></option></select></form></body></html>`
	duoPromptOK   = `{"stat":"OK","response":{"txid":"txid-1"}}`
	duoPushed     = `{"stat":"OK","response":{"status":"Pushed a login request","status_code":"pushed"}}`
	duoSuccess    = `{"stat":"OK","response":{"result":"SUCCESS","result_url":"/frame/result/txid-1"}}`
	duoResultOK   = `{"stat":"OK","response":{"cookie":"AUTH|duo-cookie"}}`
	duoFinalOkta  = `{"status":"SUCCESS","sessionToken":"duo-session"}`
	duoSignature  = "TX|tx-sig:APP|app-sig"
	duoFactorID   = "duo-factor"
	duoStateToken = "state-duo"
)

// duoScenario configures a fake Okta + Duo server. Every list is served in order and its last
// element is repeated for any further calls.
type duoScenario struct {
	signature string
	authPages []string
	prompts   []string
	statuses  []string
	result    string

	mu    sync.Mutex
	log   requestLog
	forms map[string][]url.Values
}

func pick(list []string, call int) string {
	if call > len(list) {
		return list[len(list)-1]
	}
	return list[call-1]
}

func (s *duoScenario) recordForm(t *testing.T, r *http.Request) {
	assert.NoError(t, r.ParseForm())
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.forms == nil {
		s.forms = map[string][]url.Values{}
	}
	s.forms[r.URL.Path] = append(s.forms[r.URL.Path], r.PostForm)
}

func (s *duoScenario) formsFor(path string) []url.Values {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.forms[path]
}

func (s *duoScenario) start(t *testing.T) *httptest.Server {
	t.Helper()

	if s.signature == "" {
		s.signature = duoSignature
	}
	if s.result == "" {
		s.result = duoResultOK
	}

	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		call := s.log.add(r.URL.Path)
		switch r.URL.Path {
		case "/verify/duo":
			if call == 1 {
				_, _ = fmt.Fprintf(w, `{"status":"MFA_CHALLENGE","_embedded":{"factor":{"_embedded":{"verification":{`+
					`"host":"%s","signature":"%s","_links":{"complete":{"href":"https://%s/duo/callback"}}}}}}}`,
					r.Host, s.signature, r.Host)
				return
			}
			_, _ = fmt.Fprint(w, duoFinalOkta)
		case "/frame/web/v1/auth":
			s.recordForm(t, r)
			_, _ = fmt.Fprint(w, pick(s.authPages, call))
		case "/frame/prompt":
			s.recordForm(t, r)
			_, _ = fmt.Fprint(w, pick(s.prompts, call))
		case "/frame/status":
			s.recordForm(t, r)
			_, _ = fmt.Fprint(w, pick(s.statuses, call))
		case "/frame/result/txid-1":
			s.recordForm(t, r)
			_, _ = fmt.Fprint(w, s.result)
		case "/duo/callback", "/certifier", "/certs":
			s.recordForm(t, r)
			_, _ = fmt.Fprint(w, `{"stat":"OK"}`)
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(ts.Close)
	return ts
}

func runDuo(t *testing.T, s *duoScenario, option string) (string, error) {
	t.Helper()

	withFastPolling(t)
	ts := s.start(t)
	oc, _ := offlineTestClient(t)
	resp := mfaRequiredResponse(t, duoStateToken, testFactor{id: duoFactorID, provider: "DUO", factorType: "web", href: ts.URL + "/verify/duo"})
	return verifyMfa(oc, "okta.example.com", &creds.LoginDetails{DuoMFAOption: option}, resp)
}

func TestVerifyMfa_DuoFlow(t *testing.T) {
	t.Run("push after polling completes the Okta callback", func(t *testing.T) {
		s := &duoScenario{
			authPages: []string{duoAuthPage},
			prompts:   []string{duoPromptOK},
			statuses: []string{
				duoPushed,
				duoPushed,
				`{"stat":"OK","response":{"result":"SUCCESS","result_url":"/frame/result/txid-1","sid":"sid-2"}}`,
			},
		}
		got, err := runDuo(t, s, "Duo Push")
		require.NoError(t, err)
		assert.Equal(t, "duo-session", got)
		assert.Equal(t, 3, s.log.count("/frame/status"))

		// The refreshed sid from the status poll must be used for the result request.
		results := s.formsFor("/frame/result/txid-1")
		require.Len(t, results, 1)
		assert.Equal(t, "sid-2", results[0].Get("sid"))

		callbacks := s.formsFor("/duo/callback")
		require.Len(t, callbacks, 1)
		assert.Equal(t, duoFactorID, callbacks[0].Get("id"))
		assert.Equal(t, duoStateToken, callbacks[0].Get("stateToken"))
		assert.Equal(t, "AUTH|duo-cookie:APP|app-sig", callbacks[0].Get("sig_response"))
	})

	t.Run("option is chosen interactively when not configured", func(t *testing.T) {
		pr := withMockPrompter(t)
		pr.On("Choose", "Select a DUO MFA Option", []string{"Duo Push", "Passcode"}).Return(1).Once()
		pr.On("StringRequired", "Enter passcode").Return("987654").Once()

		s := &duoScenario{authPages: []string{duoAuthPage}, prompts: []string{duoPromptOK}, statuses: []string{duoSuccess}}
		got, err := runDuo(t, s, "")
		require.NoError(t, err)
		assert.Equal(t, "duo-session", got)

		prompts := s.formsFor("/frame/prompt")
		require.Len(t, prompts, 1)
		assert.Equal(t, "Passcode", prompts[0].Get("factor"))
		assert.Equal(t, "987654", prompts[0].Get("passcode"))
		assert.Equal(t, "sid-1", prompts[0].Get("sid"))
	})

	t.Run("trusted certificate validation is performed before prompting", func(t *testing.T) {
		s := &duoScenario{prompts: []string{duoPromptOK}, statuses: []string{duoSuccess}}
		s.authPages = []string{"", duoAuthPage}
		ts := s.start(t)
		s.authPages[0] = fmt.Sprintf(`<form id="client_cert_form"><input name="sid" value="cert-sid">`+
			`<input name="certs_txid" value="cert-tx"><input name="certs_url" value="%[1]s/certs">`+
			`<input name="certifier_url" value="%[1]s/certifier"></form>`, ts.URL)

		withFastPolling(t)
		oc, _ := offlineTestClient(t)
		resp := mfaRequiredResponse(t, duoStateToken, testFactor{id: duoFactorID, provider: "DUO", factorType: "web", href: ts.URL + "/verify/duo"})
		got, err := verifyMfa(oc, "okta.example.com", &creds.LoginDetails{DuoMFAOption: "Duo Push"}, resp)
		require.NoError(t, err)
		assert.Equal(t, "duo-session", got)
		assert.Equal(t, 1, s.log.count("/certifier"))

		auths := s.formsFor("/frame/web/v1/auth")
		require.Len(t, auths, 2)
		assert.Equal(t, "cert-sid", auths[1].Get("sid"))
		assert.Equal(t, "cert-tx", auths[1].Get("certs_txid"))
	})

	errorCases := []struct {
		name     string
		scenario *duoScenario
		option   string
		wantErr  string
	}{
		{
			name:     "unknown configured option",
			scenario: &duoScenario{authPages: []string{duoAuthPage}},
			option:   "Carrier Pigeon",
			wantErr:  "error unsupported MFA option 'Carrier Pigeon'",
		},
		{
			name:     "auth page without sid",
			scenario: &duoScenario{authPages: []string{`<html><body><form></form></body></html>`}},
			option:   "Duo Push",
			wantErr:  "unable to locate duo sid",
		},
		{
			name:     "malformed verification signature",
			scenario: &duoScenario{signature: "no-colon-here", authPages: []string{duoAuthPage}},
			option:   "Duo Push",
			wantErr:  "invalid duo verification signature",
		},
		{
			name:     "prompt rejected",
			scenario: &duoScenario{authPages: []string{duoAuthPage}, prompts: []string{`{"stat":"FAIL"}`}},
			option:   "Duo Push",
			wantErr:  "error authenticating mfa device",
		},
		{
			name: "push denied while polling",
			scenario: &duoScenario{
				authPages: []string{duoAuthPage},
				prompts:   []string{duoPromptOK},
				statuses:  []string{duoPushed, `{"stat":"OK","response":{"result":"FAILURE","status":"Login request denied"}}`},
			},
			option:  "Duo Push",
			wantErr: "failed to authenticate device",
		},
		{
			name: "result submission rejected",
			scenario: &duoScenario{
				authPages: []string{duoAuthPage},
				prompts:   []string{duoPromptOK},
				statuses:  []string{duoSuccess},
				result:    `{"stat":"FAIL","message":"session expired"}`,
			},
			option:  "Duo Push",
			wantErr: "duoResultSubmit: FAIL session expired",
		},
		{
			name: "result without cookie",
			scenario: &duoScenario{
				authPages: []string{duoAuthPage},
				prompts:   []string{duoPromptOK},
				statuses:  []string{duoSuccess},
				result:    `{"stat":"OK","response":{}}`,
			},
			option:  "Duo Push",
			wantErr: "Unable to get response.cookie",
		},
	}
	for _, tc := range errorCases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := runDuo(t, tc.scenario, tc.option)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.wantErr)
			assert.Empty(t, got)
		})
	}
}

func TestVerifyMfa_DuoU2F(t *testing.T) {
	device := &mocks.U2FDevice{}
	withMockDevice(t, device)
	device.On("Close").Return()
	device.On("Authenticate", mock.MatchedBy(func(r *u2fhost.AuthenticateRequest) bool {
		return r.Challenge == "u2f-nonce" && r.AppId == "duo-app" && r.KeyHandle == "duo-kh" && !r.WebAuthn
	})).Return(&u2fhost.AuthenticateResponse{ClientData: "cd", SignatureData: "sig"}, nil).Once()

	s := &duoScenario{
		authPages: []string{`<form><input name="sid" value="sid-1"><select>` +
			`<option name="webauthn" value="webauthn-token"></option></select></form>`},
		prompts: []string{duoPromptOK, `{"stat":"OK","response":{"txid":"txid-2"}}`},
		statuses: []string{
			`{"stat":"OK","response":{"status_code":"u2f_sent","u2f_sign_request":[{"appId":"duo-app","version":"U2F_V2",` +
				`"challenge":"u2f-nonce","keyHandle":"duo-kh","sessionId":"u2f-session"}]}}`,
			duoSuccess,
		},
	}
	got, err := runDuo(t, s, "U2F Key")
	require.NoError(t, err)
	assert.Equal(t, "duo-session", got)
	device.AssertExpectations(t)

	prompts := s.formsFor("/frame/prompt")
	require.Len(t, prompts, 2)
	assert.Equal(t, "U2F Token", prompts[0].Get("factor"))
	assert.Equal(t, "u2f_token", prompts[0].Get("device"))
	assert.Equal(t, "u2f_finish", prompts[1].Get("factor"))

	var responseData ResponseData
	require.NoError(t, json.Unmarshal([]byte(prompts[1].Get("response_data")), &responseData))
	assert.Equal(t, ResponseData{SessionId: "u2f-session", ClientData: "cd", SignatureData: "sig", KeyHandle: "duo-kh"}, responseData)

	statuses := s.formsFor("/frame/status")
	require.Len(t, statuses, 2)
	assert.Equal(t, "txid-2", statuses[1].Get("txid"))
}

func TestVerifyTrustedCert_Errors(t *testing.T) {
	certForm := func(certsURL, certifierURL string) string {
		return fmt.Sprintf(`<form id="client_cert_form"><input name="sid" value="sid"><input name="certs_txid" value="tx">`+
			`<input name="certs_url" value="%s"><input name="certifier_url" value="%s"></form>`, certsURL, certifierURL)
	}

	t.Run("falls back to the online validator when the local certifier is down", func(t *testing.T) {
		var log requestLog
		ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			log.add(r.URL.Path)
			switch r.URL.Path {
			case "/certs":
				assert.Equal(t, "AJAX", r.URL.Query().Get("type"))
				assert.Equal(t, "tx", r.URL.Query().Get("certs_txid"))
				_, _ = fmt.Fprint(w, `{"stat":"OK"}`)
			case "/submit":
				_, _ = fmt.Fprint(w, `<form><input name="sid" value="after-cert"></form>`)
			default:
				t.Errorf("unexpected request %s", r.URL.Path)
			}
		}))
		defer ts.Close()

		oc, _ := offlineTestClient(t)
		doc, err := verifyTrustedCert(oc, mustDoc(t, certForm(ts.URL+"/certs", closedServerURL(t)+"/certifier")), "duo.example.com", ts.URL+"/submit", url.Values{})
		require.NoError(t, err)
		sid, _ := doc.Find(`input[name="sid"]`).Attr("value")
		assert.Equal(t, "after-cert", sid)
		assert.Equal(t, 1, log.count("/certs"))
	})

	t.Run("validation failure", func(t *testing.T) {
		ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = fmt.Fprint(w, `{"stat":"FAIL"}`)
		}))
		defer ts.Close()

		oc, _ := offlineTestClient(t)
		_, err := verifyTrustedCert(oc, mustDoc(t, certForm(ts.URL+"/certs", ts.URL+"/certifier")), "duo.example.com", ts.URL+"/submit", url.Values{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "error validation certificate")
	})

	t.Run("both validators unreachable", func(t *testing.T) {
		down := closedServerURL(t)
		oc, _ := offlineTestClient(t)
		_, err := verifyTrustedCert(oc, mustDoc(t, certForm(down+"/certs", down+"/certifier")), "duo.example.com", down+"/submit", url.Values{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "error retrieving cert validation response")
	})
}

func TestVerifyEndpointHealth_AliveUnreachable(t *testing.T) {
	oc, _ := offlineTestClient(t)
	down := closedServerURL(t)
	host := down[len("https://"):]

	_, err := verifyEndpointHealth(oc, mustDoc(t, fakeEndpointForm), host, host, host, down+"/submit", url.Values{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to query alive URL")
}

func TestVerifyEndpointHealth_StatusCheckUnreachable(t *testing.T) {
	// Regression: a transport error in the background status check used to panic on a nil response.
	withFastPolling(t)

	var log requestLog
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.add(r.URL.Path)
		if r.URL.Path == "/submit" {
			_, _ = fmt.Fprint(w, `<form><input name="sid" value="after-health"></form>`)
		}
	}))
	defer ts.Close()

	endpointHost := ts.URL[len("https://"):]
	duoHost := closedServerURL(t)[len("https://"):]
	oc, _ := offlineTestClient(t)

	require.NotPanics(t, func() {
		doc, err := verifyEndpointHealth(oc, mustDoc(t, fakeEndpointForm), "https://"+duoHost, endpointHost, duoHost, ts.URL+"/submit", url.Values{})
		require.NoError(t, err)
		sid, _ := doc.Find(`input[name="sid"]`).Attr("value")
		assert.Equal(t, "after-health", sid)
	})
	assert.Equal(t, 1, log.count("/alive"))
	assert.Equal(t, 1, log.count("/report"))
}

// withShortPollTimeout shortens the MFA polling budget for the duration of a test.
func withShortPollTimeout(t *testing.T) {
	t.Helper()

	orig := mfaPollTimeout
	mfaPollTimeout = 20 * time.Millisecond
	t.Cleanup(func() { mfaPollTimeout = orig })
}

// Regression: a Duo push that never reaches a terminal result stops with a timeout error.
func TestVerifyMfa_DuoPushPollIsBounded(t *testing.T) {
	withShortPollTimeout(t)
	s := &duoScenario{authPages: []string{duoAuthPage}, prompts: []string{duoPromptOK}, statuses: []string{duoPushed}}

	got, err := runDuo(t, s, "Duo Push")
	require.ErrorIs(t, err, errMFAPollTimeout)
	assert.Empty(t, got)
	assert.Positive(t, s.log.count("/frame/status"))
}

// Regression: when the health report fails, the blocked status request is cancelled and the report
// error is returned, instead of waiting for a status response that never arrives.
func TestVerifyEndpointHealth_ReportFailureCancelsStatusRequest(t *testing.T) {
	withFastPolling(t)

	statusCancelled := make(chan struct{})
	duoServer := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// check_endpoint_app_status blocks until the client gives up.
		<-r.Context().Done()
		close(statusCancelled)
	}))
	defer duoServer.Close()

	endpoint := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/report" {
			// Drop the connection so the report request fails with a transport error.
			conn, _, err := w.(http.Hijacker).Hijack()
			if err == nil {
				_ = conn.Close()
			}
		}
	}))
	defer endpoint.Close()

	oc, _ := offlineTestClient(t)
	duoHost := duoServer.URL[len("https://"):]
	endpointHost := endpoint.URL[len("https://"):]

	done := make(chan error, 1)
	go func() {
		_, err := verifyEndpointHealth(oc, mustDoc(t, fakeEndpointForm), "https://"+duoHost, endpointHost, duoHost, endpoint.URL+"/submit", url.Values{})
		done <- err
	}()

	select {
	case err := <-done:
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to query report URL")
	case <-time.After(10 * time.Second):
		t.Fatal("verifyEndpointHealth did not return after the report failed")
	}
	select {
	case <-statusCancelled:
	case <-time.After(10 * time.Second):
		t.Fatal("the status request was not cancelled")
	}
}
