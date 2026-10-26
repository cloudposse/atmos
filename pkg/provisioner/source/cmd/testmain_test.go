package cmd

import (
	"os"
	"testing"

	"github.com/cloudposse/atmos/pkg/data"
	iolib "github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/pkg/ui"
)

// TestMain initializes the I/O writer and ui formatter so data.Write*/ui.Write*
// calls (used transitively via pkg/utils.PrintAsYAML/PrintAsJSON in describe.go)
// don't panic or silently no-op during tests.
func TestMain(m *testing.M) {
	ioCtx, err := iolib.NewContext()
	if err != nil {
		panic("pkg/provisioner/source/cmd: failed to create IO context: " + err.Error())
	}
	data.InitWriter(ioCtx)
	ui.InitFormatter(ioCtx)

	os.Exit(m.Run())
}
