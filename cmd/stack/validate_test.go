package stack

import (
	"bytes"
	stdio "io"
	"path/filepath"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	iolib "github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui"
)

// stackValidateTestStreams is a minimal io.Streams implementation for
// capturing ui.* output (UIStream) in tests. Captured text is stored without
// ANSI escapes because CI runners can advertise color support, which would
// otherwise split the text that tests assert on.
type stackValidateTestStreams struct{ output *bytes.Buffer }

// plainWriter strips ANSI escape sequences before writing to the buffer.
type plainWriter struct{ buffer *bytes.Buffer }

func (w plainWriter) Write(p []byte) (int, error) {
	if _, err := w.buffer.WriteString(ansi.Strip(string(p))); err != nil {
		return 0, err
	}
	return len(p), nil
}

func (s stackValidateTestStreams) Input() stdio.Reader     { return bytes.NewReader(nil) }
func (s stackValidateTestStreams) Output() stdio.Writer    { return plainWriter{s.output} }
func (s stackValidateTestStreams) Error() stdio.Writer     { return plainWriter{s.output} }
func (s stackValidateTestStreams) RawOutput() stdio.Writer { return plainWriter{s.output} }
func (s stackValidateTestStreams) RawError() stdio.Writer  { return plainWriter{s.output} }

func TestStackValidateCmd_RegisteredUnderStack(t *testing.T) {
	found := false
	for _, c := range stackCmd.Commands() {
		if c.Name() == "validate" {
			found = true
			break
		}
	}
	assert.True(t, found, "expected \"validate\" to be registered as a subcommand of \"stack\"")
}

func TestStackValidateCmd_HasSchemaOverrideFlag(t *testing.T) {
	// The alias must accept the same schema-override flag as `atmos validate
	// stacks`, which the shared executor reads from the command's flag set.
	flag := stackValidateCmd.PersistentFlags().Lookup("schemas-atmos-manifest")
	require.NotNil(t, flag, "expected the schemas-atmos-manifest flag to be defined")
	assert.NotNil(t, stackValidateCmd.PersistentFlags().Lookup("affected"))
	assert.NotNil(t, stackValidateCmd.PersistentFlags().Lookup("base"))
}

func TestStackValidateCmdRichFormat(t *testing.T) {
	original := atmosConfigPtr
	t.Cleanup(func() { atmosConfigPtr = original })
	atmosConfigPtr = &schema.AtmosConfiguration{StacksBaseAbsolutePath: filepath.Join(t.TempDir(), "missing-stacks")}

	output := &bytes.Buffer{}
	ioCtx, err := iolib.NewContext(iolib.WithStreams(stackValidateTestStreams{output: output}))
	require.NoError(t, err)
	ui.InitFormatter(ioCtx)
	t.Cleanup(ui.Reset)

	command := &cobra.Command{}
	command.Flags().String("format", "rich", "")
	command.Flags().StringSlice("exclude", nil, "")
	require.NoError(t, stackValidateCmd.RunE(command, nil))
	assert.Contains(t, output.String(), "No stack manifests found")

	invalid := &cobra.Command{}
	invalid.Flags().String("format", "xml", "")
	invalid.Flags().StringSlice("exclude", nil, "")
	err = stackValidateCmd.RunE(invalid, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "expected text or rich")
}

// The end-to-end behavior (delegating to `atmos validate stacks`) requires the
// root command's inherited global flags, so it is exercised through the CLI
// integration tests rather than by invoking the bare subcommand's RunE here.
