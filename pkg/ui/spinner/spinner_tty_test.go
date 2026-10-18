package spinner

import (
	"errors"
	"io"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ansi"
	iolib "github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/pkg/ui"
)

// ttyWaitTimeout bounds how long a test waits for the spinner's goroutine.
const ttyWaitTimeout = 10 * time.Second

// lockedBuffer collects the spinner's output; the renderer writes from its own goroutine.
type lockedBuffer struct {
	mu  sync.Mutex
	buf strings.Builder
}

// Write appends p to the buffer.
func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

// String returns everything written so far.
func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// useInteractiveSpinner makes the spinners take their terminal path without a real terminal and returns
// what they print. Terminal output is forced on (as for screenshot recordings), the output goes to a
// buffer, and stdin is a pipe so bubbletea never tries to open the developer's real terminal for input.
func useInteractiveSpinner(t *testing.T) *lockedBuffer {
	t.Helper()

	viper.Set("force-tty", true)
	t.Cleanup(func() { viper.Set("force-tty", false) })

	ioCtx, err := iolib.NewContext()
	require.NoError(t, err)
	ui.InitFormatter(ioCtx)
	t.Cleanup(ui.Reset)

	out := &lockedBuffer{}
	origUI := iolib.UI
	iolib.UI = out
	t.Cleanup(func() { iolib.UI = origUI })

	r, w, err := os.Pipe()
	require.NoError(t, err)
	origStdin := os.Stdin
	os.Stdin = r
	t.Cleanup(func() {
		os.Stdin = origStdin
		_ = r.Close()
		_ = w.Close()
	})

	return out
}

// TestSpinner_Start_CtrlCCallsTheInterruptHandler checks that ctrl+c in the running spinner reaches the handler and Stop does not block afterwards.
func TestSpinner_Start_CtrlCCallsTheInterruptHandler(t *testing.T) {
	useInteractiveSpinner(t)
	interrupted := make(chan struct{})
	s := New("Working")
	require.True(t, s.isTTY)
	s.SetInterruptHandler(func() { close(interrupted) })

	s.Start()
	require.NotNil(t, s.program)
	s.program.Send(tea.KeyMsg{Type: tea.KeyCtrlC})

	select {
	case <-interrupted:
	case <-time.After(ttyWaitTimeout):
		require.Fail(t, "the interrupt handler was not called")
	}

	// The owner still stops the spinner afterwards; that must not block.
	stopped := make(chan struct{})
	go func() {
		defer close(stopped)
		s.Stop()
	}()
	select {
	case <-stopped:
	case <-time.After(ttyWaitTimeout):
		require.Fail(t, "Stop blocked after an interrupt")
	}
	assert.Nil(t, s.program)
}

// TestSpinner_Start_CompletionDoesNotCallTheInterruptHandler checks that finishing the spinner normally is not an interrupt.
func TestSpinner_Start_CompletionDoesNotCallTheInterruptHandler(t *testing.T) {
	tests := []struct {
		name       string
		finish     func(s *Spinner)
		wantOutput string
	}{
		{name: "stop", finish: func(s *Spinner) { s.Stop() }},
		{name: "success", finish: func(s *Spinner) { s.Success("Finished building") }, wantOutput: "Finished building"},
		{name: "error", finish: func(s *Spinner) { s.Error("Build failed") }, wantOutput: "Build failed"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out := useInteractiveSpinner(t)
			calls := 0
			s := New("Working")
			s.SetInterruptHandler(func() { calls++ })

			s.Start()
			s.Update("Compiling")
			s.Update("") // An empty update is ignored.
			tt.finish(s)

			assert.Nil(t, s.program, "the spinner is finished")
			assert.Zero(t, calls, "only ctrl+c is an interrupt")
			if tt.wantOutput != "" {
				assert.Contains(t, ansi.Strip(out.String()), tt.wantOutput)
			}

			// Finishing again is harmless.
			assert.NotPanics(t, func() { s.Stop() })
		})
	}
}

// TestSpinner_Update_BeforeStartIsIgnored checks that updating a spinner that was never started is a no-op.
func TestSpinner_Update_BeforeStartIsIgnored(t *testing.T) {
	useInteractiveSpinner(t)
	s := New("Working")
	require.True(t, s.isTTY)

	assert.NotPanics(t, func() { s.Update("Compiling") })
	assert.Nil(t, s.program)
}

// TestExecWithSpinner_Interactive covers ExecWithSpinner on its terminal path.
func TestExecWithSpinner_Interactive(t *testing.T) {
	t.Run("success shows the completed message", func(t *testing.T) {
		out := useInteractiveSpinner(t)
		ran := false

		err := ExecWithSpinner("Installing", "Installed tool", func() error {
			ran = true
			return nil
		})

		require.NoError(t, err)
		assert.True(t, ran)
		assert.Contains(t, ansi.Strip(out.String()), "Installed tool")
	})

	t.Run("an operation failure is returned and the progress message is shown as the failure", func(t *testing.T) {
		out := useInteractiveSpinner(t)
		boom := errors.New("download failed")

		err := ExecWithSpinner("Installing", "Installed tool", func() error { return boom })

		require.ErrorIs(t, err, boom)
		assert.NotErrorIs(t, err, errUtils.ErrSpinnerOperationInterrupted)
		shown := ansi.Strip(out.String())
		assert.Contains(t, shown, "Installing")
		assert.NotContains(t, shown, "Installed tool")
	})
}

