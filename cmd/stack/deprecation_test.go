package stack

import (
	"bytes"
	stdio "io"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/data"
	iolib "github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/pkg/ui"
)

// splitTestStreams keeps the data channel (stdout) and UI channel (stderr) apart.
type splitTestStreams struct{ stdout, stderr *bytes.Buffer }

func (s splitTestStreams) Input() stdio.Reader     { return bytes.NewReader(nil) }
func (s splitTestStreams) Output() stdio.Writer    { return plainWriter{s.stdout} }
func (s splitTestStreams) Error() stdio.Writer     { return plainWriter{s.stderr} }
func (s splitTestStreams) RawOutput() stdio.Writer { return plainWriter{s.stdout} }
func (s splitTestStreams) RawError() stdio.Writer  { return plainWriter{s.stderr} }

// A deprecated legacy command's notice must not reach stdout, where it would corrupt
// `value=$(atmos stack get ...)` and JSON output.
func TestDeprecatedLegacyCommandReportsReplacementOnStderrOnly(t *testing.T) {
	streams := splitTestStreams{stdout: &bytes.Buffer{}, stderr: &bytes.Buffer{}}
	ctx, err := iolib.NewContext(iolib.WithStreams(streams))
	require.NoError(t, err)
	data.InitWriter(ctx)
	ui.InitFormatter(ctx)
	t.Cleanup(data.Reset)
	t.Cleanup(ui.Reset)

	cobraOutput := &bytes.Buffer{}
	command := &cobra.Command{
		Use: "get",
		RunE: func(*cobra.Command, []string) error {
			return data.Writeln(`["alpha"]`)
		},
	}
	command.SetOut(cobraOutput)
	deprecateLegacyCommand(command, "atmos stack config get")
	command.SetArgs([]string{})

	require.NoError(t, command.Execute())

	assert.Equal(t, "[\"alpha\"]\n", streams.stdout.String())
	assert.Empty(t, cobraOutput.String(), "Cobra's output stream is stdout in Atmos")
	// The UI formatter renders the Markdown code span, so assert the words rather than backticks.
	assert.Contains(t, streams.stderr.String(), `Command "get" is deprecated`)
	assert.Contains(t, streams.stderr.String(), "atmos stack config get")
	assert.True(t, command.Hidden)
	assert.Empty(t, command.Deprecated)
}

func TestLegacyStackCommandsAreDeprecatedWithoutCobraNotices(t *testing.T) {
	for _, command := range []*cobra.Command{stackGetCmd, stackSetCmd, stackDeleteCmd, stackFormatCmd} {
		assert.True(t, command.Hidden, command.Name())
		assert.Empty(t, command.Deprecated, command.Name())
		assert.Equal(t, "atmos stack config "+command.Name(), command.Annotations[legacyReplacementAnnotation])
	}
}

// Each registered legacy command, run through its real wrapper against an explicit --file manifest,
// reports the replacement on stderr only: stdout carries just the command's own data.
func TestRegisteredLegacyStackCommandsReportReplacementOnStderrOnly(t *testing.T) {
	tests := []struct {
		command    *cobra.Command
		args       []string
		wantStdout string
	}{
		{stackGetCmd, []string{"vars.region"}, "us-east-1\n"},
		{stackSetCmd, []string{"vars.region", "us-west-2"}, ""},
		{stackDeleteCmd, []string{"vars.region"}, ""},
		{stackFormatCmd, nil, ""},
	}
	for _, tt := range tests {
		t.Run(tt.command.Name(), func(t *testing.T) {
			resetEditFlags(t)
			chdirToValidAtmosProject(t)
			streams := splitTestStreams{stdout: &bytes.Buffer{}, stderr: &bytes.Buffer{}}
			ctx, err := iolib.NewContext(iolib.WithStreams(streams))
			require.NoError(t, err)
			data.InitWriter(ctx)
			ui.InitFormatter(ctx)
			t.Cleanup(data.Reset)
			t.Cleanup(ui.Reset)

			file := filepath.Join(t.TempDir(), "prod.yaml")
			manifest := "components:\n  terraform:\n    mycomponent:\n      vars:\n        region: us-east-1\n"
			require.NoError(t, os.WriteFile(file, []byte(manifest), 0o644))
			flagStack, flagComponent, flagFile = "nonprod", "mycomponent", file

			require.NoError(t, tt.command.RunE(tt.command, tt.args))

			assert.Equal(t, tt.wantStdout, streams.stdout.String())
			assert.Contains(t, streams.stderr.String(), `Command "`+tt.command.Name()+`" is deprecated`)
			assert.Contains(t, streams.stderr.String(), "atmos stack config "+tt.command.Name())
		})
	}
}
