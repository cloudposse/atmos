package cloudformation

import (
	"os"
	"testing"

	"github.com/cloudposse/atmos/pkg/data"
	iolib "github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/pkg/ui"
)

// TestMain initializes the data and UI output layers for the package's tests.
// The default I/O context resolves os.Stdout/os.Stderr dynamically at write time,
// so tests that capture output by swapping os.Stdout continue to work.
//
// It also lets this package's own test binary stand in for a cross-platform
// "write a marker file" command, used by hooks: fixtures that need to prove a
// hook actually ran (e.g. TestRunWithHooks_SetsFailureOutcome) without
// depending on platform-specific binaries like `touch`. Mirrors
// cmd/helmfile/testmain_test.go.
func TestMain(m *testing.M) {
	if path := os.Getenv("_ATMOS_TEST_WRITE_MARKER"); path != "" {
		if err := os.WriteFile(path, []byte("after-hook-fired"), 0o600); err != nil {
			os.Exit(1)
		}
		os.Exit(0)
	}

	ioCtx, err := iolib.NewContext()
	if err != nil {
		panic(err)
	}
	data.InitWriter(ioCtx)
	ui.InitFormatter(ioCtx)

	os.Exit(m.Run())
}
