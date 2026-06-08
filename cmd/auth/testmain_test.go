package auth

import (
	"os"
	"testing"

	"github.com/cloudposse/atmos/pkg/data"
	iolib "github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/pkg/ui"
)

// TestMain initialises the global I/O context and UI formatter once for the
// package so tests that exercise printWhoamiJSON, printWhoamiHuman, list
// renderers, etc. don't panic with "data.InitWriter() must be called".
//
// It also implements the cross-platform "exit 0 / exit 1" subprocess pattern
// described in CLAUDE.md so executeCommandWithEnv tests can spawn the test
// binary itself instead of relying on Unix-only `true` / `false` or
// PATH-dependent binaries like `go`. Tests set _ATMOS_AUTH_TEST_EXIT_OK=1 or
// _ATMOS_AUTH_TEST_EXIT_ONE=1 and use os.Executable() as the command.
func TestMain(m *testing.M) {
	if os.Getenv("_ATMOS_AUTH_TEST_EXIT_OK") == "1" {
		os.Exit(0)
	}
	if os.Getenv("_ATMOS_AUTH_TEST_EXIT_ONE") == "1" {
		os.Exit(1)
	}

	ioCtx, err := iolib.NewContext()
	if err != nil {
		// Tests cannot run without an IO context; fail loud.
		panic("cmd/auth tests: failed to create IO context: " + err.Error())
	}
	data.InitWriter(ioCtx)
	ui.InitFormatter(ioCtx)

	os.Exit(m.Run())
}
