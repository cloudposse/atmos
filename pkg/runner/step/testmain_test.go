package step

import (
	"bufio"
	"io"
	"os"
	"strings"
	"testing"
)

// _ATMOS_STEP_SESSION_SHELL gates a fake interactive shell used to drive
// asciicast.RunSession from runCastSessionMode tests without depending on a
// real platform shell (there is no cross-platform "sh"/"cmd.exe" we can rely
// on in CI). The test binary itself impersonates the shell: it echoes a
// "ready" marker in response to a scripted "printf ready" line, mirroring
// pkg/asciicast's own session test helper.
const sessionShellHelperEnv = "_ATMOS_STEP_SESSION_SHELL"

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
	if os.Getenv(sessionShellHelperEnv) == "1" {
		runStepSessionShellHelper()
		os.Exit(0)
	}
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

// runStepSessionShellHelper is a minimal line-oriented "shell" driven over
// stdin/stdout: it recognizes a couple of scripted commands used by
// runCastSessionMode tests and echoes deterministic output for each.
func runStepSessionShellHelper() {
	reader := bufio.NewReader(os.Stdin)
	var line strings.Builder
	for {
		b, err := reader.ReadByte()
		if err != nil {
			if err == io.EOF {
				return
			}
			os.Exit(1)
		}
		switch b {
		case 4:
			return
		case '\r', '\n':
			if strings.TrimSpace(line.String()) == "printf ready" {
				_, _ = os.Stdout.WriteString("ready\n")
			}
			line.Reset()
		default:
			_ = line.WriteByte(b)
		}
	}
}
