package helmfile

import (
	"os"
	"testing"
)

// TestMain lets this package's own test binary stand in for a cross-platform
// "write a marker file" command, used by hooks: fixtures that need to prove a
// hook actually ran (e.g. TestHelmfileRun_NodeHooksFallbackOnEarlyFailure)
// without depending on platform-specific binaries like `touch`.
func TestMain(m *testing.M) {
	if path := os.Getenv("_ATMOS_TEST_WRITE_MARKER"); path != "" {
		if err := os.WriteFile(path, []byte("after-hook-fired"), 0o600); err != nil {
			os.Exit(1)
		}
		os.Exit(0)
	}
	os.Exit(m.Run())
}
