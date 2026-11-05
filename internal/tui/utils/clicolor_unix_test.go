//go:build !windows

package utils

import (
	"bytes"
	"os"
	"os/exec"
	"testing"

	"github.com/creack/pty"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	iolib "github.com/cloudposse/atmos/pkg/io"
)

// TestStyledTextRespectsCLICOLOR verifies both logo output paths honor automatic color,
// explicit forcing, and global opt-outs on a color-capable terminal.
func TestStyledTextRespectsCLICOLOR(t *testing.T) {
	// supportscolor.Stdout caches its first result for the lifetime of the process.
	// Other tests may initialize it before this test installs a color-capable PTY.
	const childEnv = "ATMOS_TEST_STYLED_TEXT_CHILD"
	if os.Getenv(childEnv) != "1" {
		executable, err := os.Executable()
		require.NoError(t, err)
		t.Setenv(childEnv, "1")
		cmd := exec.Command(executable, "-test.run=^TestStyledTextRespectsCLICOLOR$", "-test.v")
		output, err := cmd.CombinedOutput()
		require.NoError(t, err, "%s", output)
		return
	}
	testStyledTextOnTerminal(t)
}

func testStyledTextOnTerminal(t *testing.T) {
	t.Helper()
	master, slave, err := pty.Open()
	require.NoError(t, err)
	original := os.Stdout
	os.Stdout = slave
	t.Cleanup(func() { os.Stdout = original; _ = slave.Close(); _ = master.Close() })
	for _, key := range []string{"NO_COLOR", "ATMOS_NO_COLOR", "ATMOS_FORCE_COLOR", "CLICOLOR", "CLICOLOR_FORCE", "FORCE_COLOR", "CI"} {
		// Register restoration, then remove the variable: supportscolor treats
		// empty FORCE_COLOR as forced color and empty NO_COLOR/CI as present.
		t.Setenv(key, "")
		require.NoError(t, os.Unsetenv(key))
	}
	t.Setenv("TERM", "xterm-256color")
	for _, tt := range []struct {
		name, cliColor, noColor, force string
		wantColor                      bool
	}{
		{"terminal color prerequisite", "1", "", "", true},
		{"zero disables terminal logo", "0", "", "", false},
		{"NO_COLOR beats CLICOLOR", "1", "1", "", false},
		{"force overrides zero", "0", "", "1", true},
		{"NO_COLOR beats both", "1", "1", "1", false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("CLICOLOR", tt.cliColor)
			t.Setenv("NO_COLOR", tt.noColor)
			if tt.noColor == "" {
				require.NoError(t, os.Unsetenv("NO_COLOR"))
			}
			t.Setenv("CLICOLOR_FORCE", tt.force)
			var output bytes.Buffer
			require.NoError(t, PrintStyledTextToSpecifiedOutput(&output, "A"))
			assert.Equal(t, tt.wantColor, bytes.Contains(output.Bytes(), []byte("\x1b")))
			output.Reset()
			originalData := iolib.Data
			iolib.Data = &output
			t.Cleanup(func() { iolib.Data = originalData })
			require.NoError(t, PrintStyledText("A"))
			assert.Equal(t, tt.wantColor, bytes.Contains(output.Bytes(), []byte("\x1b")))
		})
	}
}
