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
	"errors"
	"fmt"
	"io"
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

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

type errorReader struct{}

func (errorReader) Read([]byte) (int, error) {
	return 0, errors.New("read failed")
}

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

func makeGzip(t *testing.T, content string) []byte {
	t.Helper()
	var buf bytes.Buffer
	gzWriter := gzip.NewWriter(&buf)
	_, err := gzWriter.Write([]byte(content))
	require.NoError(t, err)
	require.NoError(t, gzWriter.Close())
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

func overridePreseedHTTPClient(t *testing.T, client *http.Client) {
	t.Helper()
	original := preseedHTTPClient
	preseedHTTPClient = client
	t.Cleanup(func() { preseedHTTPClient = original })
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

func skipIfNodejsOrgArchiveUnsupported(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "linux" && hostUsesMuslLibc() {
		t.Skip("nodejs.org Linux archives are glibc-linked; musl requires PLAYWRIGHT_NODEJS_PATH")
	}
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
	skipIfNodejsOrgArchiveUnsupported(t)

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
	skipIfNodejsOrgArchiveUnsupported(t)

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

func TestSeedPlaywrightCoreDownloadFailures(t *testing.T) {
	t.Run("metadata download error", func(t *testing.T) {
		overridePreseedHTTPClient(t, &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			return nil, errors.New("network down")
		})})

		err := seedPlaywrightCore(t.TempDir(), "1.2.3")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "could not download")
	})

	t.Run("tarball download error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/playwright-core/1.2.3" {
				fmt.Fprintf(w, `{"dist":{"tarball":%q,"integrity":"sha512-any"}}`, serverURL(t, r)+"/missing.tgz")
				return
			}
			http.NotFound(w, r)
		}))
		defer server.Close()
		overridePreseedHosts(t, server.URL, server.URL)

		err := seedPlaywrightCore(t.TempDir(), "1.2.3")
		require.Error(t, err)
		assert.ErrorIs(t, err, errUtils.ErrPlaywrightDriverSeed)
		assert.Contains(t, err.Error(), "returned status 404")
	})
}

func serverURL(t *testing.T, r *http.Request) string {
	t.Helper()
	return "http://" + r.Host
}

func TestNpmPackageDistFailures(t *testing.T) {
	tests := []struct {
		name       string
		body       string
		statusCode int
		wantText   string
	}{
		{
			name:       "invalid json",
			body:       "{not json",
			statusCode: http.StatusOK,
			wantText:   "could not parse npm metadata",
		},
		{
			name:       "missing tarball",
			body:       `{"dist":{"integrity":"sha512-value"}}`,
			statusCode: http.StatusOK,
			wantText:   "has no tarball URL",
		},
		{
			name:       "registry status error",
			body:       "not found",
			statusCode: http.StatusNotFound,
			wantText:   "returned status 404",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tt.statusCode)
				_, _ = w.Write([]byte(tt.body))
			}))
			defer server.Close()
			overridePreseedHosts(t, server.URL, server.URL)

			tarballURL, integrity, err := npmPackageDist("playwright-core", "1.2.3")
			require.Error(t, err)
			assert.Empty(t, tarballURL)
			assert.Empty(t, integrity)
			assert.Contains(t, err.Error(), tt.wantText)
		})
	}
}

func TestExtractNpmPackageFailures(t *testing.T) {
	tests := []struct {
		name     string
		body     []byte
		wantText string
	}{
		{
			name:     "invalid gzip",
			body:     []byte("not a gzip"),
			wantText: "could not read playwright-core archive",
		},
		{
			name:     "no package files",
			body:     makeTgz(t, map[string]string{"unrelated/file.txt": "skip"}),
			wantText: "no files extracted",
		},
		{
			name:     "corrupt tar",
			body:     makeGzip(t, "not a tar stream"),
			wantText: "could not read playwright-core archive",
		},
		{
			name:     "entry escapes driver directory",
			body:     makeTgz(t, map[string]string{"package/../../escape.txt": "bad"}),
			wantText: "escapes the driver directory",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := extractNpmPackage(tt.body, t.TempDir())
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantText)
		})
	}
}

