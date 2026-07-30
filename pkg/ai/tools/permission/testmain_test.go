package permission

import (
	"os"
	"testing"

	iolib "github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/pkg/ui"
)

// TestMain initializes the global ui formatter before running tests in this
// package. Several tests (e.g. TestCLIPrompter_displayPrompt_*) redirect
// os.Stderr via os.Pipe to capture output written through ui.Write/Writef,
// ui.Warningf, and ui.Successf. Those package-level functions no-op until
// ui.InitFormatter has been called at least once, so without this TestMain
// the redirected pipes would capture nothing.
func TestMain(m *testing.M) {
	ioCtx, err := iolib.NewContext()
	if err != nil {
		panic(err)
	}
	ui.InitFormatter(ioCtx)

	os.Exit(m.Run())
}
