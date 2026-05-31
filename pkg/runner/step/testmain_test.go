package step

import (
	"os"
	"testing"
)

// TestMain lets the test binary impersonate a fake "atmos" executable so that
// subprocess-executing handlers (e.g. AtmosHandler) can be tested
// cross-platform without a real atmos install.
//
// When _ATMOS_STEP_FAKE is set, the process behaves as the fake binary and
// exits immediately instead of running the test suite:
//   - "ok":   print a known marker to stdout, exit 0.
//   - "fail": print an error marker to stderr, exit 3.
//
// AtmosHandler.runAtmosCommand resolves the binary via os.Executable(), which
// in tests is this binary, so the sentinel is delivered via the step's env.
func TestMain(m *testing.M) {
	switch os.Getenv("_ATMOS_STEP_FAKE") {
	case "ok":
		_, _ = os.Stdout.WriteString("fake-atmos-output")
		os.Exit(0)
	case "fail":
		_, _ = os.Stderr.WriteString("fake-atmos-error")
		os.Exit(3)
	}

	os.Exit(m.Run())
}
