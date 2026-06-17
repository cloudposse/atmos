package secret

import (
	"bytes"
	stdio "io"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/cloudposse/atmos/pkg/data"
	iolib "github.com/cloudposse/atmos/pkg/io"
)

// testStreams is a minimal io.Streams backed by buffers, used to initialize the data/ui channels so
// handlers that write output (e.g. `secret get`, `secret pull`) do not panic in tests.
type testStreams struct {
	stdin  stdio.Reader
	stdout *bytes.Buffer
	stderr *bytes.Buffer
}

func (ts *testStreams) Input() stdio.Reader     { return ts.stdin }
func (ts *testStreams) Output() stdio.Writer    { return ts.stdout }
func (ts *testStreams) Error() stdio.Writer     { return ts.stderr }
func (ts *testStreams) RawOutput() stdio.Writer { return ts.stdout }
func (ts *testStreams) RawError() stdio.Writer  { return ts.stderr }

// setupIO initializes the global data/ui I/O context with buffers and resets it via t.Cleanup so
// handlers that write to the data channel (e.g. `secret get`, `secret pull`) do not panic and do
// not leak output between tests.
func setupIO(t *testing.T) {
	t.Helper()

	streams := &testStreams{stdin: &bytes.Buffer{}, stdout: &bytes.Buffer{}, stderr: &bytes.Buffer{}}
	ioCtx, err := iolib.NewContext(iolib.WithStreams(streams))
	if err != nil {
		t.Fatalf("failed to create I/O context: %v", err)
	}
	data.InitWriter(ioCtx)
	t.Cleanup(func() { data.Reset() })
}

// resetSecretFlags resets the secret command tree's persistent scope flags and every subcommand's
// local flags to their defaults so each test starts from a clean slate. Cobra retains parsed flag
// values across Execute() calls, and those values are bound into the global viper via BindPFlag — so
// without this reset a --stack from one test leaks into the next. We reset the pflag values (not
// viper overrides): viper.Set would take highest precedence and defeat subsequent flag parsing.
func resetSecretFlags(t *testing.T) {
	t.Helper()

	resetCmdFlags := func(cmd *cobra.Command) {
		cmd.Flags().VisitAll(func(f *pflag.Flag) {
			_ = f.Value.Set(f.DefValue)
			f.Changed = false
		})
		cmd.PersistentFlags().VisitAll(func(f *pflag.Flag) {
			_ = f.Value.Set(f.DefValue)
			f.Changed = false
		})
	}

	resetCmdFlags(secretCmd)
	for _, sub := range secretCmd.Commands() {
		resetCmdFlags(sub)
	}
	t.Cleanup(func() {
		resetCmdFlags(secretCmd)
		for _, sub := range secretCmd.Commands() {
			resetCmdFlags(sub)
		}
	})
}

// runSecretSubcommand executes the given secret subcommand by name through the secretCmd parent so
// the persistent scope flags (stack/component/type/identity) are wired exactly as in production. The
// caller passes the subcommand args (including its name and any --stack/--component flags).
func runSecretSubcommand(t *testing.T, args ...string) error {
	t.Helper()

	resetSecretFlags(t)
	secretCmd.SetArgs(args)
	t.Cleanup(func() { secretCmd.SetArgs(nil) })
	return secretCmd.Execute()
}