func TestExtractNpmPackageWriteFailure(t *testing.T) {
	driverDir := filepath.Join(t.TempDir(), "driver-file")
	require.NoError(t, os.WriteFile(driverDir, []byte("not a directory"), 0o644))

	err := extractNpmPackage(makeTgz(t, map[string]string{"package/cli.js": "cli"}), driverDir)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "could not create directory")
}

func TestSeedNodeBinaryShortCircuits(t *testing.T) {
	t.Run("node path set", func(t *testing.T) {
		t.Setenv("PLAYWRIGHT_NODEJS_PATH", "/usr/bin/node")
		overridePreseedHosts(t, "http://127.0.0.1:1", "http://127.0.0.1:1")

		assert.NoError(t, seedNodeBinary(t.TempDir()))
	})

	t.Run("node already exists", func(t *testing.T) {
		t.Setenv("PLAYWRIGHT_NODEJS_PATH", "")
		overridePreseedHosts(t, "http://127.0.0.1:1", "http://127.0.0.1:1")

		driverDir := t.TempDir()
		nodeName := "node"
		if runtime.GOOS == windowsOS {
			nodeName = "node.exe"
		}
		require.NoError(t, os.WriteFile(filepath.Join(driverDir, nodeName), []byte("node"), 0o755))

		assert.NoError(t, seedNodeBinary(driverDir))
	})
}

func TestSeedNodeBinaryDownloadFailure(t *testing.T) {
	skipIfNodejsOrgArchiveUnsupported(t)
	t.Setenv("PLAYWRIGHT_NODEJS_PATH", "")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()
	overridePreseedHosts(t, server.URL, server.URL)

	err := seedNodeBinary(t.TempDir())
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrPlaywrightDriverSeed)
	assert.Contains(t, err.Error(), "returned status 404")
}

func TestVerifyNodeChecksumMissingEntry(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("deadbeef  other-archive.tar.gz\n"))
	}))
	defer server.Close()
	overridePreseedHosts(t, server.URL, server.URL)

	err := verifyNodeChecksum([]byte("node archive"), "node-archive.tar.gz")
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrPlaywrightDriverSeed)
	assert.Contains(t, err.Error(), "no published checksum found")
}

func TestVerifyNodeChecksumDownloadFailure(t *testing.T) {
	overridePreseedHTTPClient(t, &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("network down")
	})})

	err := verifyNodeChecksum([]byte("node archive"), "node-archive.tar.gz")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "could not download")
}

func TestExtractTarGzSingleEntryFailures(t *testing.T) {
	destPath := filepath.Join(t.TempDir(), "node")

	err := extractTarGzSingleEntry([]byte("not a gzip"), "node/bin/node", destPath)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "could not read Node.js archive")

	err = extractTarGzSingleEntry(makeGzip(t, "not a tar stream"), "node/bin/node", destPath)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "could not read Node.js archive")

	err = extractTarGzSingleEntry(makeTgz(t, map[string]string{"other": "content"}), "node/bin/node", destPath)
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrPlaywrightDriverSeed)
	assert.Contains(t, err.Error(), "entry node/bin/node not found")
}

func TestExtractZipSingleEntry(t *testing.T) {
	destPath := filepath.Join(t.TempDir(), "node.exe")
	err := extractZipSingleEntry(makeZip(t, map[string]string{"node/node.exe": "zip node"}), "node/node.exe", destPath)
	require.NoError(t, err)
	body, err := os.ReadFile(destPath)
	require.NoError(t, err)
	assert.Equal(t, "zip node", string(body))

	err = extractZipSingleEntry(makeZip(t, map[string]string{"other": "content"}), "node/node.exe", filepath.Join(t.TempDir(), "missing.exe"))
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrPlaywrightDriverSeed)
	assert.Contains(t, err.Error(), "entry node/node.exe not found")

	err = extractZipSingleEntry([]byte("not a zip"), "node/node.exe", filepath.Join(t.TempDir(), "bad.exe"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "could not read Node.js archive")
}

func TestWritePreseedFileCopyError(t *testing.T) {
	err := writePreseedFile(filepath.Join(t.TempDir(), "node"), errorReader{}, preseedFileMode)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "could not write file")
}

