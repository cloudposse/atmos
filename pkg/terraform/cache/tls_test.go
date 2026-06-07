package cache

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

func TestGenerateAndWriteProxyCert(t *testing.T) {
	dir := filepath.Join(t.TempDir(), tlsDirName)
	certPath := filepath.Join(dir, tlsCertFile)
	keyPath := filepath.Join(dir, tlsKeyFile)

	cert, err := generateAndWriteProxyCert(dir, certPath, keyPath)
	require.NoError(t, err)
	require.NotEmpty(t, cert.Certificate)

	// Both PEM files exist on disk.
	certInfo, err := os.Stat(certPath)
	require.NoError(t, err)
	keyInfo, err := os.Stat(keyPath)
	require.NoError(t, err)

	if runtime.GOOS != "windows" {
		assert.Equal(t, os.FileMode(tlsCertPerm), certInfo.Mode().Perm())
		assert.Equal(t, os.FileMode(tlsKeyPerm), keyInfo.Mode().Perm())
	}

	// The certificate is a loopback root with the expected SANs and validity.
	leaf, err := x509.ParseCertificate(cert.Certificate[0])
	require.NoError(t, err)
	assert.True(t, leaf.IsCA)
	assert.Contains(t, leaf.DNSNames, "localhost")
	assert.True(t, containsIP(leaf.IPAddresses, net.IPv4(loopbackV4, 0, 0, 1)), "must include 127.0.0.1 SAN")
	assert.True(t, containsIP(leaf.IPAddresses, net.IPv6loopback), "must include ::1 SAN")
	assert.WithinDuration(t, time.Now().Add(certValidity), leaf.NotAfter, time.Hour)
}

func TestLoadProxyCert(t *testing.T) {
	dir := filepath.Join(t.TempDir(), tlsDirName)
	certPath := filepath.Join(dir, tlsCertFile)
	keyPath := filepath.Join(dir, tlsKeyFile)

	t.Run("missing files", func(t *testing.T) {
		_, ok := loadProxyCert(certPath, keyPath)
		assert.False(t, ok)
	})

	_, err := generateAndWriteProxyCert(dir, certPath, keyPath)
	require.NoError(t, err)

	t.Run("round-trips a freshly generated cert", func(t *testing.T) {
		cert, ok := loadProxyCert(certPath, keyPath)
		require.True(t, ok)
		assert.NotNil(t, cert.Leaf)
	})

	t.Run("corrupt PEM is rejected", func(t *testing.T) {
		bad := filepath.Join(t.TempDir(), "bad.pem")
		require.NoError(t, os.WriteFile(bad, []byte("not a pem"), tlsCertPerm))
		_, ok := loadProxyCert(bad, keyPath)
		assert.False(t, ok)
	})

	t.Run("near-expiry cert is rejected", func(t *testing.T) {
		expDir := t.TempDir()
		expCert := filepath.Join(expDir, tlsCertFile)
		expKey := filepath.Join(expDir, tlsKeyFile)
		// NotAfter is inside the renewal window, so the cert is treated as expired.
		writeCertWithExpiry(t, expCert, expKey, time.Now().Add(certRenewBefore/2))
		_, ok := loadProxyCert(expCert, expKey)
		assert.False(t, ok)
	})
}

func TestEnsureProxyCert_GeneratesThenReuses(t *testing.T) {
	root := t.TempDir()

	first, err := ensureProxyCert(root)
	require.NoError(t, err)
	assert.True(t, first.Generated, "first call generates a new cert")
	assert.Equal(t, filepath.Join(root, tlsDirName, tlsCertFile), first.CertPEMPath)
	_, statErr := os.Stat(first.CertPEMPath)
	require.NoError(t, statErr)

	second, err := ensureProxyCert(root)
	require.NoError(t, err)
	assert.False(t, second.Generated, "second call reuses the cached cert")
	assert.Equal(t, first.CertPEMPath, second.CertPEMPath)
}

