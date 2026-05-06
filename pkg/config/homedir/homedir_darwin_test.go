//go:build darwin

package homedir

import (
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestGetDarwinHomeDir_RealDscl verifies that getDarwinHomeDir returns an
// absolute path on macOS when dscl is available. The test is skipped when dscl
// is not found on PATH (e.g., in containers without macOS tooling).
//
// This is an integration test: it exercises the full dscl code path with the
// real tool, so it is gated to darwin builds only.
func TestGetDarwinHomeDir_RealDscl(t *testing.T) {
	// Skip if dscl is unavailable — distroless or Docker containers may lack it.
	if _, err := exec.LookPath("dscl"); err != nil {
		t.Skip("dscl not found on PATH; skipping macOS dscl integration test.")
	}

	// Retrieve the current username independently so the test does not rely on
	// $USER being set.
	username, err := shellGetUsernameFunc()
	if err != nil {
		t.Skipf("cannot determine current username (%v); skipping.", err)
	}

	// HOME is cleared here as a precaution; getDarwinHomeDir queries dscl
	// directly and does not read $HOME, so this has no functional effect
	// on the function under test.
	t.Setenv("HOME", "")

	home, err := getDarwinHomeDir(username)
	require.NoError(t, err, "getDarwinHomeDir must succeed when dscl is available.")
	assert.True(t, filepath.IsAbs(home), "getDarwinHomeDir must return an absolute path; got %q.", home)
	assert.NotEmpty(t, home, "getDarwinHomeDir must return a non-empty path.")
}
