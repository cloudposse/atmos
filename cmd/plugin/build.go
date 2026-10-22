package plugin

import (
	"io"
	"path/filepath"

	"github.com/spf13/cobra"

	pluginbuild "github.com/cloudposse/atmos/pkg/plugin/build"
	"github.com/cloudposse/atmos/pkg/process"
	"github.com/cloudposse/atmos/pkg/runner/step"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui"
)

func buildCommand() *cobra.Command {
	return &cobra.Command{
		Use: "build [directory]", Short: "Compile a Go plugin to its manifest's executable path",
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			directory := "."
			if len(args) > 0 {
				directory = args[0]
			}
			ui.Infof("Building plugin in `%s`...", filepath.Clean(directory))
			output := step.NewOutputModeWriter(step.OutputModeViewport, "Compiling plugin", &schema.ViewportConfig{
				Height: 4, Padding: 2,
			}).WithStdin(nil)
			var executable string
			_, _, err := output.ExecuteWithIO(func(_, diagnostic io.Writer) error {
				var buildErr error
				// Compiler messages are diagnostics, including when Go emits them on stdout.
				// Keep the data channel empty in both viewport and redirected output modes.
				executable, buildErr = pluginbuild.Compile(cmd.Context(), directory, pluginbuild.WithStreams(process.Streams{
					Stdout: diagnostic, Stderr: diagnostic,
				}))
				return buildErr
			})
			if err != nil {
				return err
			}
			ui.Successf("Plugin built successfully: `%s`", displayPath(executable))
			return nil
		},
	}
}