func TestReadSystemCertBundle_HonorsSSLCertFile(t *testing.T) {
	bundle := filepath.Join(t.TempDir(), "roots.pem")
	want := []byte("SYSTEM ROOTS PLACEHOLDER\n")
	require.NoError(t, os.WriteFile(bundle, want, tlsCertPerm))
	t.Setenv("SSL_CERT_FILE", bundle)

	got, ok := readSystemCertBundle()
	require.True(t, ok)
	assert.Equal(t, want, got)
}

func TestReadSystemCertBundle_FallsBackToCandidates(t *testing.T) {
	// With SSL_CERT_FILE empty, the candidate-path search runs. On Linux/macOS a system
	// bundle exists, so a found bundle must be non-empty; on a hermetic box it returns false.
	t.Setenv("SSL_CERT_FILE", "")
	got, ok := readSystemCertBundle()
	if ok {
		assert.NotEmpty(t, got)
	}
}

func TestBuildTrustBundle(t *testing.T) {
	// Make the system base deterministic via SSL_CERT_FILE.
	sysRoots := filepath.Join(t.TempDir(), "roots.pem")
	require.NoError(t, os.WriteFile(sysRoots, []byte("SYSTEM_ROOTS_MARKER\n"), tlsCertPerm))
	t.Setenv("SSL_CERT_FILE", sysRoots)

	dir := filepath.Join(t.TempDir(), tlsDirName)
	certPath := filepath.Join(dir, tlsCertFile)
	keyPath := filepath.Join(dir, tlsKeyFile)
	_, err := generateAndWriteProxyCert(dir, certPath, keyPath)
	require.NoError(t, err)

	env, err := buildTrustBundle(certPath)
	require.NoError(t, err)
	bundlePath := filepath.Join(dir, tlsBundleFile)
	require.Equal(t, []string{"SSL_CERT_FILE=" + bundlePath}, env)

	contents, err := os.ReadFile(bundlePath)
	require.NoError(t, err)
	certPEM, err := os.ReadFile(certPath)
	require.NoError(t, err)
	assert.Contains(t, string(contents), "SYSTEM_ROOTS_MARKER", "bundle must keep the system roots")
	assert.Contains(t, string(contents), string(certPEM), "bundle must append the proxy cert")
}

func TestBuildTrustBundle_UnreadableCert(t *testing.T) {
	_, err := buildTrustBundle(filepath.Join(t.TempDir(), "does-not-exist.pem"))
	require.Error(t, err)
}

func TestProxyCertPath(t *testing.T) {
	dir := t.TempDir()
	cfg := &schema.AtmosConfiguration{}
	cfg.Components.Terraform.Cache = &schema.TerraformCacheConfig{Location: dir}

	path, err := ProxyCertPath(cfg)
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(dir, tlsDirName, tlsCertFile), path)
}

// containsIP reports whether ips contains target.
func containsIP(ips []net.IP, target net.IP) bool {
	for _, ip := range ips {
		if ip.Equal(target) {
			return true
		}
	}
	return false
}

// writeCertWithExpiry generates a self-signed loopback cert/key with the given
// NotAfter and writes them as PEM, mirroring generateAndWriteProxyCert so loadProxyCert
// accepts the format. Used to exercise the renewal-window rejection path.
func writeCertWithExpiry(t *testing.T, certPath, keyPath string, notAfter time.Time) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), serialBits))
	require.NoError(t, err)
	template := x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: certCommonName},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     notAfter,
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		IsCA:         true,
		DNSNames:     []string{"localhost"},
		IPAddresses:  []net.IP{net.IPv4(loopbackV4, 0, 0, 1), net.IPv6loopback},
	}
	der, err := x509.CreateCertificate(rand.Reader, &template, &template, &key.PublicKey, key)
	require.NoError(t, err)
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyDER, err := x509.MarshalECPrivateKey(key)
	require.NoError(t, err)
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})

	require.NoError(t, os.WriteFile(certPath, certPEM, tlsCertPerm))
	require.NoError(t, os.WriteFile(keyPath, keyPEM, tlsKeyPerm))
}
