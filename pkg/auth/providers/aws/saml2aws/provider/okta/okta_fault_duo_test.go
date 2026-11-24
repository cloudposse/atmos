package okta

import (
	"errors"
	"fmt"
	"net/http"
	"sync"
	"testing"

	"github.com/marshallbrekka/go-u2fhost"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/cloudposse/atmos/pkg/auth/providers/aws/saml2aws/creds"
	"github.com/cloudposse/atmos/pkg/auth/providers/aws/saml2aws/mocks"
)

const (
	duoFaultHost = "duo.example.com"
	// duoU2FSent is the Duo status response that asks the client to sign a U2F challenge.
	duoU2FSent = `{"stat":"OK","response":{"status_code":"u2f_sent","u2f_sign_request":[{"appId":"duo-app","version":"U2F_V2",` +
		`"challenge":"u2f-nonce","keyHandle":"duo-kh","sessionId":"u2f-session"}]}}`
	duoU2FAuthPage = `<form><input name="sid" value="sid-1"><select>` +
		`<option name="webauthn" value="webauthn-token"></option></select></form>`
	duoClientCertPage = `<form id="client_cert_form"><input name="sid" value="sid-1">` +
		`<input name="certs_url" value="https://duo.example.com/certs"><input name="certs_txid" value="tx-1">` +
		`<input name="certifier_url" value="%s"></form>`
	duoEndpointHealthPage = `<form id="endpoint-health-form"><input name="sid" value="sid-1"><input name="txid" value="tx-1">` +
		`<input name="eh_service_url" value="https://127.0.0.1:53100/eh"></form>`
)

// duoFake is an in-process Okta + Duo endpoint set. Every list is served in order and its last element
// repeats for later calls.
type duoFake struct {
	host     string
	callback string
	auth     []string
	prompt   []string
	status   []string

	mu        sync.Mutex
	resultSID string
}

func newDuoFake() *duoFake {
	return &duoFake{
		host:     duoFaultHost,
		callback: faultOrgURL + "/duo/callback",
		auth:     []string{duoAuthPage},
		prompt:   []string{duoPromptOK},
		status:   []string{duoPushed, duoSuccess},
	}
}

func (d *duoFake) routes() *routes {
	return newRoutes(map[string]func(int, *http.Request) string{
		"/verify/duo": func(call int, _ *http.Request) string {
			if call == 1 {
				return duoChallengeBody(d.host, duoSignature, d.callback)
			}
			return duoFinalOkta
		},
		"/frame/web/v1/auth": func(call int, _ *http.Request) string { return pick(d.auth, call) },
		"/frame/prompt":      func(call int, _ *http.Request) string { return pick(d.prompt, call) },
		"/frame/status":      func(call int, _ *http.Request) string { return pick(d.status, call) },
		"/frame/result/txid-1": func(_ int, r *http.Request) string {
			_ = r.ParseForm()
			d.mu.Lock()
			d.resultSID = r.PostForm.Get("sid")
			d.mu.Unlock()
			return duoResultOK
		},
		"/duo/callback":                    static(`{"stat":"OK"}`),
		"/certs":                           static(`{"stat":"OK"}`),
		"/alive":                           static(`{}`),
		"/report":                          static(`{}`),
		"/frame/check_endpoint_app_status": static(`{}`),
	})
}

func (d *duoFake) sidSentToResult() string {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.resultSID
}

// run drives verifyMfa against the fake with an optional fault.
func (d *duoFake) run(t *testing.T, fault func(*http.Request) faultKind, option string) (string, *routes, error) {
	t.Helper()

	withFastPolling(t)
	handler := d.routes()
	oc := newFaultClient(t, handler, fault)
	resp := mfaRequiredResponse(t, duoStateToken, testFactor{id: duoFactorID, provider: "DUO", factorType: "web", href: faultVerifyBase + "duo"})
	got, err := verifyMfa(oc, "okta.example.com", &creds.LoginDetails{DuoMFAOption: option}, resp)
	return got, handler, err
}

