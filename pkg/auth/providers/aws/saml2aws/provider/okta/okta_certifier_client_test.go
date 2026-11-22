package okta

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// certifierCertForm renders the Duo client certificate form that verifyTrustedCert reads.
func certifierCertForm(certsURL, certifierURL string) string {
	return fmt.Sprintf(`<form id="client_cert_form"><input name="sid" value="sid"><input name="certs_txid" value="tx">`+
		`<input name="certs_url" value="%s"><input name="certifier_url" value="%s"></form>`, certsURL, certifierURL)
}

// Regression: the certifier request used to swap the shared client's transport for one that skips
// certificate verification, and redirects reused that transport, so a loopback certifier could bounce the
// request to a remote HTTPS host without certificate verification.
func TestVerifyTrustedCert_CertifierClient(t *testing.T) {
	// online serves the online validator and the final submit; it counts validator hits.
	newOnline := func(hits *atomic.Int32) *httptest.Server {
		return httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/certs":
				hits.Add(1)
				_, _ = fmt.Fprint(w, `{"stat":"OK"}`)
			case "/submit":
				_, _ = fmt.Fprint(w, `<form><input name="sid" value="after-cert"></form>`)
			default:
				t.Errorf("unexpected request %s", r.URL.Path)
			}
		}))
	}

	t.Run("loopback certifier works and the shared transport is unchanged", func(t *testing.T) {
		var onlineHits atomic.Int32
		online := newOnline(&onlineHits)
		defer online.Close()
		certifier := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = fmt.Fprint(w, `{"stat":"OK"}`)
		}))
		defer certifier.Close()

		oc, _ := offlineTestClient(t)
		before := oc.client.Transport
		doc, err := verifyTrustedCert(oc, mustDoc(t, certifierCertForm(online.URL+"/certs", certifier.URL+"/certifier")), "duo.example.com", online.URL+"/submit", url.Values{})
		require.NoError(t, err)
		sid, _ := doc.Find(`input[name="sid"]`).Attr("value")
		assert.Equal(t, "after-cert", sid)
		assert.Zero(t, onlineHits.Load(), "the loopback certifier answered, so the online validator is not used")
		assert.Same(t, before, oc.client.Transport, "the shared client transport must not be replaced")
	})

	t.Run("loopback certifier redirecting to a remote host falls back to the online validator", func(t *testing.T) {
		var onlineHits atomic.Int32
		online := newOnline(&onlineHits)
		defer online.Close()
		var remoteHits atomic.Int32
		certifier := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			remoteHits.Add(1)
			http.Redirect(w, r, "https://remote.invalid/certifier", http.StatusFound)
		}))
		defer certifier.Close()

		oc, _ := offlineTestClient(t)
		before := oc.client.Transport
		doc, err := verifyTrustedCert(oc, mustDoc(t, certifierCertForm(online.URL+"/certs", certifier.URL+"/certifier")), "duo.example.com", online.URL+"/submit", url.Values{})
		require.NoError(t, err)
		sid, _ := doc.Find(`input[name="sid"]`).Attr("value")
		assert.Equal(t, "after-cert", sid)
		assert.Equal(t, int32(1), remoteHits.Load())
		assert.Equal(t, int32(1), onlineHits.Load(), "the redirect is rejected, so the online validator is used")
		assert.Same(t, before, oc.client.Transport, "the shared client transport must not be replaced")
	})

	t.Run("non-2xx certifier response is reported as an error by the shared status check", func(t *testing.T) {
		certifier := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer certifier.Close()

		oc, _ := offlineTestClient(t)
		oc.client.CheckResponseStatus = func(_ *http.Request, res *http.Response) error {
			if res.StatusCode != http.StatusOK {
				return fmt.Errorf("unexpected status %d", res.StatusCode)
			}
			return nil
		}
		req, err := http.NewRequest(http.MethodGet, certifier.URL+"/certifier", nil)
		require.NoError(t, err)
		res, err := newCertifierClient(oc.client, certifier.URL).Do(req)
		if res != nil {
			_ = res.Body.Close()
		}
		require.Error(t, err)
	})
}

func TestNewCertifierClient_Redirects(t *testing.T) {
	var final atomic.Int32
	target := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		final.Add(1)
		_, _ = fmt.Fprint(w, "ok")
	}))
	defer target.Close()
	certifier := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/to-loopback":
			http.Redirect(w, r, target.URL+"/final", http.StatusFound)
		default:
			http.Redirect(w, r, "https://remote.invalid/final", http.StatusFound)
		}
	}))
	defer certifier.Close()

	oc, _ := offlineTestClient(t)

	t.Run("redirect to another loopback host is followed", func(t *testing.T) {
		req, err := http.NewRequest(http.MethodGet, certifier.URL+"/to-loopback", nil)
		require.NoError(t, err)
		res, err := newCertifierClient(oc.client, certifier.URL).Do(req)
		require.NoError(t, err)
		defer res.Body.Close()
		assert.Equal(t, http.StatusOK, res.StatusCode)
		assert.Equal(t, int32(1), final.Load())
	})

	t.Run("redirect to a non-loopback host is rejected", func(t *testing.T) {
		req, err := http.NewRequest(http.MethodGet, certifier.URL+"/to-remote", nil)
		require.NoError(t, err)
		res, err := newCertifierClient(oc.client, certifier.URL).Do(req)
		if res != nil {
			defer res.Body.Close()
		}
		require.Error(t, err)
		assert.ErrorIs(t, err, errCertifierRedirect)
	})
}
