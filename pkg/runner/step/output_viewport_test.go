package step

import (
	"context"
	"fmt"
	"io"
	"strings"
	"sync"
	"testing"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	iolib "github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/signals"
)

func TestOutputViewportTailAndResize(t *testing.T) {
	tail := &viewportTail{}
	_, _ = io.WriteString(tail, "old\nfirst\nsecond\n\x1b[31m"+strings.Repeat("界", 50)+"\x1b[0m\n")
	m := &outputViewportModel{tail: tail, title: "Build", width: 80, height: 24, maxHeight: 4, spinner: spinner.New()}
	view := m.View()
	assert.NotContains(t, view, "old")
	assert.Len(t, strings.Split(view, "\n"), 4)
	m.Update(tea.WindowSizeMsg{Width: 12, Height: 3})
	view = m.View()
	assert.NotContains(t, view, "first")
	assert.Len(t, strings.Split(view, "\n"), 3)
	for _, line := range strings.Split(view, "\n") {
		assert.LessOrEqual(t, ansi.StringWidth(line), 11)
	}
	m.Update(viewportFinishedMsg{})
	assert.Empty(t, m.View(), "completion clears the live window")
}

func TestOutputViewportCapturesBothStreamsAndFailureOnce(t *testing.T) {
	for _, failed := range []bool{false, true} {
		t.Run(fmt.Sprint(failed), func(t *testing.T) {
			viper.Set("force-tty", true)
			t.Cleanup(func() { viper.Set("force-tty", false) })
			out, errOut, cleanup := setupOutputModeCapture(t)
			defer cleanup()
			w := NewOutputModeWriter(OutputModeViewport, "build", &schema.ViewportConfig{Height: 4})
			stdout, stderr, err := w.ExecuteWithIO(func(stdout, stderr io.Writer) error {
				var wg sync.WaitGroup
				for _, writer := range []io.Writer{stdout, stderr} {
					wg.Go(func() {
						for i := range 100 {
							_, _ = fmt.Fprintf(writer, "case-%d\n", i)
						}
					})
				}
				wg.Wait()
				if failed {
					return context.Canceled
				}
				return nil
			})
			if failed {
				require.ErrorIs(t, err, context.Canceled)
			} else {
				require.NoError(t, err)
			}
			assert.Contains(t, stdout, "case-0\n")
			assert.Contains(t, stderr, "case-99\n")
			cleanup()
			if failed {
				assert.Equal(t, 1, strings.Count(out.String(), "case-0\n"))
				assert.Equal(t, 1, strings.Count(errOut.String(), "case-0\n"))
			} else {
				assert.Empty(t, out.String())
				assert.NotContains(t, errOut.String(), "case-0\n")
				assert.Contains(t, errOut.String(), "build completed")
			}
		})
	}
}

func TestOutputViewportExitCleanup(t *testing.T) {
	viper.Set("force-tty", true)
	t.Cleanup(func() { viper.Set("force-tty", false) })
	_, _, cleanup := setupOutputModeCapture(t)
	defer cleanup()
	w := NewOutputModeWriter(OutputModeViewport, "build", nil)
	_, _, err := w.ExecuteWithIO(func(stdout, stderr io.Writer) error {
		signals.RunExitCleanups()
		return context.Canceled
	})
	require.ErrorIs(t, err, context.Canceled)
}

func TestOutputViewportMasksLiveTextAndResults(t *testing.T) {
	viper.Set("force-tty", true)
	t.Cleanup(func() { viper.Set("force-tty", false) })
	out, errOut, cleanup := setupOutputModeCapture(t)
	defer cleanup()
	iolib.ApplyMaskingConfig(&iolib.Config{DisableMasking: false})
	secret := "viewport-secret-21d85af6"
	iolib.GetContext().Masker().RegisterValue(secret)
	maskedSecret := iolib.MaskString(secret)
	w := NewOutputModeWriter(OutputModeViewport, "build", nil)
	stdout, stderr, err := w.ExecuteWithIO(func(stdout, stderr io.Writer) error {
		_, _ = io.WriteString(stdout, secret)
		_, _ = io.WriteString(stderr, secret)
		return context.Canceled
	})
	require.ErrorIs(t, err, context.Canceled)
	cleanup()
	for _, value := range []string{stdout, stderr, out.String(), errOut.String()} {
		assert.NotContains(t, value, secret)
		assert.Contains(t, value, maskedSecret)
	}
}

func TestOutputViewportPadding(t *testing.T) {
	tail := &viewportTail{}
	_, _ = io.WriteString(tail, "hello\n")
	m := &outputViewportModel{tail: tail, title: "Compiling", width: 40, height: 4, padding: 2, spinner: spinner.New()}
	for _, line := range strings.Split(ansi.Strip(m.View()), "\n") {
		line = strings.TrimPrefix(line, "\r")
		assert.True(t, strings.HasPrefix(line, "  "), line)
		assert.True(t, strings.HasSuffix(line, "  "), line)
	}
	m.Update(tea.WindowSizeMsg{Width: 2, Height: 1})
	assert.LessOrEqual(t, ansi.StringWidth(m.View()), 1)
}
