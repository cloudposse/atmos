//go:build !windows

package step

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/creack/pty"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	iolib "github.com/cloudposse/atmos/pkg/io"
)

func TestNestedErrorChild(t *testing.T) {
	if os.Getenv("_ATMOS_NESTED_ERROR_HELPER") != "1" {
		return
	}
	// Match the root command's binding of ATMOS_FORCE_COLOR before formatting an error.
	viper.Set("force-color", os.Getenv("ATMOS_FORCE_COLOR") == "true")
	iolib.Reset()
	if err := iolib.Initialize(); err != nil {
		os.Exit(99)
	}
	err := errUtils.Build(errors.New("nested failure")).WithHint("Correct the configuration").Err()
	_, _ = io.WriteString(os.Stderr, errUtils.Format(err, errUtils.DefaultFormatterConfig()))
	_, _ = io.WriteString(os.Stderr, "nested-test-secret\n")
	if os.Getenv("ATMOS_FORCE_TTY") != "" {
		os.Exit(98)
	}
	os.Exit(17)
}

func TestNestedShellErrorsPreserveRenderingMaskingAndExitCode(t *testing.T) {
	executable, err := os.Executable()
	require.NoError(t, err)
	for _, tc := range []struct {
		name                string
		tty, noColor, force bool
	}{
		{"terminal", true, false, false},
		{"redirected", false, false, false},
		{"opt-out", true, true, true},
		{"explicit-force", false, false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, key := range []string{"NO_COLOR", "CLICOLOR", "CLICOLOR_FORCE", "FORCE_COLOR", "ATMOS_FORCE_COLOR", "ATMOS_FORCE_TTY", "CI"} {
				t.Setenv(key, "")
			}
			t.Setenv("TERM", "xterm-256color")
			t.Setenv("COLORTERM", "truecolor")
			viper.Reset()
			viper.Set("no-color", tc.noColor)
			viper.Set("force-color", tc.force)
			iolib.Reset()
			require.NoError(t, iolib.Initialize())
			t.Cleanup(func() { viper.Reset(); iolib.Reset(); _ = iolib.Initialize() })
			iolib.RegisterSecret("nested-test-secret")
			var buffer bytes.Buffer
			var destination io.Writer = &buffer
			if tc.tty {
				master, slave, err := pty.Open()
				require.NoError(t, err)
				drained := make(chan struct{})
				go func() { _, _ = io.Copy(io.Discard, master); close(drained) }()
				t.Cleanup(func() { _ = slave.Close(); _ = master.Close(); <-drained })
				destination = slave
			}
			writer := NewOutputModeWriter(OutputModeRaw, "nested", nil, noLabels())
			writer.writers = OutputWriters{Stdout: destination, Stderr: destination}
			_, captured, err := (&ShellHandler{}).runInterpreter(context.Background(), writer, shellRunSpec{
				command: fmt.Sprintf("%q -test.run=^TestNestedErrorChild$", executable),
				env:     []string{"_ATMOS_NESTED_ERROR_HELPER=1"}, stdin: strings.NewReader(""),
			})
			require.Error(t, err)
			require.Equal(t, 17, getExitCode(err))
			require.Equal(t, (tc.tty || tc.force) && !tc.noColor, strings.Contains(captured, "\x1b["))
			require.NotContains(t, captured, "# Error")
			require.NotContains(t, captured, "**Error:**")
			require.NotContains(t, captured, "nested-test-secret")
			require.Contains(t, captured, iolib.MaskReplacement)
			require.Equal(t, 1, strings.Count(captured, "💡"))
		})
	}
}
