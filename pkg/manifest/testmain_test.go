package manifest

import (
	"os"
	"testing"

	"github.com/cloudposse/atmos/pkg/data"
	iolib "github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/pkg/ui"
)

// TestMain initializes the global I/O context and UI formatter once for the
// package so tests that exercise WriteObjects (and its stdout output path via
// data.Write) don't panic with "data.InitWriter() must be called before using
// data package functions".
//
// The default I/O context resolves os.Stdout dynamically at write time, so
// tests that capture output by swapping os.Stdout (see captureManifestStdout
// in render_extra_test.go) continue to work.
func TestMain(m *testing.M) {
	ioCtx, err := iolib.NewContext()
	if err != nil {
		panic("pkg/manifest: failed to create IO context: " + err.Error())
	}
	data.InitWriter(ioCtx)
	ui.InitFormatter(ioCtx)

	os.Exit(m.Run())
}
