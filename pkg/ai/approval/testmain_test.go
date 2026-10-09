package approval

import (
	"os"
	"testing"

	iolib "github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/pkg/ui"
)

// TestMain initializes the global ui formatter so code paths that write through
// the ui package behave the same as in the CLI.
func TestMain(m *testing.M) {
	ioCtx, err := iolib.NewContext()
	if err != nil {
		panic(err)
	}
	ui.InitFormatter(ioCtx)

	os.Exit(m.Run())
}
