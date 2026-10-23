//go:build !windows

package step

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/creack/pty"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	iolib "github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/pkg/manifest"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/terminal"
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
	_, _ = fmt.Fprintf(os.Stderr, "nested-color=%t nested-ci=%s\n", terminal.New().ColorProfile() != terminal.ColorNone, os.Getenv("CI"))
	if os.Getenv("ATMOS_FORCE_TTY") != "" {
		os.Exit(98)
	}
	os.Exit(17)
}

func TestNestedManifestChild(t *testing.T) {
	if os.Getenv("_ATMOS_NESTED_MANIFEST_HELPER") != "1" {
		return
	}
	viper.Set("force-color", os.Getenv("ATMOS_FORCE_COLOR") == "true")
	iolib.Reset()
	require.NoError(t, iolib.Initialize())
	objects, err := manifest.DecodeObjects([]byte("apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: hook\n  annotations:\n    helm.sh/hook: pre-install,pre-upgrade\n"))
	require.NoError(t, err)
	config := &schema.AtmosConfiguration{}
	config.Settings.Terminal.SyntaxHighlighting.Enabled = true
	config.Settings.Terminal.ForceColor = viper.GetBool("force-color")
	require.NoError(t, manifest.WriteObjects(objects, manifest.RenderOptions{AtmosConfig: config}))
	os.Exit(0)
}

func TestNestedShellCIRedirectionPreservesManifestOutput(t *testing.T) {
	executable, err := os.Executable()
	require.NoError(t, err)
	for _, force := range []bool{false, true} {
		t.Run(fmt.Sprintf("force=%t", force), func(t *testing.T) {
			for _, key := range []string{"NO_COLOR", "CLICOLOR", "CLICOLOR_FORCE", "FORCE_COLOR", "ATMOS_FORCE_COLOR", "ATMOS_FORCE_TTY"} {
				t.Setenv(key, "")
			}
			t.Setenv("CI", "true")
			viper.Reset()
			viper.Set("force-color", force)
			iolib.Reset()
			require.NoError(t, iolib.Initialize())
			t.Cleanup(func() { viper.Reset(); iolib.Reset(); _ = iolib.Initialize() })
			path := filepath.Join(t.TempDir(), "manifest.yaml")
			var destination bytes.Buffer
			writer := NewOutputModeWriter(OutputModeRaw, "manifest", nil, noLabels())
			writer.writers = OutputWriters{Stdout: &destination, Stderr: &destination}
			_, _, err := (&ShellHandler{}).runInterpreter(context.Background(), writer, shellRunSpec{
				command: fmt.Sprintf("%q -test.run=^TestNestedManifestChild$ > %q", executable, path),
				env:     append(os.Environ(), "_ATMOS_NESTED_MANIFEST_HELPER=1"), stdin: strings.NewReader(""),
			})
			require.NoError(t, err)
			output, err := os.ReadFile(path)
			require.NoError(t, err)
			require.Equal(t, force, bytes.Contains(output, []byte("\x1b[")))
			if !force {
				require.Contains(t, string(output), "helm.sh/hook: pre-install,pre-upgrade")
				objects, err := manifest.DecodeObjects(output)
				require.NoError(t, err)
				require.Len(t, objects, 1)
				require.Equal(t, "pre-install,pre-upgrade", objects[0].GetAnnotations()["helm.sh/hook"])
			}
		})
	}
}

func TestNestedShellErrorsPreserveRenderingMaskingAndExitCode(t *testing.T) {
	executable, err := os.Executable()
	require.NoError(t, err)
	for _, tc := range []struct {
		name                    string
		tty, noColor, force, ci bool
	}{
		{"terminal", true, false, false, false},
		{"redirected", false, false, false, false},
		{"CI redirected", false, false, false, true},
		{"CI forced", false, false, true, true},
		{"CI terminal", true, false, false, true},
		{"opt-out", true, true, true, false},
		{"CI opt-out", true, true, true, true},
		{"explicit-force", false, false, true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, key := range []string{"NO_COLOR", "CLICOLOR", "CLICOLOR_FORCE", "FORCE_COLOR", "ATMOS_FORCE_COLOR", "ATMOS_FORCE_TTY", "CI"} {
				t.Setenv(key, "")
			}
			if tc.ci {
				t.Setenv("CI", "1")
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
				env:     append(os.Environ(), "_ATMOS_NESTED_ERROR_HELPER=1"), stdin: strings.NewReader(""),
			})
			require.Error(t, err)
			require.Equal(t, 17, getExitCode(err))
			require.Equal(t, (tc.tty || tc.force) && !tc.noColor, strings.Contains(captured, "\x1b["))
			require.Contains(t, captured, fmt.Sprintf("nested-color=%t", (tc.tty || tc.force) && !tc.noColor))
			if tc.ci {
				require.Contains(t, captured, "nested-ci=1")
			}
			require.NotContains(t, captured, "# Error")
			require.NotContains(t, captured, "**Error:**")
			require.NotContains(t, captured, "nested-test-secret")
			require.Contains(t, captured, iolib.MaskReplacement)
			require.Equal(t, 1, strings.Count(captured, "💡"))
		})
	}
}
