//go:build !windows

package cloudformation

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
	"github.com/stretchr/testify/require"
)

// Exercise Huh's terminal editor. Accessible line input treats prefilled text
// differently and cannot detect a suggested value being prepended to typed names.
func TestInitTerminalNames(t *testing.T) {
	if os.Getenv("ATMOS_CFN_INIT_PTY_CHILD") == "1" {
		enableInitInteractive(t)
		cmd := newInitCommand()
		cmd.SetArgs([]string{})
		require.NoError(t, cmd.ExecuteContext(t.Context()))
		return
	}
	project := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(project, "queue.yaml"), []byte("Resources: {}"), 0o600))
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	child := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestInitTerminalNames$")
	child.Dir = project
	child.Env = append(os.Environ(), "ATMOS_CFN_INIT_PTY_CHILD=1", "TERM=xterm-256color", "NO_COLOR=1",
		"HOME="+t.TempDir(), "ATMOS_CLI_CONFIG_PATH="+project, "ATMOS_BASE_PATH=", "ATMOS_GIT_ROOT_BASEPATH=false")
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
	for _, step := range []struct{ prompt, input string }{
		{"queue.yaml", "\r"},
		{"enter next", "orders-blue, orders-green\r"},
		{"enter submit", "prod\r"},
	} {
		var display bytes.Buffer
		for !bytes.HasSuffix(display.Bytes(), []byte(step.prompt)) {
			b, err := reader.ReadByte()
			require.NoError(t, err, "waiting for %q: %s", step.prompt, display.String())
			display.WriteByte(b)
		}
		_, err := terminal.WriteString(step.input)
		require.NoError(t, err)
	}
	require.NoError(t, child.Wait())
	data, err := os.ReadFile(filepath.Join(project, "stacks", "prod.yaml"))
	require.NoError(t, err)
	require.Contains(t, string(data), "stack_name: orders-blue\n")
	require.Contains(t, string(data), "stack_name: orders-green\n")
}
