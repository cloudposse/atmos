package cache

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"time"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

const (
	tlsDirName  = "tls"
	tlsCertFile = "proxy.pem"
	tlsKeyFile  = "proxy-key.pem"
	tlsKeyPerm  = 0o600
	tlsCertPerm = 0o644
	tlsDirPerm  = 0o700
	// Certificate lifetime. Apple's platform verifier rejects TLS server certificates
	// valid for more than 398 days ("certificate is not standards compliant"), so stay
	// under that.
	certValidity = 390 * 24 * time.Hour
	// Regenerate the certificate when less than this remains before expiry.
	certRenewBefore = 30 * 24 * time.Hour
	// Combined CA bundle filename (system roots + proxy cert).
	tlsBundleFile = "bundle.pem"
	// Bit length of the random certificate serial number.
	serialBits = 128
	// First octet of the IPv4 loopback address used in the certificate SAN.
	loopbackV4 = 127
)

// systemCertFileCandidates are the well-known consolidated CA bundle paths Go reads
// on Linux/BSD. The first that exists is the base for our trust bundle.
var systemCertFileCandidates = []string{
	"/etc/ssl/certs/ca-certificates.crt",                // Debian/Ubuntu/Alpine.
	"/etc/pki/tls/certs/ca-bundle.crt",                  // Fedora/RHEL.
	"/etc/ssl/ca-bundle.pem",                            // OpenSUSE.
	"/etc/pki/tls/cacert.pem",                           // OpenELEC.
	"/etc/pki/ca-trust/extracted/pem/tls-ca-bundle.pem", // CentOS/RHEL 7.
	"/etc/ssl/cert.pem",                                 // Alpine/macOS.
}

// buildTrustBundle writes a CA bundle (system roots + the proxy certificate) next to
// the cert and returns the env vars that make terraform/tofu trust the proxy via
// Go's SSL_CERT_FILE. This is honored on Linux/BSD. On macOS and Windows Go uses the
// platform verifier and ignores SSL_CERT_FILE, so trust there requires installing the
// certificate into the OS trust store (see `atmos terraform cache trust`); the bundle
// is still written (harmless) so the same code path works everywhere.
//
// SSL_CERT_FILE replaces the default system file, so the bundle must include the
// system roots — otherwise the subprocess loses trust for state backends and other
// TLS. When no system bundle is found, trust env is skipped rather than risk dropping
// the system roots.
func buildTrustBundle(certPath string) ([]string, error) {
	defer perf.Track(nil, "tfcache.buildTrustBundle")()

	certPEM, err := os.ReadFile(certPath)
	if err != nil {
		return nil, fmt.Errorf("%w: reading cache certificate: %w", errUtils.ErrInvalidConfig, err)
	}

	base, ok := readSystemCertBundle()
	if !ok {
		return nil, nil
	}

	bundle := make([]byte, 0, len(base)+len(certPEM)+1)
	bundle = append(bundle, base...)
	bundle = append(bundle, '\n')
	bundle = append(bundle, certPEM...)

	bundlePath := filepath.Join(filepath.Dir(certPath), tlsBundleFile)
	if err := os.WriteFile(bundlePath, bundle, tlsCertPerm); err != nil {
		return nil, fmt.Errorf("%w: writing cache trust bundle: %w", errUtils.ErrInvalidConfig, err)
	}
	return []string{"SSL_CERT_FILE=" + bundlePath}, nil
}

// readSystemCertBundle returns the host's CA bundle PEM: the user's SSL_CERT_FILE if
// set, else the first well-known consolidated bundle that exists.
func readSystemCertBundle() ([]byte, bool) {
	// SSL_CERT_FILE is a standard OpenSSL/Go system env var (not an Atmos config var);
	// read it directly to honor a user-managed bundle.
	if f := os.Getenv("SSL_CERT_FILE"); f != "" { //nolint:forbidigo // standard system env var, not Atmos config.
		if b, err := os.ReadFile(f); err == nil { //nolint:gosec // path is the user's own SSL_CERT_FILE.
			return b, true
		}
	}
	for _, candidate := range systemCertFileCandidates {
		if b, err := os.ReadFile(candidate); err == nil {
			return b, true
		}
	}
	return nil, false
}

