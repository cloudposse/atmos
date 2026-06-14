package kubernetes

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
func TestMain(m *testing.M) {
	ioCtx, err := iolib.NewContext()
	if err != nil {
		panic(err)
	}
	data.InitWriter(ioCtx)
	ui.InitFormatter(ioCtx)

	os.Exit(m.Run())
}
