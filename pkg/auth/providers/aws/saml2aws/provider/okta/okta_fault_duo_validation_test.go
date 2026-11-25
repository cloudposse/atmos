package okta

import (
	"net/http"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const faultDuoSubmitURL = "https://" + duoFaultHost + "/frame/web/v1/auth"

func TestVerifyTrustedCert_RequestFailures(t *testing.T) {
	certPage := func(certifierURL, certsURL string) string {
		return `<form><input name="sid" value="sid-1"><input name="certs_url" value="` + certsURL + `">` +
			`<input name="certs_txid" value="tx-1"><input name="certifier_url" value="` + certifierURL + `"></form>`
	}
	validation := static(`{"stat":"OK"}`)

	tests := []struct {
		name      string
		certifier func(t *testing.T) string
		certsURL  string
		submitURL string
		fault     func(*http.Request) faultKind
		wantErr   string
	}{
		{
			name:      "certifier URL that cannot be requested",
			certifier: func(*testing.T) string { return badURL },
			certsURL:  "https://duo.example.com/certs",
			submitURL: faultDuoSubmitURL,
			wantErr:   "error building cert validation request",
		},
		{
			name:      "online validation URL that cannot be requested",
			certifier: closedServerURL,
			certsURL:  badURL,
			submitURL: faultDuoSubmitURL,
			wantErr:   "error building cert validation request",
		},
		{
			name:      "online validation unreadable body",
			certifier: closedServerURL,
			certsURL:  "https://duo.example.com/certs",
			submitURL: faultDuoSubmitURL,
			fault:     faultAt(faultBody, "/certs", 0),
			wantErr:   "error retrieving body from response",
		},
		{
			name:      "auth URL that cannot be requested",
			certifier: closedServerURL,
			certsURL:  "https://duo.example.com/certs",
			submitURL: badURL,
			wantErr:   "error building authentication request",
		},
		{
			name:      "auth transport failure",
			certifier: closedServerURL,
			certsURL:  "https://duo.example.com/certs",
			submitURL: faultDuoSubmitURL,
			fault:     faultAt(faultTransport, "/frame/web/v1/auth", 0),
			wantErr:   "error retrieving verify response",
		},
		{
			name:      "auth unreadable body",
			certifier: closedServerURL,
			certsURL:  "https://duo.example.com/certs",
			submitURL: faultDuoSubmitURL,
			fault:     faultAt(faultBody, "/frame/web/v1/auth", 0),
			wantErr:   "error parsing document",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			handler := newRoutes(map[string]func(int, *http.Request) string{"/certs": validation, "/frame/web/v1/auth": validation})
			oc := newFaultClient(t, handler, tc.fault)
			doc := mustDoc(t, certPage(tc.certifier(t), tc.certsURL))

			got, err := verifyTrustedCert(oc, doc, duoFaultHost, tc.submitURL, url.Values{"v": {"2.8"}})
			require.Error(t, err)
			assert.Nil(t, got)
			assert.ErrorContains(t, err, tc.wantErr)
		})
	}
}

func TestVerifyEndpointHealth_AuthResubmitFailures(t *testing.T) {
	tests := []struct {
		name      string
		submitURL string
		fault     func(*http.Request) faultKind
		wantErr   string
	}{
		{name: "auth URL that cannot be requested", submitURL: badURL, wantErr: "error building authentication request"},
		{name: "auth transport failure", submitURL: faultDuoSubmitURL, fault: faultAt(faultTransport, "/frame/web/v1/auth", 0), wantErr: "error retrieving verify response"},
		{name: "auth unreadable body", submitURL: faultDuoSubmitURL, fault: faultAt(faultBody, "/frame/web/v1/auth", 0), wantErr: "error parsing document"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			withFastPolling(t)
			empty := static(`{}`)
			handler := newRoutes(map[string]func(int, *http.Request) string{
				"/alive": empty, "/report": empty, "/frame/check_endpoint_app_status": empty, "/frame/web/v1/auth": empty,
			})
			oc := newFaultClient(t, handler, tc.fault)
			doc := mustDoc(t, duoEndpointHealthPage)

			got, err := verifyEndpointHealth(oc, doc, faultDuoSubmitURL, "127.0.0.1:53100", duoFaultHost, tc.submitURL, url.Values{})
			require.Error(t, err)
			assert.Nil(t, got)
			assert.ErrorContains(t, err, tc.wantErr)
			assert.Equal(t, 1, handler.calls("/report"), "the health report is sent before the form is re-submitted")
		})
	}
}
