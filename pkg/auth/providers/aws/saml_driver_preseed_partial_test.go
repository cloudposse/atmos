package aws

import (
	"crypto/sha512"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
)

// coreOnlyServer serves a valid playwright-core tarball but fails every Node.js request, and
// counts the requests for the tarball metadata.
func coreOnlyServer(t *testing.T) *atomic.Int32 {
	t.Helper()
	tgz := makeTgz(t, map[string]string{"package/cli.js": "console.log('cli')", "package/lib/x.js": "x"})
	sum := sha512.Sum512(tgz)
	integrity := "sha512-" + base64.StdEncoding.EncodeToString(sum[:])

	var coreRequests atomic.Int32
	mux := http.NewServeMux()
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	mux.HandleFunc("/playwright-core/", func(w http.ResponseWriter, _ *http.Request) {
		coreRequests.Add(1)
		fmt.Fprintf(w, `{"dist":{"tarball":%q,"integrity":%q}}`, server.URL+"/tarball.tgz", integrity)
	})
	mux.HandleFunc("/tarball.tgz", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(tgz)
	})
	// Everything else, including the Node.js archive and checksums, is a 404.
	overridePreseedHosts(t, server.URL, server.URL)
	return &coreRequests
}

// Regression: when the Node.js download failed after package/cli.js was written, the half-seeded
// driver stayed behind. Playwright treats an existing cli.js as a complete driver and runs the missing
// Node.js binary for its version check, so its own downloader never ran.
func TestEnsurePlaywrightDriver_NodeFailureRollsBackSeededPackage(t *testing.T) {
	skipIfNodejsOrgArchiveUnsupported(t)
	driverDir := t.TempDir()
	t.Setenv("PLAYWRIGHT_DRIVER_PATH", driverDir)
	t.Setenv("PLAYWRIGHT_NODEJS_PATH", "")
	t.Setenv("PLAYWRIGHT_CLI_PATH", "")
	coreRequests := coreOnlyServer(t)

	err := ensurePlaywrightDriver()
	require.ErrorIs(t, err, errUtils.ErrPlaywrightDriverSeed)
	assert.NotErrorIs(t, err, errUtils.ErrPlaywrightDriverIntegrity)
	assert.EqualValues(t, 1, coreRequests.Load(), "the package was fetched, then rolled back")
	assert.NoFileExists(t, filepath.Join(driverDir, "package", "cli.js"))
	assert.NoDirExists(t, filepath.Join(driverDir, "package"))
}

// Negative path: an installed driver is never rolled back by a failed Node.js download.
func TestEnsurePlaywrightDriver_NodeFailureKeepsInstalledPackage(t *testing.T) {
	skipIfNodejsOrgArchiveUnsupported(t)
	driverDir := t.TempDir()
	t.Setenv("PLAYWRIGHT_DRIVER_PATH", driverDir)
	t.Setenv("PLAYWRIGHT_NODEJS_PATH", "")
	t.Setenv("PLAYWRIGHT_CLI_PATH", "")
	coreRequests := coreOnlyServer(t)
	cliPath := filepath.Join(driverDir, "package", "cli.js")
	require.NoError(t, os.MkdirAll(filepath.Dir(cliPath), 0o755))
	require.NoError(t, os.WriteFile(cliPath, []byte("installed-cli"), 0o644))

	require.ErrorIs(t, ensurePlaywrightDriver(), errUtils.ErrPlaywrightDriverSeed)
	assert.Zero(t, coreRequests.Load(), "an installed package is not fetched again")
	content, err := os.ReadFile(cliPath)
	require.NoError(t, err)
	assert.Equal(t, "installed-cli", string(content))
}

// Regression: with PLAYWRIGHT_CLI_PATH the bundled CLI is unused, so an unverifiable or unreachable
// bundled tarball must not block login. The Node.js runtime is still seeded.
func TestEnsurePlaywrightDriver_ExternalCLISkipsBundledCLI(t *testing.T) {
	skipIfNodejsOrgArchiveUnsupported(t)
	driverDir := t.TempDir()
	t.Setenv("PLAYWRIGHT_DRIVER_PATH", driverDir)
	t.Setenv("PLAYWRIGHT_NODEJS_PATH", "")
	t.Setenv("PLAYWRIGHT_CLI_PATH", filepath.Join(t.TempDir(), "cli.js"))

	nodeName, nodeBody, shasums := nodeArchive(t)
	var coreRequests atomic.Int32
	mux := http.NewServeMux()
	server := httptest.NewServer(mux)
	defer server.Close()
	mux.HandleFunc("/playwright-core/", func(w http.ResponseWriter, _ *http.Request) {
		coreRequests.Add(1)
		fmt.Fprintf(w, `{"dist":{"tarball":%q,"integrity":"sha512-tampered"}}`, server.URL+"/tarball.tgz")
	})
	mux.HandleFunc("/v"+playwrightNodeVersion+"/SHASUMS256.txt", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(shasums))
	})
	mux.HandleFunc("/v"+playwrightNodeVersion+"/"+nodeName, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(nodeBody)
	})
	overridePreseedHosts(t, server.URL, server.URL)

	require.NoError(t, ensurePlaywrightDriver())
	assert.Zero(t, coreRequests.Load(), "the unused bundled CLI must not be fetched")
	assert.NoFileExists(t, filepath.Join(driverDir, "package", "cli.js"))
	node := "node"
	if runtime.GOOS == windowsOS {
		node = "node.exe"
	}
	assert.FileExists(t, filepath.Join(driverDir, node))
}
