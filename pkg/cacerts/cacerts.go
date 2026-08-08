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
//
// BuildBundle() combines the host's system CA bundle (via the same lookup as Find()) with a
// caller-supplied extra PEM (e.g. a private root CA a subprocess would otherwise never trust,
// such as pkg/cacerts/rds's embedded Amazon RDS CA bundle) and materializes the result at a
// stable, content-hash-keyed path under the Atmos XDG cache directory.
package cacerts

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sync"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/filesystem"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/xdg"
)

// bundleFilePerm is the permission used for materialized combined CA bundle files. World/group
// readable, matching the other CA bundle writers in this codebase (azure_trust.go, terraform/cache/tls.go).
const bundleFilePerm = 0o644

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

// BuildBundle returns the path to a CA trust bundle combining the host's system CA store (via the
// same lookup as Find(), when one exists) with the caller-supplied extra PEM appended. This is how
// callers add trust for a CA the system store doesn't (and will never) carry — e.g. Amazon RDS's
// private root CAs (pkg/cacerts/rds) — without losing trust for everything else the system store
// already covers.
//
// The combined bundle is content-hash-keyed and written once under the Atmos XDG cache directory,
// so repeated calls across invocations resolve to the same stable on-disk path (unlike a fresh
// temp file per call, which would leave nothing for a later `--print-command`-printed path to
// point at, and would need explicit cleanup).
//
// Returns "" (with no error) when there is nothing to write — no system bundle was found AND extra
// is empty. Callers should treat "" the same way they treat Find() returning "": pass nothing to
// the subprocess and let it fall back to its own trust store.
func BuildBundle(extra []byte) (string, error) {
	defer perf.Track(nil, "cacerts.BuildBundle")()

	base := readBundle(Find())

	combined := make([]byte, 0, len(base)+len(extra)+1)
	combined = append(combined, base...)
	if len(base) > 0 && len(extra) > 0 {
		combined = append(combined, '\n')
	}
	combined = append(combined, extra...)

	if len(combined) == 0 {
		return "", nil
	}

	return writeBundle(combined)
}

// readBundle reads path, returning nil (not an error) when path is empty or unreadable — a system
// bundle we can't use is treated the same as no system bundle at all.
func readBundle(path string) []byte {
	if path == "" {
		return nil
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	return b
}

// writeBundle materializes combined under a content-hash-keyed filename in the Atmos XDG cache
// directory, reusing an already-written file with the same hash rather than rewriting it.
func writeBundle(combined []byte) (string, error) {
	sum := sha256.Sum256(combined)
	fileName := fmt.Sprintf("bundle-%x.pem", sum[:8])

	cacheDir, err := xdg.GetXDGCacheDir("cacerts", xdg.DefaultCacheDirPerm)
	if err != nil {
		return "", fmt.Errorf("%w: %w", errUtils.ErrCABundleBuild, err)
	}
	bundlePath := filepath.Join(cacheDir, fileName)

	if info, err := os.Stat(bundlePath); err == nil && !info.IsDir() {
		return bundlePath, nil
	}

	fs := filesystem.NewOSFileSystem()
	if err := fs.WriteFileAtomic(bundlePath, combined, bundleFilePerm); err != nil {
		return "", fmt.Errorf("%w: %w", errUtils.ErrCABundleBuild, err)
	}
	return bundlePath, nil
}