// ProxyCertPath returns the on-disk path of the cache proxy certificate for the
// given configuration, whether or not it has been generated yet. Used by the
// `cache trust` command to locate the certificate.
func ProxyCertPath(atmosConfig *schema.AtmosConfiguration) (string, error) {
	defer perf.Track(atmosConfig, "tfcache.ProxyCertPath")()

	root, err := ResolveRoot(atmosConfig)
	if err != nil {
		return "", err
	}
	return filepath.Join(root, tlsDirName, tlsCertFile), nil
}

// proxyCert holds the loaded or generated certificate material for the proxy.
type proxyCert struct {
	// Certificate is the TLS keypair the proxy serves with.
	Certificate tls.Certificate
	// CertPEMPath is the on-disk path of the certificate PEM, for trust wiring.
	CertPEMPath string
	// Generated is true when this run created the certificate (vs. reusing a cached one).
	Generated bool
}

// ensureProxyCert loads the cached self-signed proxy certificate from <root>/tls,
// generating and caching a new one when missing or near expiry. The certificate is a
// loopback root (SANs 127.0.0.1, ::1, localhost) so terraform/tofu can trust it for
// the https network mirror. Caching avoids regenerating on every invocation.
func ensureProxyCert(root string) (*proxyCert, error) {
	defer perf.Track(nil, "tfcache.ensureProxyCert")()

	dir := filepath.Join(root, tlsDirName)
	certPath := filepath.Join(dir, tlsCertFile)
	keyPath := filepath.Join(dir, tlsKeyFile)

	if cert, ok := loadProxyCert(certPath, keyPath); ok {
		return &proxyCert{Certificate: cert, CertPEMPath: certPath}, nil
	}

	cert, err := generateAndWriteProxyCert(dir, certPath, keyPath)
	if err != nil {
		return nil, err
	}
	return &proxyCert{Certificate: cert, CertPEMPath: certPath, Generated: true}, nil
}

// loadProxyCert loads a cached keypair, returning ok=false when missing, unreadable,
// or within certRenewBefore of expiry.
func loadProxyCert(certPath, keyPath string) (tls.Certificate, bool) {
	certPEM, err := os.ReadFile(certPath)
	if err != nil {
		return tls.Certificate{}, false
	}
	keyPEM, err := os.ReadFile(keyPath)
	if err != nil {
		return tls.Certificate{}, false
	}
	cert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		return tls.Certificate{}, false
	}
	leaf, err := x509.ParseCertificate(cert.Certificate[0])
	if err != nil {
		return tls.Certificate{}, false
	}
	if time.Now().After(leaf.NotAfter.Add(-certRenewBefore)) {
		return tls.Certificate{}, false
	}
	cert.Leaf = leaf
	return cert, true
}

// generateAndWriteProxyCert creates a self-signed loopback certificate and persists it.
func generateAndWriteProxyCert(dir, certPath, keyPath string) (tls.Certificate, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("%w: generating cache TLS key: %w", errUtils.ErrInvalidConfig, err)
	}

	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), serialBits))
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("%w: generating cache TLS serial: %w", errUtils.ErrInvalidConfig, err)
	}

	now := time.Now()
	template := x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{CommonName: "Atmos Terraform Registry Cache", Organization: []string{"Atmos"}},
		NotBefore:             now.Add(-time.Hour),
		NotAfter:              now.Add(certValidity),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		IsCA:                  true,
		DNSNames:              []string{"localhost"},
		IPAddresses:           []net.IP{net.IPv4(loopbackV4, 0, 0, 1), net.IPv6loopback},
	}

	der, err := x509.CreateCertificate(rand.Reader, &template, &template, &key.PublicKey, key)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("%w: creating cache TLS certificate: %w", errUtils.ErrInvalidConfig, err)
	}

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("%w: marshaling cache TLS key: %w", errUtils.ErrInvalidConfig, err)
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})

	if err := os.MkdirAll(dir, tlsDirPerm); err != nil {
		return tls.Certificate{}, fmt.Errorf("%w: creating cache TLS dir: %w", errUtils.ErrInvalidConfig, err)
	}
	if err := os.WriteFile(certPath, certPEM, tlsCertPerm); err != nil {
		return tls.Certificate{}, fmt.Errorf("%w: writing cache TLS certificate: %w", errUtils.ErrInvalidConfig, err)
	}
	if err := os.WriteFile(keyPath, keyPEM, tlsKeyPerm); err != nil {
		return tls.Certificate{}, fmt.Errorf("%w: writing cache TLS key: %w", errUtils.ErrInvalidConfig, err)
	}

	cert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("%w: loading cache TLS keypair: %w", errUtils.ErrInvalidConfig, err)
	}
	return cert, nil
}
