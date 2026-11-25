//go:build !windows

package ui

import (
	"bufio"
	"context"
	"io"
	"os"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stopOnInterruptOptions runs the test binary as a terraform that stops gracefully on interrupt.
func stopOnInterruptOptions(t *testing.T, args []string) *ExecuteOptions {
	t.Helper()
	exePath, err := os.Executable()
	require.NoError(t, err)
	return &ExecuteOptions{
		Command:    exePath,
		Args:       args,
		WorkingDir: t.TempDir(),
		Env:        append(os.Environ(), "_ATMOS_TEST_STOP_ON_INTERRUPT=1"),
	}
}

// TestStreamingCommandFirstCtrlCInterruptsTerraform verifies the first Ctrl-C keypress in the
// TUI delivers an interrupt to terraform instead of killing it, so terraform can save state.
func TestStreamingCommandFirstCtrlCInterruptsTerraform(t *testing.T) {
	opts := stopOnInterruptOptions(t, nil)
	run, stdout, err := newStreamingCommand(context.Background(), opts, []string{"-test.run=^$"})
	require.NoError(t, err)
	t.Cleanup(run.kill)
	output := awaitReady(t, stdout)

	m := gracefulModel{inner: NewModel("comp", "stack", "apply", strings.NewReader("")), run: run}
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	assert.Nil(t, cmd, "the TUI keeps running while terraform stops")
	stopping := updated.(gracefulModel)
	assert.True(t, stopping.stopping)
	assert.Equal(t, os.Interrupt, stopping.signal, "a keypress stands for an interrupt")
	assert.Contains(t, stopping.View(), "Press Ctrl-C again to force exit.")
	assert.False(t, modelWasCancelled(stopping.inner))

	rest, readErr := io.ReadAll(output)
	require.NoError(t, readErr)
	assert.Equal(t, "interrupt", strings.TrimSpace(string(rest)), "terraform receives exactly the interrupt")
	require.NoError(t, run.wait(), "terraform stopped on its own")
}

// TestInitCommandInterruptStopsTerraform verifies init runs under the same graceful stop.
func TestInitCommandInterruptStopsTerraform(t *testing.T) {
	res, err := newInitCommand(context.Background(), stopOnInterruptOptions(t, []string{"-test.run=^$"}))
	require.NoError(t, err)
	t.Cleanup(res.run.kill)
	output := awaitReady(t, res.reader)

	require.NoError(t, res.run.interrupt())
	rest, readErr := io.ReadAll(output)
	require.NoError(t, readErr)
	<-res.done
	require.NoError(t, *res.err)
	assert.Equal(t, "interrupt", strings.TrimSpace(string(rest)))
}

// awaitReady reads the helper's "ready" line and returns the rest of its output.
func awaitReady(t *testing.T, r io.Reader) io.Reader {
	t.Helper()
	reader := bufio.NewReader(r)
	line, err := reader.ReadString('\n')
	require.NoError(t, err)
	require.Equal(t, "ready", strings.TrimSpace(line), "prerequisite: the helper handles signals")
	return reader
}
