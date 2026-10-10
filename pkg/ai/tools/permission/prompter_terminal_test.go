package permission

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/x/ansi"
	"github.com/creack/pty"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
)

// permissionTerminal supplies real terminal input to the accessible prompt. The
// deadline bounds a regression that stops consuming the supplied response.
func permissionTerminal(t *testing.T, response string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("PTYs are unavailable on Windows")
	}
	master, slave, err := pty.Open()
	require.NoError(t, err)
	t.Cleanup(func() { _ = master.Close() })
	t.Cleanup(func() { _ = slave.Close() })
	outputPath := filepath.Join(t.TempDir(), "prompt.txt")
	output, err := os.Create(outputPath)
	require.NoError(t, err)
	t.Cleanup(func() { _ = output.Close() })
	stdin, stdout, stderr := os.Stdin, os.Stdout, os.Stderr
	os.Stdin, os.Stdout, os.Stderr = slave, output, output
	t.Cleanup(func() { os.Stdin, os.Stdout, os.Stderr = stdin, stdout, stderr })
	t.Setenv("TERM", "dumb")
	timer := time.AfterFunc(5*time.Second, func() { _ = master.Close() })
	t.Cleanup(func() { timer.Stop() })
	_, err = master.WriteString(response + "\n")
	require.NoError(t, err)
	return outputPath
}

func TestCLIPrompterTerminalCachedChoices(t *testing.T) {
	for _, tc := range []struct {
		name     string
		response string
		allowed  bool
		saved    bool
	}{
		{name: "allow once", response: "1", allowed: true},
		{name: "always allow", response: "2", allowed: true, saved: true},
		{name: "deny once", response: "3"},
		{name: "always deny", response: "4", saved: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			outputPath := permissionTerminal(t, tc.response)
			base := t.TempDir()
			cache, err := NewPermissionCache(base)
			require.NoError(t, err)
			tool := scopedFakeTool{name: "Bash", key: "Bash(atmos list stacks)"}
			allowed, err := NewCLIPrompterWithCache(cache).Prompt(context.Background(), tool,
				map[string]interface{}{"command": "atmos list stacks"})
			require.NoError(t, err)
			assert.Equal(t, tc.allowed, allowed)

			// Reload from disk so a transient in-memory decision cannot pass.
			reloaded, err := NewPermissionCache(base)
			require.NoError(t, err)
			prompter := NewCLIPrompterWithCache(reloaded)
			decision, found := prompter.checkCachedForTool(tool)
			assert.Equal(t, tc.saved, found)
			if tc.saved {
				assert.Equal(t, tc.allowed, decision)
			}
			assert.False(t, prompter.HasCachedDecision(scopedFakeTool{name: "Bash", key: "Bash(atmos version)"}))
			output, err := os.ReadFile(outputPath)
			require.NoError(t, err)
			// CI enables color even for redirected output; assert the visible text.
			text := ansi.Strip(string(output))
			assert.Contains(t, text, "Allow execution?")
			assert.Contains(t, text, "atmos list stacks")
			assert.Equal(t, tc.saved, strings.Contains(text, "(saved to "), "receipt: %q", text)
		})
	}
}

func TestCLIPrompterTerminalWithoutCache(t *testing.T) {
	for _, tc := range []struct {
		response string
		allowed  bool
	}{
		{response: "y", allowed: true},
		{response: "n"},
	} {
		t.Run(tc.response, func(t *testing.T) {
			outputPath := permissionTerminal(t, tc.response)
			allowed, err := NewCLIPrompter().Prompt(context.Background(), plainFakeTool{name: "read_file"},
				map[string]interface{}{"path": "README.md"})
			require.NoError(t, err)
			assert.Equal(t, tc.allowed, allowed)
			output, err := os.ReadFile(outputPath)
			require.NoError(t, err)
			text := ansi.Strip(string(output))
			assert.Contains(t, text, "Allow execution?")
			assert.NotContains(t, text, "(saved to ")
		})
	}
}

func TestRunRequestFormCancellation(t *testing.T) {
	t.Setenv("TERM", "xterm")
	form := huh.NewForm(huh.NewGroup(huh.NewConfirm().Title("Allow execution?"))).
		WithTimeout(time.Second).
		WithProgramOptions(tea.WithoutRenderer(), tea.WithoutSignalHandler()).
		WithInput(strings.NewReader("\x03")).
		WithOutput(io.Discard)
	require.ErrorIs(t, runRequestForm(form), errUtils.ErrUserAborted)
}

func TestRunRequestFormTimeout(t *testing.T) {
	t.Setenv("TERM", "xterm")
	form := huh.NewForm(huh.NewGroup(huh.NewConfirm().Title("Allow execution?"))).
		WithTimeout(time.Millisecond).
		WithProgramOptions(tea.WithoutRenderer(), tea.WithoutSignalHandler()).
		WithInput(strings.NewReader("")).
		WithOutput(io.Discard)
	err := runRequestForm(form)
	require.ErrorIs(t, err, errUtils.ErrAIPermissionPromptFailed)
	require.ErrorIs(t, err, huh.ErrTimeout)
}
