package version

import (
	"os"
	"testing"

	iolib "github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/pkg/ui"
)

// TestMain initializes the ui formatter so that ui.Write*/ui.Warning* calls
// (used by formatters.go) actually emit output during tests instead of
// silently no-oping.
func TestMain(m *testing.M) {
	ioCtx, err := iolib.NewContext()
	if err != nil {
		panic("cmd/version: failed to create IO context: " + err.Error())
	}
	ui.InitFormatter(ioCtx)
	os.Exit(m.Run())
}