// TestExecWithSpinnerDynamic_Interactive covers ExecWithSpinnerDynamic on its terminal path.
func TestExecWithSpinnerDynamic_Interactive(t *testing.T) {
	tests := []struct {
		name      string
		operation func() (string, error)
		wantErr   error
		wantShown string
		notShown  string
	}{
		{
			name:      "the operation chooses the completed message",
			operation: func() (string, error) { return "Compared main...feature", nil },
			wantShown: "Compared main...feature",
		},
		{
			name:      "an empty completed message falls back to the progress message",
			operation: func() (string, error) { return "", nil },
			wantShown: "Comparing refs",
		},
		{
			name:      "an operation failure is returned",
			operation: func() (string, error) { return "never shown", io.ErrUnexpectedEOF },
			wantErr:   io.ErrUnexpectedEOF,
			wantShown: "Comparing refs",
			notShown:  "never shown",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out := useInteractiveSpinner(t)

			err := ExecWithSpinnerDynamic("Comparing refs", tt.operation)

			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
			} else {
				require.NoError(t, err)
			}
			shown := ansi.Strip(out.String())
			assert.Contains(t, shown, tt.wantShown)
			if tt.notShown != "" {
				assert.NotContains(t, shown, tt.notShown)
			}
		})
	}
}

// captureUIOutput runs fn with the UI formatter initialized and returns what it wrote to stderr.
func captureUIOutput(t *testing.T, fn func()) string {
	t.Helper()

	ioCtx, err := iolib.NewContext()
	require.NoError(t, err)
	ui.InitFormatter(ioCtx)
	t.Cleanup(ui.Reset)

	r, w, err := os.Pipe()
	require.NoError(t, err)
	origStderr := os.Stderr
	os.Stderr = w
	defer func() { os.Stderr = origStderr }()

	fn()
	require.NoError(t, w.Close())

	var buf strings.Builder
	_, err = io.Copy(&buf, r)
	require.NoError(t, err)
	require.NoError(t, r.Close())

	return ansi.Strip(buf.String())
}

// TestSpinner_MessagesWhenNothingIsAnimating covers a spinner that is not showing (no terminal, or never
// started): its messages are not lost, they are printed as status lines.
func TestSpinner_MessagesWhenNothingIsAnimating(t *testing.T) {
	tests := []struct {
		name  string
		isTTY bool
		act   func(s *Spinner)
		want  string
	}{
		{name: "update without a terminal prints a status line", isTTY: false, act: func(s *Spinner) { s.Update("Compiling") }, want: "Compiling"},
		{name: "success without a terminal prints the message", isTTY: false, act: func(s *Spinner) { s.Success("All done") }, want: "All done"},
		{name: "error without a terminal prints the message", isTTY: false, act: func(s *Spinner) { s.Error("It broke") }, want: "It broke"},
		{name: "success on a spinner that never started prints the message", isTTY: true, act: func(s *Spinner) { s.Success("All done") }, want: "All done"},
		{name: "error on a spinner that never started prints the message", isTTY: true, act: func(s *Spinner) { s.Error("It broke") }, want: "It broke"},
		{name: "update on a spinner that never started prints nothing", isTTY: true, act: func(s *Spinner) { s.Update("Compiling") }, want: ""},
		{name: "an empty update prints nothing", isTTY: false, act: func(s *Spinner) { s.Update("") }, want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &Spinner{progressMsg: "Working", isTTY: tt.isTTY}

			output := captureUIOutput(t, func() { tt.act(s) })

			if tt.want == "" {
				assert.Empty(t, strings.TrimSpace(output))
				return
			}
			assert.Contains(t, output, tt.want)
			assert.Nil(t, s.program)
		})
	}
}

// TestClipToWidth covers the live line truncation, including widths too small to truncate against.
func TestClipToWidth(t *testing.T) {
	const line = "Downloading artifact number one of many"

	tests := []struct {
		name  string
		width int
		want  string
	}{
		{name: "unknown width leaves the line alone", width: 0, want: line},
		{name: "negative width leaves the line alone", width: -3, want: line},
		{name: "width of one leaves no room for content and leaves the line alone", width: 1, want: line},
		{name: "a line that fits is unchanged", width: len(line) + 5, want: line},
		{name: "a long line is cut with an ellipsis within the margin", width: 10, want: "Download…"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := clipToWidth(line, tt.width)

			assert.Equal(t, tt.want, got)
			if tt.width > liveLineMargin {
				assert.LessOrEqual(t, len([]rune(got)), tt.width-liveLineMargin, "never reaches the last column")
			}
		})
	}
}
