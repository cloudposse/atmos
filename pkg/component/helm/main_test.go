package helm

import (
	"os"
	"testing"

	"github.com/cloudposse/atmos/pkg/data"
	iolib "github.com/cloudposse/atmos/pkg/io"
)

// TestMain initializes the data-package writer once for the whole package. Apply/delete now emit a
// status line via data.Writeln (issue #2), which panics if the writer was never initialized; doing it
// here keeps every test that exercises those operations working without per-test setup.
func TestMain(m *testing.M) {
	ioCtx, err := iolib.NewContext()
	if err != nil {
		panic(err)
	}
	data.InitWriter(ioCtx)
	os.Exit(m.Run())
}