func TestVerifyMfa_DuoInProcessFlow(t *testing.T) {
	t.Run("a new sid from the status response is used for the result request", func(t *testing.T) {
		d := newDuoFake()
		d.status = []string{`{"stat":"OK","response":{"result":"SUCCESS","result_url":"/frame/result/txid-1","sid":"sid-rotated"}}`}
		got, _, err := d.run(t, nil, "Duo Push")
		require.NoError(t, err)
		assert.Equal(t, "duo-session", got)
		assert.Equal(t, "sid-rotated", d.sidSentToResult())
	})

	t.Run("a new sid from a status poll is used for the result request", func(t *testing.T) {
		d := newDuoFake()
		d.status = []string{duoPushed, `{"stat":"OK","response":{"result":"SUCCESS","result_url":"/frame/result/txid-1","sid":"sid-polled"}}`}
		got, handler, err := d.run(t, nil, "Duo Push")
		require.NoError(t, err)
		assert.Equal(t, "duo-session", got)
		assert.Equal(t, 2, handler.calls("/frame/status"))
		assert.Equal(t, "sid-polled", d.sidSentToResult())
	})

	t.Run("endpoint health check runs and the auth form is submitted again", func(t *testing.T) {
		d := newDuoFake()
		d.auth = []string{duoEndpointHealthPage, duoAuthPage}
		d.status = []string{duoSuccess}
		got, handler, err := d.run(t, nil, "Duo Push")
		require.NoError(t, err)
		assert.Equal(t, "duo-session", got)
		assert.Equal(t, 1, handler.calls("/alive"))
		assert.Equal(t, 1, handler.calls("/report"))
		assert.Equal(t, 2, handler.calls("/frame/web/v1/auth"))
	})
}

func TestVerifyMfa_DuoFailureModes(t *testing.T) {
	errPrompt := errors.New("prompt failed")
	badResultURL := `{"stat":"OK","response":{"result":"SUCCESS","result_url":":bad"}}`

	tests := []struct {
		name    string
		option  string
		setup   func(t *testing.T, d *duoFake)
		fault   func(*http.Request) faultKind
		prompt  func(pr *mocks.MockPrompter)
		wantErr string
	}{
		{name: "duo host that cannot be requested", option: "Duo Push", setup: func(_ *testing.T, d *duoFake) { d.host = "bad host" }, wantErr: "error building authentication request"},
		{name: "auth transport failure", option: "Duo Push", fault: faultAt(faultTransport, "/frame/web/v1/auth", 1), wantErr: "error retrieving verify response"},
		{name: "auth unreadable body", option: "Duo Push", fault: faultAt(faultBody, "/frame/web/v1/auth", 1), wantErr: "error parsing document"},
		{
			name:   "client certificate validation fails",
			option: "Duo Push",
			setup: func(t *testing.T, d *duoFake) {
				d.auth = []string{fmt.Sprintf(duoClientCertPage, closedServerURL(t))}
			},
			fault:   faultAt(faultTransport, "/certs", 0),
			wantErr: "couldn't validate client cert",
		},
		{name: "endpoint health validation fails", option: "Duo Push", setup: func(_ *testing.T, d *duoFake) { d.auth = []string{duoEndpointHealthPage} }, fault: faultAt(faultTransport, "/alive", 0), wantErr: "couldn't validate endpoint health"},
		{
			name: "option prompt failure",
			prompt: func(pr *mocks.MockPrompter) {
				pr.EXPECT().Choose("Select a DUO MFA Option", gomock.Any()).Return(0, errPrompt).Times(1)
			},
			wantErr: "error selecting the DUO MFA option",
		},
		{
			name:   "passcode prompt failure",
			option: "Passcode",
			prompt: func(pr *mocks.MockPrompter) {
				pr.EXPECT().StringRequired("Enter passcode").Return("", errPrompt).Times(1)
			},
			wantErr: "error reading the DUO passcode",
		},
		{name: "prompt transport failure", option: "Duo Push", fault: faultAt(faultTransport, "/frame/prompt", 1), wantErr: "error retrieving verify response"},
		{name: "prompt unreadable body", option: "Duo Push", fault: faultAt(faultBody, "/frame/prompt", 1), wantErr: "error retrieving body from response"},
		{name: "status transport failure", option: "Duo Push", fault: faultAt(faultTransport, "/frame/status", 1), wantErr: "error retrieving verify response"},
		{name: "status unreadable body", option: "Duo Push", fault: faultAt(faultBody, "/frame/status", 1), wantErr: "error retrieving body from response"},
		{name: "poll transport failure", option: "Duo Push", fault: faultAt(faultTransport, "/frame/status", 2), wantErr: "error retrieving verify response"},
		{name: "poll unreadable body", option: "Duo Push", fault: faultAt(faultBody, "/frame/status", 2), wantErr: "error retrieving body from response"},
		{name: "result URL that cannot be requested", option: "Duo Push", setup: func(_ *testing.T, d *duoFake) { d.status = []string{badResultURL} }, wantErr: "error constructing request object to result url"},
		{name: "result transport failure", option: "Duo Push", fault: faultAt(faultTransport, "/frame/result/txid-1", 1), wantErr: "error retrieving duo result response"},
		{name: "result unreadable body", option: "Duo Push", fault: faultAt(faultBody, "/frame/result/txid-1", 1), wantErr: "error retrieving body from response"},
		{name: "okta callback URL that cannot be requested", option: "Duo Push", setup: func(_ *testing.T, d *duoFake) { d.callback = badURL }, wantErr: "error building authentication request"},
		{name: "okta callback transport failure", option: "Duo Push", fault: faultAt(faultTransport, "/duo/callback", 1), wantErr: "error retrieving verify response"},
		{name: "final verify transport failure", option: "Duo Push", fault: faultAt(faultTransport, "/verify/duo", 2), wantErr: "error retrieving verify response"},
		{name: "final verify unreadable body", option: "Duo Push", fault: faultAt(faultBody, "/verify/duo", 2), wantErr: "error retrieving body from response"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if tc.prompt != nil {
				tc.prompt(withMockPrompter(t))
			}
			d := newDuoFake()
			if tc.setup != nil {
				tc.setup(t, d)
			}
			got, _, err := d.run(t, tc.fault, tc.option)
			require.Error(t, err)
			assert.Empty(t, got)
			assert.ErrorContains(t, err, tc.wantErr)
			if tc.prompt != nil {
				assert.ErrorIs(t, err, errPrompt)
			}
		})
	}
}

