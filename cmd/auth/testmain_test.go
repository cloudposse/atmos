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
func TestMain(m *testing.M) {
	ioCtx, err := iolib.NewContext()
	if err != nil {
		// Tests cannot run without an IO context; fail loud.
		panic("cmd/auth tests: failed to create IO context: " + err.Error())
	}
	data.InitWriter(ioCtx)
	ui.InitFormatter(ioCtx)

	os.Exit(m.Run())
}
