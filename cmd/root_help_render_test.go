package cmd

import (
	"bytes"
	"errors"
	stdio "io"
	"os"
	"testing"

	"github.com/elewis787/boa"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ansi"
	"github.com/cloudposse/atmos/pkg/data"
	iolib "github.com/cloudposse/atmos/pkg/io"
)

// TestInitCobraConfig exercises initCobraConfig's wiring of RootCmd's output
// writer, usage/help funcs, and confirms the UsageFunc closure delegates to
// rootUsageFunc. It invokes the closure with a non-root command (rather than
// RootCmd itself) because rootUsageFunc's c.Use == rootCommandName branch
// delegates to boa, which opens a real TTY via bubbletea - unavailable in
// headless CI (see TestRootUsageFunc's comment for that branch).
func TestInitCobraConfig(t *testing.T) {
	_ = NewTestKit(t)
	originalOsExit := errUtils.OsExit
	defer func() { errUtils.OsExit = originalOsExit }()
	var exitCode int
	errUtils.OsExit = func(code int) {
		exitCode = code
		panic("os.Exit called")
	}

	ioCtx, err := iolib.NewContext()
	require.NoError(t, err)

	initCobraConfig(ioCtx)

	require.NotNil(t, RootCmd.HelpFunc())
	require.NotNil(t, RootCmd.UsageFunc())

	sub := &cobra.Command{Use: "sub"}
	var buf bytes.Buffer
	sub.SetOut(&buf)

	assert.Panics(t, func() {
		_ = RootCmd.UsageFunc()(sub)
	})
	assert.Equal(t, 1, exitCode)
}

func TestRootUsageFunc(t *testing.T) {
	styles := boa.DefaultStyles()
	b := boa.New(boa.WithStyles(styles))

	// Note: rootUsageFunc's c.Use == rootCommandName branch (which delegates
	// to boa.UsageFunc) is intentionally not covered here - boa renders usage
	// via bubbletea, which opens a real TTY (/dev/tty) and fails outright in
	// headless CI/sandboxes with no TTY available. That branch is exercised
	// live whenever a real invalid `atmos` invocation renders usage; it isn't
	// safely unit-testable without a pty.

	t.Run("valid positional args show usage without unknown command error", func(t *testing.T) {
		_ = NewTestKit(t)
		originalOsExit := errUtils.OsExit
		defer func() { errUtils.OsExit = originalOsExit }()
		var exitCode int
		errUtils.OsExit = func(code int) {
			exitCode = code
			panic("os.Exit called")
		}

		// No Args validator means all positional args validate (ValidateArgsOrNil
		// returns nil), so this should hit the "valid positional args" branch.
		sub := &cobra.Command{Use: "sub"}
		var buf bytes.Buffer
		sub.SetOut(&buf)
		require.NoError(t, sub.Flags().Parse([]string{"positional1"}))

		assert.Panics(t, func() {
			_ = rootUsageFunc(sub, b)
		})
		assert.Equal(t, 1, exitCode)
	})

	t.Run("invalid args fall through to showUsageAndExit", func(t *testing.T) {
		_ = NewTestKit(t)
		originalOsExit := errUtils.OsExit
		defer func() { errUtils.OsExit = originalOsExit }()
		var exitCode int
		errUtils.OsExit = func(code int) {
			exitCode = code
			panic("os.Exit called")
		}

		sub := &cobra.Command{Use: "sub", Args: cobra.ExactArgs(0)}
		var buf bytes.Buffer
		sub.SetOut(&buf)
		require.NoError(t, sub.Flags().Parse([]string{"unexpected"}))

		assert.Panics(t, func() {
			_ = rootUsageFunc(sub, b)
		})
		assert.Equal(t, 1, exitCode)
	})

	t.Run("no positional args shows usage and exits", func(t *testing.T) {
		_ = NewTestKit(t)
		originalOsExit := errUtils.OsExit
		defer func() { errUtils.OsExit = originalOsExit }()
		var exitCode int
		errUtils.OsExit = func(code int) {
			exitCode = code
			panic("os.Exit called")
		}

		sub := &cobra.Command{Use: "sub"}
		var buf bytes.Buffer
		sub.SetOut(&buf)

		assert.Panics(t, func() {
			_ = rootUsageFunc(sub, b)
		})
		assert.Equal(t, 1, exitCode)
	})
}

