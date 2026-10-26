//go:build !windows

package step

import (
	"bufio"
	"context"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/creack/pty"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	iolib "github.com/cloudposse/atmos/pkg/io"
)

// promptTerminal supplies actual terminal input and a race-safe output capture.
func promptTerminal(t *testing.T) (*os.File, *syncBuffer) {
	t.Helper()
	master, terminal, err := pty.Open()
	require.NoError(t, err)
	reader, output, err := os.Pipe()
	require.NoError(t, err)
	oldInput, oldOutput, oldError := os.Stdin, os.Stdout, os.Stderr
	os.Stdin, os.Stdout, os.Stderr = terminal, output, output
	iolib.Reset()
	require.NoError(t, iolib.Initialize())
	capture := &syncBuffer{}
	drained := make(chan struct{})
	go func() {
		_, _ = io.Copy(capture, reader)
		close(drained)
	}()
	t.Cleanup(func() {
		_ = output.Close()
		<-drained
		_ = reader.Close()
		_ = master.Close()
		_ = terminal.Close()
		os.Stdin, os.Stdout, os.Stderr = oldInput, oldOutput, oldError
		iolib.Reset()
		_ = iolib.Initialize()
	})
	return master, capture
}

func TestInteractiveMaskingShowsPromptBeforeReadingInput(t *testing.T) {
	for _, shell := range []bool{false, true} {
		for _, scoped := range []bool{false, true} {
			name := "callback"
			if shell {
				name = "shell"
			}
			if scoped {
				name += "-scoped"
			}
			t.Run(name, func(t *testing.T) {
				master, display := promptTerminal(t)
				writer := NewOutputModeWriter(OutputModeRaw, "prompt", nil, noLabels())
				if scoped {
					writer.writers = OutputWriters{Stdout: display, Stderr: display}
				}
				done := make(chan error, 1)
				go func() {
					var err error
					if shell {
						_, _, err = (&ShellHandler{}).runInterpreter(context.Background(), writer, shellRunSpec{
							command: "printf 'PROMPT> '; printf 'DIAGNOSTIC> ' >&2; read answer; printf 'ANSWER=%s\\n' \"$answer\"",
							stdin:   os.Stdin,
						})
					} else {
						_, _, err = writer.ExecuteWithIO(func(stdout, stderr io.Writer) error {
							_, _ = io.WriteString(stdout, "PROMPT> ")
							_, _ = io.WriteString(stderr, "DIAGNOSTIC> ")
							answer, readErr := bufio.NewReader(os.Stdin).ReadString('\n')
							_, _ = io.WriteString(stdout, "ANSWER="+answer)
							return readErr
						})
					}
					done <- err
				}()
				// Even on failure, send input so the runner exits before global streams are restored.
				assert.Eventually(t, func() bool {
					return strings.Contains(display.String(), "PROMPT> ") && strings.Contains(display.String(), "DIAGNOSTIC> ")
				}, 2*time.Second, time.Millisecond)
				_, err := io.WriteString(master, "hello\n")
				require.NoError(t, err)
				require.NoError(t, <-done)
				assert.Eventually(t, func() bool { return strings.Contains(display.String(), "ANSWER=hello") }, time.Second, time.Millisecond)
			})
		}
	}
}

func TestInteractiveMaskingStillProtectsSplitLiterals(t *testing.T) {
	_, display := promptTerminal(t)
	iolib.RegisterSecret(streamedSecret)
	writer := NewOutputModeWriter(OutputModeRaw, "secret", nil, noLabels())
	writer.writers = OutputWriters{Stdout: display, Stderr: display}
	_, _, err := writer.ExecuteWithIO(func(stdout, stderr io.Writer) error {
		for _, stream := range []io.Writer{stdout, stderr} {
			for _, b := range []byte(streamedSecret + "\n") {
				_, _ = stream.Write([]byte{b})
				assert.NotContains(t, display.String(), streamedSecret)
			}
		}
		return nil
	})
	require.NoError(t, err)
	assert.Equal(t, strings.Repeat(iolib.MaskReplacement+"\n", 2), display.String())
}

func TestInteractiveMaskingUsesActualInput(t *testing.T) {
	for _, name := range []string{"nil", "pipe"} {
		t.Run(name, func(t *testing.T) {
			var stdin io.Reader
			if name == "pipe" {
				stdin = strings.NewReader("input")
			}
			_, display := promptTerminal(t)
			require.NoError(t, iolib.RegisterPattern(`TOKEN_[0-9]+`))
			writer := NewOutputModeWriter(OutputModeRaw, "noninteractive", nil, noLabels()).WithStdin(stdin)
			writer.writers = OutputWriters{Stdout: display, Stderr: display}
			_, _, err := writer.ExecuteWithIO(func(stdout, stderr io.Writer) error {
				for _, stream := range []io.Writer{stdout, stderr} {
					_, err := io.WriteString(stream, "TOKEN_")
					require.NoError(t, err)
					assert.NotContains(t, display.String(), "TOKEN_")
					_, err = io.WriteString(stream, "123\n")
					require.NoError(t, err)
				}
				return nil
			})
			require.NoError(t, err)
			assert.Equal(t, strings.Repeat(iolib.MaskReplacement+"\n", 2), display.String())
		})
	}
}
