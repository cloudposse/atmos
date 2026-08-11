package cacerts

import (
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLocate_Windows(t *testing.T) {
	// Windows uses Schannel and has no canonical file-based bundle path.
	// locate() must return "" on windows regardless of what files exist.
	assert.Equal(t, "", locate("windows"))
}

func TestLocate_FindsExistingPath(t *testing.T) {
	// Verify the locate function returns SOMETHING on Unix-likes, since
	// every reasonable test runner has at least one of the candidate
	// paths populated by the OS or by ca-certificates packages.
	if runtime.GOOS == "windows" {
		t.Skip("no candidate file-based bundle on Windows")
	}
	got := locate(runtime.GOOS)
	assert.NotEmpty(t, got, "expected to find at least one CA bundle on %s", runtime.GOOS)

	// Sanity check: whatever was returned must exist and be a regular file.
	info, err := os.Stat(got)
	require.NoError(t, err)
	assert.False(t, info.IsDir(), "returned path %q must be a file", got)
}

func TestFind_CachesResult(t *testing.T) {
	// Find() should hit sync.Once on the first call and return the same
	// path on subsequent calls without re-walking the candidate list.
	first := Find()
	second := Find()
	assert.Equal(t, first, second)
}

func TestEnv_NoBundle(t *testing.T) {
	// When no bundle path is known, Env() returns nil — callers add
	// nothing to the subprocess environment.
	savedPath := cachedPath
	savedOnce := findOnce
	cachedPath = ""
	findOnce = new(sync.Once)
	findOnce.Do(func() {})
	t.Cleanup(func() {
		cachedPath = savedPath
		findOnce = savedOnce
	})

	// Force the saved value to take effect without invoking sync.Once.
	got := Env()
	assert.Nil(t, got)
}

func TestEnv_WithBundle(t *testing.T) {
	// Stub a known-good path and verify Env() returns both expected keys
	// pointing at it. Both env vars must be set so we cover both Python
	// "ssl" and Python "requests" library conventions in one shot.
	dir := t.TempDir()
	pem := filepath.Join(dir, "cert.pem")
	require.NoError(t, os.WriteFile(pem, []byte("dummy"), 0o644))

	savedPath := cachedPath
	savedOnce := findOnce
	cachedPath = pem
	findOnce = new(sync.Once)
	findOnce.Do(func() {})
	t.Cleanup(func() {
		cachedPath = savedPath
		findOnce = savedOnce
	})

	got := Env()
	require.NotNil(t, got)
	assert.Equal(t, pem, got[EnvSSLCertFile])
	assert.Equal(t, pem, got[EnvRequestsCABundle])
}

func TestEnv_VarNamesAreCanonical(t *testing.T) {
	// Guard against accidental rename: downstream tools and ops runbooks
	// depend on these exact env var names. Failure here means we have
	// to update upstream consumers (or back out the rename).
	assert.Equal(t, "SSL_CERT_FILE", EnvSSLCertFile)
	assert.Equal(t, "REQUESTS_CA_BUNDLE", EnvRequestsCABundle)
}

// isolateXDGCache points ATMOS_XDG_CACHE_HOME at a fresh temp dir for the duration of the test, so
// BuildBundle tests never read or write the real user cache.
func isolateXDGCache(t *testing.T) {
	t.Helper()
	t.Setenv("ATMOS_XDG_CACHE_HOME", t.TempDir())
}

func TestBuildBundle_CombinesSystemAndExtra(t *testing.T) {
	isolateXDGCache(t)

	// Stub a deterministic "system bundle" the same way TestEnv_WithBundle does.
	dir := t.TempDir()
	sysPEM := filepath.Join(dir, "system.pem")
	require.NoError(t, os.WriteFile(sysPEM, []byte("SYSTEM_ROOT_MARKER\n"), 0o644))
	savedPath, savedOnce := cachedPath, findOnce
	cachedPath = sysPEM
	findOnce = new(sync.Once)
	findOnce.Do(func() {})
	t.Cleanup(func() { cachedPath, findOnce = savedPath, savedOnce })

	extra := []byte("EXTRA_CA_MARKER\n")
	path, err := BuildBundle(extra)
	require.NoError(t, err)
	require.NotEmpty(t, path)

	contents, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Contains(t, string(contents), "SYSTEM_ROOT_MARKER", "the combined bundle must keep the system roots")
	assert.Contains(t, string(contents), "EXTRA_CA_MARKER", "the combined bundle must append the extra CA")
}

func TestBuildBundle_NoSystemBundleUsesExtraOnly(t *testing.T) {
	isolateXDGCache(t)

	// Stub "no system bundle found".
	savedPath, savedOnce := cachedPath, findOnce
	cachedPath = ""
	findOnce = new(sync.Once)
	findOnce.Do(func() {})
	t.Cleanup(func() { cachedPath, findOnce = savedPath, savedOnce })

	extra := []byte("ONLY_EXTRA_MARKER\n")
	path, err := BuildBundle(extra)
	require.NoError(t, err)
	require.NotEmpty(t, path, "extra alone is enough to produce a bundle even with no system store")

	contents, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, extra, contents)
}

func TestBuildBundle_NothingToWriteReturnsEmpty(t *testing.T) {
	isolateXDGCache(t)

	savedPath, savedOnce := cachedPath, findOnce
	cachedPath = ""
	findOnce = new(sync.Once)
	findOnce.Do(func() {})
	t.Cleanup(func() { cachedPath, findOnce = savedPath, savedOnce })

	path, err := BuildBundle(nil)
	require.NoError(t, err)
	assert.Empty(t, path, "no system bundle and no extra means nothing to write")
}

func TestBuildBundle_StableContentHashedPathIsReused(t *testing.T) {
	isolateXDGCache(t)

	savedPath, savedOnce := cachedPath, findOnce
	cachedPath = ""
	findOnce = new(sync.Once)
	findOnce.Do(func() {})
	t.Cleanup(func() { cachedPath, findOnce = savedPath, savedOnce })

	extra := []byte("REUSE_MARKER\n")
	first, err := BuildBundle(extra)
	require.NoError(t, err)
	require.NotEmpty(t, first)

	// Prove reuse, not just path equality: the content-hash filename alone would already guarantee
	// two calls return the same PATH regardless of whether writeBundle's os.Stat short-circuit
	// actually skips the rewrite. Write a sentinel marker onto the materialized file, call
	// BuildBundle again with the SAME extra, and confirm the marker survives. If the os.Stat check
	// were removed, writeBundle would call fs.WriteFileAtomic unconditionally, which overwrites the
	// file with `extra`'s content and would wipe the marker out.
	const sentinel = "SENTINEL-DO-NOT-OVERWRITE"
	require.NoError(t, os.WriteFile(first, []byte(sentinel), 0o644))

	second, err := BuildBundle(extra)
	require.NoError(t, err)
	// Same content must resolve to the SAME stable path across calls (content-hash-keyed), so a
	// --print-command-printed path stays valid instead of pointing at a one-shot temp file.
	assert.Equal(t, first, second)

	contents, err := os.ReadFile(second)
	require.NoError(t, err)
	assert.Equal(t, sentinel, string(contents), "BuildBundle must NOT rewrite an already-materialized bundle for the same content hash")
}
