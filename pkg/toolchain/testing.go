package toolchain

import (
	"testing"

	"github.com/cloudposse/atmos/pkg/data"
	iolib "github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/pkg/ui"
)

// setupTestIO initializes the IO context for tests that use data or ui packages.
// This should be called at the beginning of any test that calls functions using
// data.Write*() or ui.*() functions.
func setupTestIO(t *testing.T) {
	t.Helper()
	ioCtx, err := iolib.NewContext()
	if err != nil {
		t.Fatalf("failed to create IO context: %v", err)
	}
	data.InitWriter(ioCtx)
	ui.InitFormatter(ioCtx)
}
