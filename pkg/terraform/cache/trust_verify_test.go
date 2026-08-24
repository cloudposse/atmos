package cache

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
)

// forceTrustRequired overrides the platform gate so VerifyTrust runs its probe body
// on any OS, restoring the real implementation when the test finishes.
func forceTrustRequired(t *testing.T, required bool) {
	t.Helper()
	prev := trustInstructionsFn
	trustInstructionsFn = func() (bool, string) { return required, "" }
	t.Cleanup(func() { trustInstructionsFn = prev })
}

func TestVerifyTrust_NoProbeCases(t *testing.T) {
	t.Run("nil setup", func(t *testing.T) {
		var s *Setup
		require.NoError(t, s.VerifyTrust(context.Background()))
	})

	t.Run("empty proxy URL", func(t *testing.T) {
		s := &Setup{}
		require.NoError(t, s.VerifyTrust(context.Background()))
	})

	t.Run("platform does not require trust", func(t *testing.T) {
		forceTrustRequired(t, false)
		// Even with a proxyURL set, a platform that trusts via SSL_CERT_FILE is a no-op.
		s := &Setup{proxyURL: "https://127.0.0.1:1/"}
		require.NoError(t, s.VerifyTrust(context.Background()))
	})
}

func TestVerifyTrust_UntrustedCertReturnsActionableError(t *testing.T) {
	forceTrustRequired(t, true)

	// httptest's TLS server presents a cert signed by a test CA absent from the OS
	// trust store, so the default client's probe fails with a certificate error.
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	s := &Setup{proxyURL: srv.URL + "/"}
	err := s.VerifyTrust(context.Background())
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrCacheCertUntrusted)
}

func TestVerifyTrust_TrustedProbeSucceeds(t *testing.T) {
	forceTrustRequired(t, true)

	// A plain-HTTP endpoint completes without a TLS error, so the probe is a no-op.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	s := &Setup{proxyURL: srv.URL + "/"}
	require.NoError(t, s.VerifyTrust(context.Background()))
}

func TestVerifyTrust_NonCertErrorIsTolerated(t *testing.T) {
	forceTrustRequired(t, true)

	// Point at a closed port: client.Do fails with a connection error, not a
	// certificate error, so VerifyTrust defers to terraform rather than reporting trust.
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	url := srv.URL
	srv.Close()

	s := &Setup{proxyURL: url + "/"}
	require.NoError(t, s.VerifyTrust(context.Background()))
}

func TestVerifyTrust_InvalidProxyURLIsTolerated(t *testing.T) {
	forceTrustRequired(t, true)

	// A malformed URL makes http.NewRequestWithContext fail; VerifyTrust returns nil.
	s := &Setup{proxyURL: "http://invalid url/"}
	require.NoError(t, s.VerifyTrust(context.Background()))
}

func TestIsCertTrustError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{name: "unknown authority", err: x509.UnknownAuthorityError{}, want: true},
		{name: "hostname mismatch", err: x509.HostnameError{Host: "x"}, want: true},
		{name: "certificate invalid", err: x509.CertificateInvalidError{}, want: true},
		{name: "tls verification error", err: &tls.CertificateVerificationError{}, want: true},
		{name: "wrapped cert error", err: errors.Join(errors.New("ctx"), x509.UnknownAuthorityError{}), want: true},
		{name: "plain error", err: errors.New("connection refused"), want: false},
		{name: "nil error", err: nil, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, isCertTrustError(tt.err))
		})
	}
}
