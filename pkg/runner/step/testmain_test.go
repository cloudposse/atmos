package step

import (
	"bufio"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/cloudposse/atmos/pkg/data"
	iolib "github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/pkg/ui"
)

// _ATMOS_STEP_SESSION_SHELL gates a fake interactive shell used to drive
// asciicast.RunSession from runCastSessionMode tests without depending on a
// real platform shell (there is no cross-platform "sh"/"cmd.exe" we can rely
// on in CI). The test binary itself impersonates the shell: it echoes a
// "ready" marker in response to a scripted "printf ready" line, mirroring
// pkg/asciicast's own session test helper.
const sessionShellHelperEnv = "_ATMOS_STEP_SESSION_SHELL"

// sessionShellDelayEnv simulates a subprocess scheduled after its PTY has
// already echoed input, exposing waits that mistake input echo for a response.
const sessionShellDelayEnv = "_ATMOS_STEP_SESSION_DELAY"

// _atmosStepFakeRMGlobEnv names the env var carrying the glob pattern that the
// "rm-glob-and-fail" _ATMOS_STEP_FAKE mode deletes before exiting non-zero.
// This lets cast_test.go exercise a child step that deletes a file and fails,
// without depending on the platform-specific "rm" binary or a shell.
const atmosStepFakeRMGlobEnv = "_ATMOS_STEP_FAKE_RM_GLOB"

// atmosStepFakeSleepMSEnv carries the sleep duration, in milliseconds, of the "sleep" _ATMOS_STEP_FAKE mode.
const atmosStepFakeSleepMSEnv = "_ATMOS_STEP_FAKE_SLEEP_MS"

// atmosStepFakeEnvNameEnv names the variable the "printenv" _ATMOS_STEP_FAKE mode prints.
const atmosStepFakeEnvNameEnv = "_ATMOS_STEP_FAKE_ENV_NAME"

// TestMain lets the test binary impersonate a fake "atmos" executable so that
// subprocess-executing handlers (e.g. AtmosHandler) can be tested
// cross-platform without a real atmos install.
//
// When _ATMOS_STEP_FAKE is set, the process behaves as the fake binary and
// exits immediately instead of running the test suite:
//   - "ok":               print a known marker to stdout, exit 0.
//   - "fail":             print an error marker to stderr, exit 3.
//   - "rm-glob-and-fail": delete files matching atmosStepFakeRMGlobEnv's glob
//     pattern, then exit 1.
//
// AtmosHandler.runAtmosCommand resolves the binary via os.Executable(), which
// in tests is this binary, so the sentinel is delivered via the step's env.
func TestMain(m *testing.M) {
	// _ATMOS_STEP_FAKE is checked before sessionShellHelperEnv: a test that
	// spawns a session (t.Setenv(sessionShellHelperEnv, "1"), a real process
	// env mutation) and then, in the same test, runs a real `type: atmos`/
	// `type: shell` child step wants that child's subprocess to behave as
	// the fake command it explicitly configured via step Env -- not as the
	// shell helper, whose sentinel it only inherited ambiently through
	// os.Environ(). An explicit per-step request must win over an inherited
	// ambient one.
	switch os.Getenv("_ATMOS_STEP_FAKE") {
	case "ok":
		_, _ = os.Stdout.WriteString("fake-atmos-output")
		os.Exit(0)
	case "fail":
		_, _ = os.Stderr.WriteString("fake-atmos-error")
		os.Exit(3)
	case "rm-glob-and-fail":
		removeGlobMatches(os.Getenv(atmosStepFakeRMGlobEnv))
		os.Exit(1)
	case "spin-output":
		_, _ = os.Stdout.WriteString("spin-stdout")
		_, _ = os.Stderr.WriteString("spin-stderr")
		os.Exit(0)
	case "sleep":
		// Sleep for _ATMOS_STEP_FAKE_SLEEP_MS milliseconds so tests can exercise step timeouts.
		if ms, err := strconv.Atoi(os.Getenv(atmosStepFakeSleepMSEnv)); err == nil {
			time.Sleep(time.Duration(ms) * time.Millisecond)
		}
		os.Exit(0)
	case "printenv":
		// Print the value of the variable named by _ATMOS_STEP_FAKE_ENV_NAME.
		_, _ = os.Stdout.WriteString(os.Getenv(os.Getenv(atmosStepFakeEnvNameEnv)))
		os.Exit(0)
	}
	if os.Getenv(sessionShellHelperEnv) == "1" {
		runStepSessionShellHelper()
		os.Exit(0)
	}

	ioCtx, err := iolib.NewContext()
	if err != nil {
		panic(err)
	}
	data.InitWriter(ioCtx)
	ui.InitFormatter(ioCtx)

	builtInStepTypes = registeredStepTypesForDocumentation()
	os.Exit(m.Run())
}

// removeGlobMatches deletes every file matching pattern, ignoring a pattern
// that matches nothing.
func removeGlobMatches(pattern string) {
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return
	}
	for _, match := range matches {
		_ = os.Remove(match)
	}
}

// runStepSessionShellHelper is a minimal line-oriented "shell" driven over
// stdin/stdout: it recognizes a couple of scripted commands used by
// runCastSessionMode tests and echoes deterministic output for each.
func runStepSessionShellHelper() {
	if os.Getenv(sessionShellDelayEnv) == "1" {
		time.Sleep(3 * time.Second)
	}
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
