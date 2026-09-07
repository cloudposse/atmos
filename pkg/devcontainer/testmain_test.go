package devcontainer

import (
	"os"
	"testing"

	"github.com/cloudposse/atmos/pkg/data"
	iolib "github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/pkg/ui"
)

// TestMain initializes the I/O writer and ui formatter so that data.Write*
// (used by lifecycle_config.go and lifecycle_list.go for primary command
// output) and ui.Warning*/ui.Infof (status messages) actually emit output
// during tests instead of silently no-oping or panicking.
func TestMain(m *testing.M) {
	ioCtx, err := iolib.NewContext()
	if err != nil {
		panic("pkg/devcontainer: failed to create IO context: " + err.Error())
	}
	data.InitWriter(ioCtx)
	ui.InitFormatter(ioCtx)
	os.Exit(m.Run())
}