func TestWritePreseedFileOpenError(t *testing.T) {
	err := writePreseedFile(t.TempDir(), bytes.NewReader([]byte("node")), preseedFileMode)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "could not create file")
}

func TestPreseedDownloadFailures(t *testing.T) {
	t.Run("request error", func(t *testing.T) {
		overridePreseedHTTPClient(t, &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			return nil, errors.New("dial failed")
		})})

		body, err := preseedDownload("http://example.invalid/archive.tgz")
		require.Error(t, err)
		assert.Nil(t, body)
		assert.Contains(t, err.Error(), "could not download")
	})

	t.Run("status error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "missing", http.StatusNotFound)
		}))
		defer server.Close()

		body, err := preseedDownload(server.URL)
		require.Error(t, err)
		assert.Nil(t, body)
		assert.ErrorIs(t, err, errUtils.ErrPlaywrightDriverSeed)
		assert.Contains(t, err.Error(), "returned status 404")
	})

	t.Run("read error", func(t *testing.T) {
		overridePreseedHTTPClient(t, &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(errorReader{}),
			}, nil
		})})

		body, err := preseedDownload("http://example.invalid/archive.tgz")
		require.Error(t, err)
		assert.Nil(t, body)
		assert.Contains(t, err.Error(), "could not read")
	})
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
	suffix, err := nodePlatformSuffix()
	if runtime.GOOS == "linux" && hostUsesMuslLibc() {
		require.Error(t, err)
		assert.ErrorIs(t, err, errUtils.ErrPlaywrightDriverSeed)
		assert.Contains(t, err.Error(), "PLAYWRIGHT_NODEJS_PATH")
		assert.Contains(t, err.Error(), "musl")
		return
	}

	// Compile/CI guard: every supported platform the test suite runs on must map cleanly.
	require.NoError(t, err)
	assert.NotEmpty(t, suffix)
}

func TestNodePlatformSuffixFor_KnownPlatforms(t *testing.T) {
	tests := []struct {
		name       string
		goos       string
		goarch     string
		linuxMusl  bool
		wantSuffix string
	}{
		{
			name:       "linux amd64 glibc",
			goos:       "linux",
			goarch:     "amd64",
			wantSuffix: "linux-x64",
		},
		{
			name:       "linux arm64 glibc",
			goos:       "linux",
			goarch:     "arm64",
			wantSuffix: "linux-arm64",
		},
		{
			name:       "darwin arm64",
			goos:       "darwin",
			goarch:     "arm64",
			wantSuffix: "darwin-arm64",
		},
		{
			name:       "windows amd64",
			goos:       "windows",
			goarch:     "amd64",
			wantSuffix: "win-x64",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			suffix, err := nodePlatformSuffixFor(tt.goos, tt.goarch, tt.linuxMusl)
			require.NoError(t, err)
			assert.Equal(t, tt.wantSuffix, suffix)
		})
	}
}

func TestNodePlatformSuffixFor_UnsupportedPlatformsRequireNodePath(t *testing.T) {
	tests := []struct {
		name      string
		goos      string
		goarch    string
		linuxMusl bool
		wantText  string
	}{
		{
			name:     "unsupported arch",
			goos:     "linux",
			goarch:   "ppc64le",
			wantText: "no prebuilt Node.js",
		},
		{
			name:     "unsupported os",
			goos:     "freebsd",
			goarch:   "amd64",
			wantText: "no prebuilt Node.js",
		},
		{
			name:      "linux musl",
			goos:      "linux",
			goarch:    "amd64",
			linuxMusl: true,
			wantText:  "musl-compatible Node.js",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			suffix, err := nodePlatformSuffixFor(tt.goos, tt.goarch, tt.linuxMusl)
			require.Error(t, err)
			assert.Empty(t, suffix)
			assert.ErrorIs(t, err, errUtils.ErrPlaywrightDriverSeed)
			assert.Contains(t, err.Error(), "PLAYWRIGHT_NODEJS_PATH")
			assert.Contains(t, err.Error(), tt.wantText)
		})
	}
}
