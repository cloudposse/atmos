package exec

import (
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestExecuteAtmosCmd_TUIStartFailureIsReturned covers the line in
// ExecuteAtmosCmd right after `tui.Execute(...)` (`ui.Writeln("")`), which
// unconditionally runs whether or not the TUI started successfully.
//
// In a headless test process there is no controlling TTY, so Bubble Tea's
// Program.Run() cannot open one (see charmbracelet/bubbletea's
// openInputTTY, which falls back to opening /dev/tty and fails fast when
// none is available) and returns an error immediately instead of blocking on
// input. That lets this test reach the `ui.Writeln("")` line and the
// subsequent `if err != nil { return err }` branch without needing a real
// interactive session.
//
// This fails-fast behavior is Unix-specific: on Windows, Bubble Tea reads
// input via syscall.ReadConsole against the process's console handle, which
// blocks waiting for real input instead of erroring when there's no
// interactive session, hanging this test (and the whole package's test
// binary) until the CI job's 40-minute timeout kills it.
func TestExecuteAtmosCmd_TUIStartFailureIsReturned(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Skipping on Windows: Bubble Tea's console input read blocks instead of failing fast without a TTY")
	}

	workDir := "../../tests/fixtures/scenarios/packer"
	t.Chdir(workDir)
	// Isolate from the repo's own atmos.yaml and disable parent/git-root
	// discovery.
	t.Setenv("ATMOS_CLI_CONFIG_PATH", ".")

	err := ExecuteAtmosCmd()

	require.Error(t, err)
}
