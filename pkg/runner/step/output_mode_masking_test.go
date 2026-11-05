package step

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	iolib "github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/pkg/schema"
)

const (
	streamedSecret = "s3cr3t-streamed-token"
	streamedPEM    = "-----BEGIN TEST CERTIFICATE-----\nATMOS-TEST-PEM-LINE-ONE-AAAAAAAAAAAAAAAA\nATMOS-TEST-PEM-LINE-TWO-BBBBBBBBBBBBBBBB\n-----END TEST CERTIFICATE-----\n"
)

// noLabels hides the step header and footer so assertions see only command output.
func noLabels() *schema.ShowConfig {
	return &schema.ShowConfig{Labels: BoolPtr(false)}
}

// writeSplit writes "pre <secret> post\n" to w in two writes split at offset k.
func writeSplit(w io.Writer, k int) {
	line := "pre " + streamedSecret + " post\n"
	_, _ = io.WriteString(w, line[:k])
	_, _ = io.WriteString(w, line[k:])
}

func splitOffsets() []int {
	offsets := make([]int, 0, len(streamedSecret)+len(" post\n")+len("pre ")+1)
	for k := 0; k <= len("pre "+streamedSecret+" post\n"); k++ {
		offsets = append(offsets, k)
	}
	return offsets
}

// TestOutputModes_SecretSplitAcrossWritesIsNeverDisplayedUnmasked proves that a single-line secret
// split at every byte offset across two writes is masked by each output mode on both streams.
func TestOutputModes_SecretSplitAcrossWritesIsNeverDisplayedUnmasked(t *testing.T) {
	modes := []OutputMode{OutputModeRaw, OutputModeLog, OutputModeViewport}
	for _, mode := range modes {
		t.Run(string(mode), func(t *testing.T) {
			out, errOut, cleanup := setupOutputModeCapture(t)
			defer cleanup()
			iolib.RegisterSecret(streamedSecret)

			// Viewport only renders live (and shows full logs on failure) with a TTY; otherwise it
			// streams like raw mode. Force the TTY path and fail the runner so the captured logs are shown.
			viper.Set("force-tty", mode == OutputModeViewport)
			t.Cleanup(func() { viper.Set("force-tty", false) })

			w := NewOutputModeWriter(mode, "streamed", &schema.ViewportConfig{Height: 4})
			offsets := splitOffsets()
			var captured []string
			for _, k := range offsets {
				stdout, stderr, _ := w.ExecuteWithIO(func(stdout, stderr io.Writer) error {
					writeSplit(stdout, k)
					writeSplit(stderr, k)
					if mode == OutputModeViewport {
						return context.Canceled
					}
					return nil
				})
				captured = append(captured, stdout, stderr)
			}
			cleanup()

			for _, display := range []string{out.String(), errOut.String()} {
				assert.NotContains(t, display, streamedSecret, "displayed output leaked the secret")
				rendered := strings.Count(display, "pre "+iolib.MaskReplacement+" post\n")
				if mode == OutputModeViewport {
					// The live window also renders the tail, so only require the masked line to appear.
					assert.Positive(t, rendered)
					continue
				}
				assert.Equal(t, len(offsets), rendered, "every split must render the masked line exactly once")
			}

			// Captured values keep today's semantics: raw and log capture raw bytes (only the displayed
			// streams are masked), viewport captures through the masker.
			want := "pre " + streamedSecret + " post\n"
			if mode == OutputModeViewport {
				want = "pre " + iolib.MaskReplacement + " post\n"
			}
			for _, c := range captured {
				assert.Equal(t, want, c)
			}
		})
	}
}

// TestOutputModes_SecretSplitAcrossManyBatchesIsNotLeaked drives the secret one byte at a time.
func TestOutputModes_SecretSplitByteAtATimeIsNotLeaked(t *testing.T) {
	for _, mode := range []OutputMode{OutputModeRaw, OutputModeLog} {
		t.Run(string(mode), func(t *testing.T) {
			out, errOut, cleanup := setupOutputModeCapture(t)
			defer cleanup()
			iolib.RegisterSecret(streamedSecret)

			w := NewOutputModeWriter(mode, "streamed", nil, noLabels())
			_, _, err := w.ExecuteWithIO(func(stdout, stderr io.Writer) error {
				for _, b := range []byte("pre " + streamedSecret + " post\n") {
					_, _ = stdout.Write([]byte{b})
					_, _ = stderr.Write([]byte{b})
				}
				return nil
			})
			require.NoError(t, err)
			cleanup()

			assert.Equal(t, "pre "+iolib.MaskReplacement+" post\n", out.String())
			assert.Equal(t, "pre "+iolib.MaskReplacement+" post\n", errOut.String())
		})
	}
}

// TestOutputModes_MultilineSecretSpanningBatchesRendersLikeUnsplit proves a PEM delivered in
// line batches collapses into one mask in raw and log mode, exactly like the unsplit text.
func TestOutputModes_MultilineSecretSpanningBatchesRendersLikeUnsplit(t *testing.T) {
	for _, mode := range []OutputMode{OutputModeRaw, OutputModeLog} {
		t.Run(string(mode), func(t *testing.T) {
			out, _, cleanup := setupOutputModeCapture(t)
			defer cleanup()
			iolib.RegisterSecret(streamedPEM)
			want := iolib.GetContext().Masker().Mask("header\n" + streamedPEM + "footer\n")
			require.Equal(t, 1, strings.Count(want, iolib.MaskReplacement))

			w := NewOutputModeWriter(mode, "pem", nil, noLabels())
			_, _, err := w.ExecuteWithIO(func(stdout, _ io.Writer) error {
				_, _ = io.WriteString(stdout, "header\n")
				for _, line := range strings.SplitAfter(streamedPEM, "\n") {
					_, _ = io.WriteString(stdout, line)
				}
				_, _ = io.WriteString(stdout, "footer\n")
				return nil
			})
			require.NoError(t, err)
			cleanup()

			assert.Equal(t, want, out.String())
		})
	}
}

// TestShellHandler_SecretSplitAcrossBuiltinWritesIsMasked covers the in-process shell interpreter path.
func TestShellHandler_SecretSplitAcrossBuiltinWritesIsMasked(t *testing.T) {
	for _, mode := range []OutputMode{OutputModeRaw, OutputModeLog, OutputModeNone} {
		t.Run(string(mode), func(t *testing.T) {
			out, _, cleanup := setupOutputModeCapture(t)
			defer cleanup()
			iolib.RegisterSecret(streamedSecret)

			handler := &ShellHandler{BaseHandler: NewBaseHandler("shell", CategoryCommand, false)}
			writer := NewOutputModeWriter(mode, "shell", nil, noLabels())
			command := fmt.Sprintf("printf 'pre %s'; printf '%s post\\n'", streamedSecret[:7], streamedSecret[7:])
			stdout, _, err := handler.runInterpreter(context.Background(), writer, shellRunSpec{
				stepName: "shell",
				command:  command,
				env:      os.Environ(),
			})
			require.NoError(t, err)
			cleanup()

			// The step value is captured through the masker in this path.
			assert.Equal(t, "pre "+iolib.MaskReplacement+" post\n", stdout)
			assert.NotContains(t, out.String(), streamedSecret)
			if mode != OutputModeNone {
				assert.Equal(t, "pre "+iolib.MaskReplacement+" post\n", out.String())
			}
		})
	}
}
