package telemetry

import (
	"os"
	"testing"

	iolib "github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/pkg/ui"
)

// TestMain initializes the ui formatter so that ui.MarkdownMessageNoWrapf
// (used by PrintTelemetryDisclosure) actually emits output during tests
// instead of silently no-oping.
func TestMain(m *testing.M) {
	ioCtx, err := iolib.NewContext()
	if err != nil {
		panic("pkg/telemetry: failed to create IO context: " + err.Error())
	}
	ui.InitFormatter(ioCtx)
	os.Exit(m.Run())
}
