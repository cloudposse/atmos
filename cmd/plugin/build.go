package plugin

import (
	"fmt"
	"io"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/flags"
	pluginbuild "github.com/cloudposse/atmos/pkg/plugin/build"
	"github.com/cloudposse/atmos/pkg/process"
	"github.com/cloudposse/atmos/pkg/runner/step"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui"
)

const (
	buildPlatformFlag     = "platform"
	buildAllPlatformsFlag = "all-platforms"
)

func buildCommand() *cobra.Command {
	parser := flags.NewStandardParser(
		flags.WithStringFlag(buildPlatformFlag, "", "", "Build a declared OS/architecture[/variant] target (defaults to the current platform)"),
		flags.WithBoolFlag(buildAllPlatformsFlag, "", false, "Build every platform declared in the manifest"),
		flags.WithEnvVars(buildPlatformFlag, "ATMOS_PLUGIN_BUILD_PLATFORM"),
		flags.WithEnvVars(buildAllPlatformsFlag, "ATMOS_PLUGIN_BUILD_ALL_PLATFORMS"),
	)
	command := &cobra.Command{
		Use: "build [directory]", Short: "Compile a Go plugin to its manifest's executable paths",
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			values := viper.New()
			if err := parser.BindFlagsToViper(cmd, values); err != nil {
				return err
			}
			platform, allPlatforms := values.GetString(buildPlatformFlag), values.GetBool(buildAllPlatformsFlag)
			if platform != "" && allPlatforms {
				return fmt.Errorf("%w: --platform and --all-platforms cannot be combined", errUtils.ErrPluginFlagConflict)
			}
			directory := "."
			if len(args) > 0 {
				directory = args[0]
			}
			ui.Infof("Building plugin in `%s`...", filepath.Clean(directory))
			output := step.NewOutputModeWriter(step.OutputModeViewport, "Compiling plugin", &schema.ViewportConfig{
				Height: 4, Padding: 2,
			}).WithStdin(nil)
			var executables []string
			_, _, err := output.ExecuteWithIO(func(_, diagnostic io.Writer) error {
				var buildErr error
				// Compiler messages are diagnostics, including when Go emits them on stdout.
				// Keep the data channel empty in both viewport and redirected output modes.
				executables, buildErr = pluginbuild.CompileAll(cmd.Context(), directory,
					pluginbuild.WithPlatform(platform), pluginbuild.WithAllPlatforms(allPlatforms),
					pluginbuild.WithStreams(process.Streams{Stdout: diagnostic, Stderr: diagnostic}))
				return buildErr
			})
			for _, executable := range executables {
				ui.Successf("Plugin built successfully: `%s`", displayPath(executable))
			}
			return err
		},
	}
	parser.RegisterFlags(command)
	return command
}
