package aws

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
)

// makeTgz builds a gzipped tarball with the given entries (name -> content).
func makeTgz(t *testing.T, entries map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	gzWriter := gzip.NewWriter(&buf)
	tarWriter := tar.NewWriter(gzWriter)
	for name, content := range entries {
		require.NoError(t, tarWriter.WriteHeader(&tar.Header{
			Name: name, Mode: 0o644, Size: int64(len(content)), Typeflag: tar.TypeReg,
		}))
		_, err := tarWriter.Write([]byte(content))
		require.NoError(t, err)
	}
	require.NoError(t, tarWriter.Close())
	require.NoError(t, gzWriter.Close())
	return buf.Bytes()
}

// makeZip builds a zip archive with the given entries (name -> content).
func makeZip(t *testing.T, entries map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zipWriter := zip.NewWriter(&buf)
	for name, content := range entries {
		w, err := zipWriter.Create(name)
		require.NoError(t, err)
		_, err = w.Write([]byte(content))
		require.NoError(t, err)
	}
	require.NoError(t, zipWriter.Close())
	return buf.Bytes()
}

// overridePreseedHosts points the pre-seeder's download hosts at test servers
// and restores them on cleanup.
func overridePreseedHosts(t *testing.T, npmBase, nodeBase string) {
	t.Helper()
	origNpm, origNode := npmRegistryBase, nodejsDistBase
	npmRegistryBase, nodejsDistBase = npmBase, nodeBase
	t.Cleanup(func() {
		npmRegistryBase, nodejsDistBase = origNpm, origNode
	})
}

// nodeArchive builds a platform-appropriate fake Node.js archive plus its
// SHASUMS256.txt body for the current test platform.
func nodeArchive(t *testing.T) (name string, body []byte, shasums string) {
	t.Helper()
	suffix, err := nodePlatformSuffix()
	require.NoError(t, err)
	archiveDir := fmt.Sprintf("node-v%s-%s", playwrightNodeVersion, suffix)
	if runtime.GOOS == "windows" {
		name = archiveDir + ".zip"
		body = makeZip(t, map[string]string{archiveDir + "/node.exe": "fake node binary"})
	} else {
		name = archiveDir + ".tar.gz"
		body = makeTgz(t, map[string]string{archiveDir + "/bin/node": "fake node binary"})
	}
	sum := sha256.Sum256(body)
	shasums = hex.EncodeToString(sum[:]) + "  " + name + "\n"
	return name, body, shasums
}

func TestEnsurePlaywrightDriver_AlreadySeededIsOffline(t *testing.T) {
	driverDir := t.TempDir()
	t.Setenv("PLAYWRIGHT_DRIVER_PATH", driverDir)
	// Unreachable hosts prove no network request is made when already seeded.
	overridePreseedHosts(t, "http://127.0.0.1:1", "http://127.0.0.1:1")

	require.NoError(t, os.MkdirAll(filepath.Join(driverDir, "package"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(driverDir, "package", "cli.js"), []byte("cli"), 0o644))
	nodeName := "node"
	if runtime.GOOS == "windows" {
		nodeName = "node.exe"
	}
	require.NoError(t, os.WriteFile(filepath.Join(driverDir, nodeName), []byte("node"), 0o755))

	assert.NoError(t, ensurePlaywrightDriver())
}

func TestEnsurePlaywrightDriver_SeedsFromRegistries(t *testing.T) {
	driverDir := t.TempDir()
	t.Setenv("PLAYWRIGHT_DRIVER_PATH", driverDir)
	t.Setenv("PLAYWRIGHT_NODEJS_PATH", "")

	tgz := makeTgz(t, map[string]string{
		"package/cli.js":     "console.log('cli')",
		"package/lib/x.js":   "x",
		"unrelated/skip.txt": "skipped",
	})
	tgzSum := sha512.Sum512(tgz)
	integrity := "sha512-" + base64.StdEncoding.EncodeToString(tgzSum[:])

	nodeArchiveName, nodeBody, shasums := nodeArchive(t)

	mux := http.NewServeMux()
	server := httptest.NewServer(mux)
	defer server.Close()

	mux.HandleFunc("/playwright-core/", func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprintf(w, `{"dist":{"tarball":%q,"integrity":%q}}`, server.URL+"/tarball.tgz", integrity)
	})
	mux.HandleFunc("/tarball.tgz", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(tgz)
	})
	mux.HandleFunc("/v"+playwrightNodeVersion+"/SHASUMS256.txt", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(shasums))
	})
	mux.HandleFunc("/v"+playwrightNodeVersion+"/"+nodeArchiveName, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(nodeBody)
	})

	overridePreseedHosts(t, server.URL, server.URL)

	require.NoError(t, ensurePlaywrightDriver())

	// package/ tree extracted; unrelated entries skipped.
	cli, err := os.ReadFile(filepath.Join(driverDir, "package", "cli.js"))
	require.NoError(t, err)
	assert.Equal(t, "console.log('cli')", string(cli))
	assert.FileExists(t, filepath.Join(driverDir, "package", "lib", "x.js"))
	assert.NoFileExists(t, filepath.Join(driverDir, "unrelated", "skip.txt"))

	// Node binary placed at the driver root.
	nodeName := "node"
	if runtime.GOOS == "windows" {
		nodeName = "node.exe"
	}
	nodeContent, err := os.ReadFile(filepath.Join(driverDir, nodeName))
	require.NoError(t, err)
	assert.Equal(t, "fake node binary", string(nodeContent))
}