func TestVerifyMfa_DuoU2FFailureModes(t *testing.T) {
	signed := &u2fhost.AuthenticateResponse{ClientData: "cd", SignatureData: "sig"}
	signOK := func(d *mocks.MockU2FDevice) { d.EXPECT().Authenticate(gomock.Any()).Return(signed, nil).Times(1) }

	tests := []struct {
		name    string
		finder  func(ctrl *gomock.Controller) DeviceFinder
		device  func(d *mocks.MockU2FDevice)
		setup   func(d *duoFake)
		fault   func(*http.Request) faultKind
		wantErr string
		wantIs  error
	}{
		{
			name:    "no U2F device is plugged in",
			finder:  func(ctrl *gomock.Controller) DeviceFinder { return failingFinder(ctrl, errNoDeviceFound) },
			wantIs:  errNoDeviceFound,
			wantErr: "no U2F device",
		},
		{
			name: "device rejects the challenge",
			device: func(d *mocks.MockU2FDevice) {
				d.EXPECT().Authenticate(gomock.Any()).Return(nil, errors.New("device refused")).Times(1)
			},
			wantErr: "device refused",
		},
		{name: "finish prompt transport failure", device: signOK, fault: faultAt(faultTransport, "/frame/prompt", 2), wantErr: "error retrieving verify response"},
		{name: "finish prompt unreadable body", device: signOK, fault: faultAt(faultBody, "/frame/prompt", 2), wantErr: "error retrieving body from response"},
		{
			name:    "finish prompt rejected by Duo",
			device:  signOK,
			setup:   func(d *duoFake) { d.prompt = []string{duoPromptOK, `{"stat":"FAIL"}`} },
			wantErr: "error authenticating mfa device",
		},
		{name: "second status transport failure", device: signOK, fault: faultAt(faultTransport, "/frame/status", 2), wantErr: "error retrieving verify response"},
		{name: "second status unreadable body", device: signOK, fault: faultAt(faultBody, "/frame/status", 2), wantErr: "error retrieving body from response"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			device := mocks.NewMockU2FDevice(ctrl)
			device.EXPECT().Close().AnyTimes()
			if tc.device != nil {
				tc.device(device)
			}
			var finder DeviceFinder = anyTimesFinderFor(ctrl, device)
			if tc.finder != nil {
				finder = tc.finder(ctrl)
			}
			withDeviceFinder(t, finder)

			d := newDuoFake()
			d.auth = []string{duoU2FAuthPage}
			d.prompt = []string{duoPromptOK, `{"stat":"OK","response":{"txid":"txid-2"}}`}
			d.status = []string{duoU2FSent, duoSuccess}
			if tc.setup != nil {
				tc.setup(d)
			}

			got, _, err := d.run(t, tc.fault, "U2F Key")
			require.Error(t, err)
			assert.Empty(t, got)
			assert.ErrorContains(t, err, tc.wantErr)
			if tc.wantIs != nil {
				assert.ErrorIs(t, err, tc.wantIs)
			}
		})
	}

	t.Run("a new sid from the U2F status is used for the result request", func(t *testing.T) {
		device := mocks.NewMockU2FDevice(gomock.NewController(t))
		device.EXPECT().Close().AnyTimes()
		signOK(device)
		withMockDevice(t, device)

		d := newDuoFake()
		d.auth = []string{duoU2FAuthPage}
		d.prompt = []string{duoPromptOK, `{"stat":"OK","response":{"txid":"txid-2"}}`}
		d.status = []string{duoU2FSent, `{"stat":"OK","response":{"result":"SUCCESS","result_url":"/frame/result/txid-1","sid":"sid-u2f"}}`}
		got, _, err := d.run(t, nil, "U2F Key")
		require.NoError(t, err)
		assert.Equal(t, "duo-session", got)
		assert.Equal(t, "sid-u2f", d.sidSentToResult())
	})
}
