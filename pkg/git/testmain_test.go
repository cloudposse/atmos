package git

import (
	"os"
	"strconv"
	"testing"
)

const (
	// The helperStderrEnv variable makes the test binary act as a subprocess that writes its value to stderr.
	helperStderrEnv = "_ATMOS_TEST_GIT_HELPER_STDERR"
	// The helperExitEnv variable sets the exit code of the helper subprocess.
	helperExitEnv = "_ATMOS_TEST_GIT_HELPER_EXIT"
)

// TestMain lets tests use the test binary itself as a cross-platform subprocess that
// writes a fixed stderr payload (without a trailing newline) and exits with a chosen code.
func TestMain(m *testing.M) {
	if payload, ok := os.LookupEnv(helperStderrEnv); ok {
		_, _ = os.Stderr.WriteString(payload)
		code, _ := strconv.Atoi(os.Getenv(helperExitEnv))
		os.Exit(code)
	}
	os.Exit(m.Run())
}