func TestIsHelpRequested(t *testing.T) {
	tests := []struct {
		name        string
		osArgs      []string
		args        []string
		flagChanged bool
		want        bool
	}{
		{name: "os.Args has --help", osArgs: []string{"atmos", "--help"}, want: true},
		{name: "os.Args has -h", osArgs: []string{"atmos", "-h"}, want: true},
		{name: "os.Args has help subcommand", osArgs: []string{"atmos", "help"}, want: true},
		{name: "args param has --help", osArgs: []string{"atmos"}, args: []string{"--help"}, want: true},
		{name: "args param has -h", osArgs: []string{"atmos"}, args: []string{"-h"}, want: true},
		{name: "args param has help", osArgs: []string{"atmos"}, args: []string{"help"}, want: true},
		{name: "help flag Changed on command", osArgs: []string{"atmos"}, flagChanged: true, want: true},
		{name: "nothing indicates help", osArgs: []string{"atmos", "version"}, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_ = NewTestKit(t)
			os.Args = tt.osArgs

			cmd := &cobra.Command{Use: "test"}
			cmd.Flags().Bool(helpFlagName, false, "help for test")
			if tt.flagChanged {
				require.NoError(t, cmd.Flags().Set(helpFlagName, "true"))
			}

			assert.Equal(t, tt.want, isHelpRequested(cmd, tt.args))
		})
	}
}

func TestParsePagerFlagValue(t *testing.T) {
	tests := []struct {
		in   string
		want bool
	}{
		{"true", true},
		{"on", true},
		{"yes", true},
		{"1", true},
		{"false", false},
		{"off", false},
		{"no", false},
		{"0", false},
		{"less", true}, // Unrecognized token is assumed to be a pager command.
	}

	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			assert.Equal(t, tt.want, parsePagerFlagValue(tt.in))
		})
	}
}

func TestRenderFlagHelp(t *testing.T) {
	t.Run("no pager flag renders directly", func(t *testing.T) {
		_ = NewTestKit(t)
		var outBuf bytes.Buffer
		cmd := &cobra.Command{Use: "test", Short: "test command description"}
		cmd.SetOut(&outBuf)

		renderFlagHelp(cmd)

		// Strip ANSI codes before checking text containment (Glamour wraps each word in styling,
		// e.g. when CI=true forces color output even without a real TTY).
		assert.Contains(t, ansi.Strip(outBuf.String()), "test command description")
	})

	t.Run("pager explicitly disabled renders directly", func(t *testing.T) {
		_ = NewTestKit(t)
		var outBuf bytes.Buffer
		cmd := &cobra.Command{Use: "test", Short: "test command description"}
		cmd.Flags().String("pager", "", "pager")
		require.NoError(t, cmd.Flags().Set("pager", "false"))
		cmd.SetOut(&outBuf)

		renderFlagHelp(cmd)

		assert.Contains(t, ansi.Strip(outBuf.String()), "test command description")
	})

	t.Run("pager explicitly enabled buffers then restores original writer", func(t *testing.T) {
		_ = NewTestKit(t)
		var outBuf bytes.Buffer
		cmd := &cobra.Command{Use: "test", Short: "test command description"}
		cmd.Flags().String("pager", "", "pager")
		require.NoError(t, cmd.Flags().Set("pager", "true"))
		cmd.SetOut(&outBuf)

		renderFlagHelp(cmd)

		// Regression check: the writer must be restored to the caller's
		// original buffer, not left pointed at the discarded internal buffer
		// used to capture help output for the pager.
		assert.Same(t, &outBuf, cmd.OutOrStdout())
	})
}

