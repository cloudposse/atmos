package env

import (
	"os"
	"testing"

	"github.com/cloudposse/atmos/pkg/data"
	iolib "github.com/cloudposse/atmos/pkg/io"
)

// TestMain initialises data.Write/data.WriteUnmasked against the SAME global
// I/O context that iolib.RegisterSecret (used throughout output_test.go) and
// iolib.MaskString operate on. Using a separately-constructed iolib.NewContext()
// here instead would give data.Write its own, unrelated masker instance, so
// secrets registered via iolib.RegisterSecret in individual tests would never
// actually get masked by data.Write. This mirrors cmd/root.go's real startup
// sequence: iolib.Initialize() then iolib.GetContext().
func TestMain(m *testing.M) {
	if err := iolib.Initialize(); err != nil {
		panic("pkg/env: failed to initialize IO context: " + err.Error())
	}
	data.InitWriter(iolib.GetContext())

	os.Exit(m.Run())
}