func TestEnsurePlaywrightDriver_IntegrityMismatchFails(t *testing.T) {
	driverDir := t.TempDir()
	t.Setenv("PLAYWRIGHT_DRIVER_PATH", driverDir)
	t.Setenv("PLAYWRIGHT_NODEJS_PATH", "/usr/bin/node") // skip the node phase

	tgz := makeTgz(t, map[string]string{"package/cli.js": "cli"})

	mux := http.NewServeMux()
	server := httptest.NewServer(mux)
	defer server.Close()
	mux.HandleFunc("/playwright-core/", func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprintf(w, `{"dist":{"tarball":%q,"integrity":"sha512-tampered"}}`, server.URL+"/tarball.tgz")
	})
	mux.HandleFunc("/tarball.tgz", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(tgz)
	})
	overridePreseedHosts(t, server.URL, server.URL)

	err := ensurePlaywrightDriver()
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrPlaywrightDriverSeed)
	assert.NoFileExists(t, filepath.Join(driverDir, "package", "cli.js"))
}

func TestEnsurePlaywrightDriver_NodeChecksumMismatchFails(t *testing.T) {
	driverDir := t.TempDir()
	t.Setenv("PLAYWRIGHT_DRIVER_PATH", driverDir)
	t.Setenv("PLAYWRIGHT_NODEJS_PATH", "")

	// Driver package already seeded; only the node phase runs.
	require.NoError(t, os.MkdirAll(filepath.Join(driverDir, "package"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(driverDir, "package", "cli.js"), []byte("cli"), 0o644))

	nodeArchiveName, nodeBody, _ := nodeArchive(t)

	mux := http.NewServeMux()
	server := httptest.NewServer(mux)
	defer server.Close()
	mux.HandleFunc("/v"+playwrightNodeVersion+"/SHASUMS256.txt", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("deadbeef  " + nodeArchiveName + "\n"))
	})
	mux.HandleFunc("/v"+playwrightNodeVersion+"/"+nodeArchiveName, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(nodeBody)
	})
	overridePreseedHosts(t, server.URL, server.URL)

	err := ensurePlaywrightDriver()
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrPlaywrightDriverSeed)
}

func TestVerifyNpmIntegrity(t *testing.T) {
	body := []byte("payload")
	sum := sha512.Sum512(body)
	good := "sha512-" + base64.StdEncoding.EncodeToString(sum[:])

	assert.NoError(t, verifyNpmIntegrity(body, good))
	assert.Error(t, verifyNpmIntegrity([]byte("other"), good))
	assert.Error(t, verifyNpmIntegrity(body, "sha1-abcdef")) // unsupported algorithm fails closed.
	assert.Error(t, verifyNpmIntegrity(body, ""))            // absent integrity fails closed.
}

func TestSafeDriverJoin_RejectsEscapes(t *testing.T) {
	dir := t.TempDir()

	inside, err := safeDriverJoin(dir, "package/cli.js")
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(dir, "package", "cli.js"), inside)

	_, err = safeDriverJoin(dir, "../escape.txt")
	assert.Error(t, err)
}

func TestNodePlatformSuffix_CurrentPlatform(t *testing.T) {
	// Compile/CI guard: every platform the test suite runs on must map cleanly.
	suffix, err := nodePlatformSuffix()
	require.NoError(t, err)
	assert.NotEmpty(t, suffix)
}
