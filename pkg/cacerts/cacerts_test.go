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