func TestRenderInteractiveHelp(t *testing.T) {
	t.Run("renders help and restores original writer", func(t *testing.T) {
		_ = NewTestKit(t)
		var outBuf bytes.Buffer
		cmd := &cobra.Command{Use: "test", Short: "test command description"}
		cmd.SetOut(&outBuf)

		renderInteractiveHelp(cmd)

		assert.Same(t, &outBuf, cmd.OutOrStdout())
	})

	t.Run("pager write failure logs a warning instead of erroring", func(t *testing.T) {
		_ = NewTestKit(t)

		// Force the global data writer (which the pager's direct-write
		// fallback goes through) to fail, so pager.Run returns a non-nil
		// error and renderInteractiveHelp takes its warn-and-continue branch.
		failingStreams := &failingWriterStreams{err: errors.New("simulated write failure")}
		failingCtx, err := iolib.NewContext(iolib.WithStreams(failingStreams))
		require.NoError(t, err)
		data.InitWriter(failingCtx)

		var outBuf bytes.Buffer
		cmd := &cobra.Command{Use: "test", Short: "test command description"}
		cmd.Flags().String("pager", "", "pager")
		require.NoError(t, cmd.Flags().Set("pager", "true"))
		cmd.SetOut(&outBuf)

		// Should not panic even though the underlying write fails - the
		// pager falls back to direct output and any error from that is only
		// logged, never propagated to the caller.
		assert.NotPanics(t, func() {
			renderInteractiveHelp(cmd)
		})
	})
}

// failingWriterStreams is an iolib.Streams implementation whose writers
// always fail, used to exercise error-handling branches that only trigger
// when the underlying write itself fails.
type failingWriterStreams struct {
	err error
}

func (s *failingWriterStreams) Input() stdio.Reader     { return bytes.NewReader(nil) }
func (s *failingWriterStreams) Output() stdio.Writer    { return failingWriter{err: s.err} }
func (s *failingWriterStreams) Error() stdio.Writer     { return failingWriter{err: s.err} }
func (s *failingWriterStreams) RawOutput() stdio.Writer { return failingWriter{err: s.err} }
func (s *failingWriterStreams) RawError() stdio.Writer  { return failingWriter{err: s.err} }

type failingWriter struct {
	err error
}

func (w failingWriter) Write(p []byte) (int, error) {
	return 0, w.err
}

func TestRenderRootHelp(t *testing.T) {
	tests := []struct {
		name   string
		osArgs []string
	}{
		{name: "flag help (--help)", osArgs: []string{"atmos", "--help"}},
		{name: "flag help (-h)", osArgs: []string{"atmos", "-h"}},
		{name: "interactive help (help)", osArgs: []string{"atmos", "help"}},
		{name: "fallback for anything else", osArgs: []string{"atmos", "version"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_ = NewTestKit(t)
			os.Args = tt.osArgs

			var outBuf bytes.Buffer
			cmd := &cobra.Command{Use: "test", Short: "test command description"}
			cmd.SetOut(&outBuf)

			assert.NotPanics(t, func() {
				renderRootHelp(cmd)
			})
		})
	}
}

func TestRootHelpFunc(t *testing.T) {
	t.Run("help not requested shows usage and exits", func(t *testing.T) {
		_ = NewTestKit(t)
		originalOsExit := errUtils.OsExit
		defer func() { errUtils.OsExit = originalOsExit }()
		var exitCode int
		errUtils.OsExit = func(code int) {
			exitCode = code
			panic("os.Exit called")
		}

		os.Args = []string{"atmos", "test"}
		var outBuf bytes.Buffer
		cmd := &cobra.Command{Use: "test", Short: "test command description"}
		cmd.SetOut(&outBuf)

		assert.Panics(t, func() {
			rootHelpFunc(cmd, nil)
		})
		assert.Equal(t, 1, exitCode)
	})

	t.Run("help requested renders and looks up example content", func(t *testing.T) {
		_ = NewTestKit(t)
		os.Args = []string{"atmos", "test", "--help"}

		var outBuf bytes.Buffer
		cmd := &cobra.Command{Use: "test", Short: "test command description"}
		cmd.SetOut(&outBuf)

		// Register a fake example entry keyed by the command's content name
		// so the examples[contentName] lookup branch is exercised.
		examples["test"] = ExampleContent{Content: "example usage of test"}
		defer delete(examples, "test")

		assert.NotPanics(t, func() {
			rootHelpFunc(cmd, []string{"--help"})
		})
		assert.Equal(t, "example usage of test", cmd.Example)
	})
}
