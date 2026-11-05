package plugin

import (
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/cloudposse/atmos/pkg/flags"
	atmosio "github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/pkg/plugin/bootstrap"
	"github.com/cloudposse/atmos/pkg/plugin/conformance"
	pluginoutput "github.com/cloudposse/atmos/pkg/plugin/output"
	"github.com/cloudposse/atmos/pkg/ui"
)

func testCommand() *cobra.Command {
	parser := flags.NewStandardParser(
		flags.WithBoolFlag("build", "", false, "Build the selected native plugin before testing"),
		flags.WithStringFlag("platform", "", "", "Require a declared OS/architecture[/variant] executable on this host"),
		flags.WithStringFlag("coverprofile", "", "", "Run Go module unit tests and save coverage at this package-relative path"),
		flags.WithStringFlag("run", "", "", "Select test names containing this text"),
		flags.WithStringFlag(outputFormatFlag, "", conformance.FormatText, "Report format: text or json"),
		flags.WithBoolFlag("integration", "", false, "Run explicitly configured live integration steps"),
	)
	command := &cobra.Command{Use: "test [directory]", Short: "Build or test a native plugin with isolated fixtures and authored test steps", Args: cobra.MaximumNArgs(1)}
	parser.RegisterFlags(command)
	command.RunE = func(cmd *cobra.Command, args []string) error {
		values := viper.New()
		if err := parser.BindFlagsToViper(cmd, values); err != nil {
			return err
		}
		format := values.GetString(outputFormatFlag)
		if err := pluginoutput.ValidateFormatAmong(format, conformance.FormatText, conformance.FormatJSON); err != nil {
			return err
		}
		directory := "."
		if len(args) > 0 {
			directory = args[0]
		}
		streams := atmosio.GetContext()
		options := conformance.Options{Build: values.GetBool("build"), Platform: values.GetString("platform"), CoverProfile: values.GetString("coverprofile"), Project: projectDirectory(), Run: values.GetString("run"), Integration: values.GetBool("integration"), Stdout: streams.UI(), Stderr: streams.UI()}
		if format == conformance.FormatText {
			mode := "isolated fixtures"
			if options.Integration {
				mode = "live integration steps"
			}
			ui.Infof("Testing plugin in `%s` with %s...", filepath.Clean(directory), mode)
			options.OnResult = conformance.Show
		}
		report, err := conformance.Run(cmd.Context(), directory, options)
		return conformance.Present(report, err, format)
	}
	return command
}

// projectDirectory is the directory policy input.path is relative to. Testing needs no project
// configuration, so without one it is the current directory.
func projectDirectory() string {
	if atmosConfig == nil {
		return "."
	}
	return bootstrap.ProjectDirectory(atmosConfig)
}
