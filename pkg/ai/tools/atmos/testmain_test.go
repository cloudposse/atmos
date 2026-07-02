package atmos

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestMain is the test binary entry-point.  When the binary is re-invoked as a
// cross-platform helper subprocess (detected via ATMOS_BASH_TOOL_TEST_MODE), it
// performs the requested operation and exits immediately so the test suite never
// runs in helper mode.
//
// Supported modes (set ATMOS_BASH_TOOL_TEST_MODE before calling Execute with
// os.Executable() as the binary):
//
// "echoargs" – print argv[1:] space-joined to stdout and exit 0.
// "pwd"      – print the process working directory to stdout and exit 0.
// "exitone"  – exit with code 1 immediately (simulates a failing command).
func TestMain(m *testing.M) {
	switch os.Getenv("ATMOS_BASH_TOOL_TEST_MODE") {
	case "echoargs":
		// Mimic POSIX echo: print all arguments space-joined.
		fmt.Println(strings.Join(os.Args[1:], " "))
		os.Exit(0)
	case "pwd":
		wd, err := os.Getwd()
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		fmt.Println(wd)
		os.Exit(0)
	case "exitone":
		// Simulates a command that exits non-zero (e.g. "ls /nonexistent").
		os.Exit(1)
	}

	os.Exit(m.Run())
}

// testHelperBin returns the absolute path to the running test binary so that
// tests can re-invoke it as a cross-platform subprocess helper.
func testHelperBin(t *testing.T) string {
	t.Helper()
	exe, err := os.Executable()
	require.NoError(t, err, "os.Executable must succeed")
	return exe
}

// testHelperAllowed returns an ExecuteBashCommandTool.allowedCmds map that
// permits the current test binary to be the executed program.  This is used
// together with testHelperBin to replace platform-specific binaries (echo, pwd,
// etc.) with a single cross-platform helper.
func testHelperAllowed(t *testing.T) map[string]bool {
	t.Helper()
	return map[string]bool{filepath.Base(testHelperBin(t)): true}
}

// quoteCmdArg wraps s in single quotes, escaping any embedded single quotes so
// the result is safe to embed in a command string passed to shell.Fields.
func quoteCmdArg(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
