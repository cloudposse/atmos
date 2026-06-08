package cache

import (
	"crypto/tls"
	"crypto/x509"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newProxyTLSServer generates a fresh self-signed proxy certificate (the same one the
// cache proxy serves) and starts a loopback HTTPS test server presenting it. It returns
// the server, the on-disk cert path, and the tls dir so callers can build the trust
// bundle next to it. The server binds 127.0.0.1, matching the cert's loopback SAN.
func newProxyTLSServer(t *testing.T) (srv *httptest.Server, certPath, dir string) {
	t.Helper()

	dir = filepath.Join(t.TempDir(), tlsDirName)
	certPath = filepath.Join(dir, tlsCertFile)
	keyPath := filepath.Join(dir, tlsKeyFile)
	cert, err := generateAndWriteProxyCert(dir, certPath, keyPath)
	require.NoError(t, err)

	srv = httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	srv.TLS = &tls.Config{Certificates: []tls.Certificate{cert}, MinVersion: tls.VersionTLS12}
	srv.StartTLS()
	t.Cleanup(srv.Close)
	return srv, certPath, dir
}

// TestTrustE2E_BundleVerifiesSelfSignedCert is the cross-platform handshake contract:
// the bundle Atmos builds (system roots + proxy cert) verifies a real TLS connection to
// the proxy's self-signed cert, while a client without it is genuinely rejected. This
// runs identically on linux/darwin/windows.
func TestTrustE2E_BundleVerifiesSelfSignedCert(t *testing.T) {
	// Deterministic system base so buildTrustBundle has roots to extend.
	sysRoots := filepath.Join(t.TempDir(), "roots.pem")
	require.NoError(t, os.WriteFile(sysRoots, []byte("SYSTEM_ROOTS_MARKER\n"), tlsCertPerm))
	t.Setenv("SSL_CERT_FILE", sysRoots)

	srv, certPath, dir := newProxyTLSServer(t)

	env, err := buildTrustBundle(certPath)
	require.NoError(t, err)
	bundlePath := filepath.Join(dir, tlsBundleFile)
	require.Equal(t, []string{"SSL_CERT_FILE=" + bundlePath}, env)

	bundlePEM, err := os.ReadFile(bundlePath)
	require.NoError(t, err)

	t.Run("client trusting the bundle connects", func(t *testing.T) {
		pool := x509.NewCertPool()
		require.True(t, pool.AppendCertsFromPEM(bundlePEM), "bundle must contain the proxy cert")
		client := &http.Client{Transport: &http.Transport{
			TLSClientConfig: &tls.Config{RootCAs: pool, MinVersion: tls.VersionTLS12},
		}}
		resp, err := client.Get(srv.URL) //nolint:noctx // loopback test request.
		require.NoError(t, err)
		t.Cleanup(func() { _ = resp.Body.Close() })
		assert.Equal(t, http.StatusOK, resp.StatusCode)
	})

	t.Run("client without the bundle is rejected", func(t *testing.T) {
		// An empty root pool cannot verify the self-signed cert, proving trust must come
		// from somewhere (the bundle on Linux, the OS store on macOS/Windows).
		client := &http.Client{Transport: &http.Transport{
			TLSClientConfig: &tls.Config{RootCAs: x509.NewCertPool(), MinVersion: tls.VersionTLS12},
		}}
		resp, err := client.Get(srv.URL) //nolint:noctx // loopback test request.
		if err == nil {
			_ = resp.Body.Close()
		}
		require.Error(t, err)
		assert.True(t, isCertTrustError(err), "expected a certificate trust error, got %v", err)
	})
}

// TestTrustE2E_PlatformDivergence pins the reason trust/untrust exists: with
// SSL_CERT_FILE pointed at the bundle, Go's default client trusts the proxy on
// Linux/BSD (out of the box, no `cache trust`) but rejects it on macOS/Windows (where
// SSL_CERT_FILE is ignored and the cert must be installed in the OS trust store). It
// runs the probe in a re-exec'd subprocess because Go caches the system cert pool once
// per process, so an in-process SSL_CERT_FILE change would not take effect.
func TestTrustE2E_PlatformDivergence(t *testing.T) {
	sysRoots := filepath.Join(t.TempDir(), "roots.pem")
	require.NoError(t, os.WriteFile(sysRoots, []byte("SYSTEM_ROOTS_MARKER\n"), tlsCertPerm))
	t.Setenv("SSL_CERT_FILE", sysRoots)

	srv, certPath, dir := newProxyTLSServer(t)

	_, err := buildTrustBundle(certPath)
	require.NoError(t, err)
	bundlePath := filepath.Join(dir, tlsBundleFile)

	exe, err := os.Executable()
	require.NoError(t, err)

	cmd := exec.Command(exe)
	cmd.Env = append(
		childEnvWithout(os.Environ(), "SSL_CERT_FILE", "_ATMOS_TEST_HTTPS_PROBE_URL"),
		"SSL_CERT_FILE="+bundlePath,
		"_ATMOS_TEST_HTTPS_PROBE_URL="+srv.URL,
	)
	probeErr := cmd.Run()

	if required, _ := TrustInstructions(); required {
		// macOS/Windows: Go uses the platform verifier and ignores SSL_CERT_FILE, so the
		// self-signed proxy cert stays untrusted until `atmos terraform cache trust` runs.
		require.Error(t, probeErr, "SSL_CERT_FILE alone must not establish trust on this platform")
	} else {
		// Linux/BSD: Go honors SSL_CERT_FILE, so the bundle alone establishes trust.
		require.NoError(t, probeErr, "SSL_CERT_FILE bundle must establish trust on this platform")
	}
}

// childEnvWithout returns env with every "KEY=..." entry for the given keys removed, so
// the caller can append authoritative values without duplicate-key ambiguity.
func childEnvWithout(env []string, keys ...string) []string {
	out := make([]string, 0, len(env))
	for _, kv := range env {
		drop := false
		for _, k := range keys {
			if strings.HasPrefix(kv, k+"=") {
				drop = true
				break
			}
		}
		if !drop {
			out = append(out, kv)
		}
	}
	return out
}
