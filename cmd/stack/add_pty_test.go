//go:build !windows

package stack

import (
	"bufio"
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/creack/pty"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/telemetry"
)

// Pressing Enter must select the first detected source, even though the source chooser
// also offers manual path entry. Huh accessible mode uses numbered input instead.
func TestStackAddTerminalDefaultSource(t *testing.T) {
	if os.Getenv("ATMOS_STACK_ADD_PTY_CHILD") == "1" {
		previous := telemetry.PreserveCIEnvVars()
		t.Cleanup(func() { telemetry.RestoreCIEnvVars(previous) })
		viper.Set("interactive", true)
		cmd := newStackAddCommand()
		cmd.SetArgs([]string{"--name", "marker"})
		require.NoError(t, cmd.ExecuteContext(t.Context()))
		return
	}
	root := t.TempDir()
	for _, dir := range []string{"images/a", "images/b"} {
		writeAdoptionFile(t, filepath.Join(root, dir, "image.pkr.hcl"), `build { sources=["source.file.proof"] }`)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 40*time.Second)
	defer cancel()
	child := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestStackAddTerminalDefaultSource$")
	child.Dir = root
	child.Env = append(os.Environ(), "ATMOS_STACK_ADD_PTY_CHILD=1", "TERM=xterm-256color", "NO_COLOR=1", "HOME="+t.TempDir(), "ATMOS_CLI_CONFIG_PATH="+root, "ATMOS_BASE_PATH=", "ATMOS_GIT_ROOT_BASEPATH=false", "ATMOS_CI_WORKER_MODE=false")
	terminal, err := pty.StartWithSize(child, &pty.Winsize{Rows: 32, Cols: 120})
	require.NoError(t, err)
	defer terminal.Close()
	defer func() {
		cancel()
		if child.ProcessState == nil {
			_ = child.Wait()
		}
	}()
	reader := bufio.NewReader(terminal)
	for _, step := range []struct{ title, input string }{{"Which deployment would you like to register?", "\r"}, {"Atmos stack", "prod\r"}, {"Save this registration?", "\r"}} {
		var display bytes.Buffer
		for !bytes.Contains(display.Bytes(), []byte(step.title)) {
			b, err := reader.ReadByte()
			require.NoError(t, err, "waiting for %q: %s", step.title, display.String())
			display.WriteByte(b)
			switch {
			case bytes.HasSuffix(display.Bytes(), []byte("\x1b[6n")):
				_, err = terminal.WriteString("\x1b[1;1R")
			case bytes.HasSuffix(display.Bytes(), []byte("\x1b]11;?\x1b\\")):
				_, err = terminal.WriteString("\x1b]11;rgb:0000/0000/0000\x1b\\")
			}
			require.NoError(t, err)
		}
		_, err := terminal.WriteString(step.input)
		require.NoError(t, err)
	}
	require.NoError(t, child.Wait())
	manifest, err := os.ReadFile(filepath.Join(root, "stacks", "prod.yaml"))
	require.NoError(t, err)
	require.Contains(t, string(manifest), "marker:")
	require.Contains(t, string(manifest), "component: a")
}
