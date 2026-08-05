// Package cacerts locates a system-trusted CA bundle for use with
// subprocesses whose own certificate stores can't be trusted to validate
// modern TLS chains.
//
// The canonical case is PyInstaller-bundled Python tools (checkov is the
// one we hit first; infracost, terraform-cost-estimation, snyk, sentry-cli,
// aws-cli v1, etc. all share the same bundling shape): the frozen `certifi`
// PEM inside the binary doesn't pick up new intermediate certs or updated
// chains until the maintainer rebuilds. Setting SSL_CERT_FILE and/or
// REQUESTS_CA_BUNDLE in the subprocess environment lets those tools fall
// back to the host's CA store, which is kept up to date by the OS or
// package manager.
//
// Find() returns the first existing well-known CA bundle path for the
// host platform, or empty string when none is found. On Windows there is
// no canonical file-based bundle (the system uses SCHANNEL), so callers
// should expect "" and let the subprocess fall back to its own logic.
package cacerts

import (
	"os"
	"runtime"
	"sync"

	"github.com/cloudposse/atmos/pkg/perf"
)

// EnvVars are the canonical environment variable names downstream tools
// look at to override their built-in CA store. Setting both covers the
// common Python landscape: `requests` uses REQUESTS_CA_BUNDLE first;
// the standard library `ssl` module uses SSL_CERT_FILE.
const (
	EnvSSLCertFile      = "SSL_CERT_FILE"
	EnvRequestsCABundle = "REQUESTS_CA_BUNDLE"
)

// candidates lists CA bundle file paths probed by Find(), in priority
// order. Every supported Unix-like platform — Darwin, Linux distros,
// and BSDs — keeps a PEM file in a well-known location, so we walk
// the list and return the first hit.
//
// The list intentionally includes both /etc paths and Homebrew paths so
// macOS users on stock or Homebrew-managed OpenSSL both get a working
// answer. Order matters only when multiple files exist — most platforms
// have exactly one.
var candidates = []string{
	// macOS (stock LibreSSL). Always present on a default install.
	"/etc/ssl/cert.pem",
	// Debian, Ubuntu, Alpine — ca-certificates package extracts a
	// concatenated PEM here.
	"/etc/ssl/certs/ca-certificates.crt",
	// RHEL, Fedora, CentOS — ca-certificates package.
	"/etc/pki/tls/certs/ca-bundle.crt",
	// RHEL alt path; some images expose only this one.
	"/etc/pki/ca-trust/extracted/pem/tls-ca-bundle.pem",
	// FreeBSD ports.
	"/usr/local/etc/ssl/cert.pem",
	"/usr/local/etc/openssl/cert.pem",
	// Homebrew on Apple Silicon and Intel respectively. Users who
	// installed `ca-certificates` via brew get a fresh bundle here.
	"/opt/homebrew/etc/ca-certificates/cert.pem",
	"/usr/local/etc/ca-certificates/cert.pem",
}

// findOnce is a pointer so tests can swap in a fresh sync.Once without
// copying (sync.Once contains noCopy, which govet/copylocks rejects).
var (
	findOnce   = new(sync.Once)
	cachedPath string
)

// Find returns the path to a system-trusted CA bundle for the host
// platform, or "" when none is found (notably on Windows). The first
// existing path from `candidates` is returned. The lookup is performed
// once per process and cached — CA bundle paths don't change during
// the lifetime of a CLI invocation.
func Find() string {
	defer perf.Track(nil, "cacerts.Find")()

	findOnce.Do(func() {
		cachedPath = locate(runtime.GOOS)
	})
	return cachedPath
}

// locate is the testable inner half of Find. Separating it lets unit
// tests stub the OS without going through sync.Once, and avoids running
// the file probe at every test invocation.
func locate(goos string) string {
	// Windows uses Schannel; there's no canonical file-based bundle path.
	// Callers should treat "" as "let the subprocess use its own store".
	if goos == "windows" {
		return ""
	}
	for _, candidate := range candidates {
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate
		}
	}
	return ""
}

// Env returns a map suitable for adding to a subprocess environment.
// Empty when no bundle is found; in that case callers should add
// nothing (leaving the subprocess to use whatever default it has).
//
// Both EnvSSLCertFile and EnvRequestsCABundle are populated because
// Python tools split on which env var they honor — `ssl` uses the
// former, `requests` uses the latter. Setting both is cheap and
// safe; setting only one risks missing the tool that reads the other.
func Env() map[string]string {
	defer perf.Track(nil, "cacerts.Env")()

	path := Find()
	if path == "" {
		return nil
	}
	return map[string]string{
		EnvSSLCertFile:      path,
		EnvRequestsCABundle: path,
	}
}
