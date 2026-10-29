package native

import (
	"os"
	"testing"

	"github.com/cloudposse/atmos/pkg/plugin/testkit/fakeplugin"
)

// TestMain lets the test binary act as the plugin process (see fakeplugin).
func TestMain(m *testing.M) {
	if fakeplugin.MaybeServe() {
		return
	}
	os.Exit(m.Run())
}
