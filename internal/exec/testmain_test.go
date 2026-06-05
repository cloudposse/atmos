package exec

import (
	"os"
	"testing"
)

// TestMain is the entry point for the internal/exec test binary.
// It intercepts several env vars before any test runs, enabling tests to use
// the test binary itself as a portable subprocess — no Unix-only binaries required.
//
// Supported env vars (processed in declaration order):
//
//	_ATMOS_TEST_COUNTER_FILE=<path>  — if set, append one byte ("x") to <path>
//	                                   on every invocation (for single-invocation
//	                                   regression guard in terraform_execute_single_invocation_test.go).
//	_ATMOS_TEST_EXIT_ONE=1           — if set, exit 1 immediately after the optional
//	                                   counter-file write (for workspace recovery tests).
func TestMain(m *testing.M) {
	// Write a single byte to the counter file on every invocation.
	// This lets tests count how many times the subprocess was spawned by reading
	// the file length: len(file) == number of invocations.
	if counterFile := os.Getenv("_ATMOS_TEST_COUNTER_FILE"); counterFile != "" {
		fd, err := os.OpenFile(counterFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
		if err == nil {
			_, _ = fd.WriteString("x")
			_ = fd.Close()
		}
	}

	// Subprocess helper: when the test binary is invoked as the "terraform" command,
	// this env var causes it to exit 1 immediately, simulating a failed workspace
	// command without requiring the POSIX "false" command.
	if os.Getenv("_ATMOS_TEST_EXIT_ONE") == "1" {
		os.Exit(1)
	}

	// Isolate the Terraform provider plugin cache for this package's tests.
	// Terraform's plugin cache is NOT safe for concurrent use, and `go test ./...`
	// runs package test binaries in parallel. Sharing the global XDG plugin cache
	// (TF_PLUGIN_CACHE_DIR -> ~/.cache/atmos/terraform/plugins) with other packages'
	// concurrent terraform invocations races on Windows, surfacing as
	// "plugin cache dir cannot be opened" / "Required plugins are not installed".
	// Redirecting XDG_CACHE_HOME to a per-binary temp dir removes the contention.
	// The real-terraform tests in this package run serially, so they safely share
	// this private cache (isolated and faster: a single provider download).
	// TestMain only has *testing.M, so t.TempDir/t.Setenv are unavailable here;
	// os.MkdirTemp/os.Setenv with explicit cleanup below is the only option.
	cacheDir, err := os.MkdirTemp("", "atmos-exec-xdg-cache-") //nolint:lintroller // no *testing.T in TestMain; cleaned up below.
	if err == nil {
		_ = os.Setenv("XDG_CACHE_HOME", cacheDir) //nolint:lintroller // no *testing.T in TestMain; process-level isolation for the whole binary.
	}

	code := m.Run()

	if cacheDir != "" {
		_ = os.RemoveAll(cacheDir) // os.Exit skips defers; clean up explicitly.
	}
	os.Exit(code)
}
