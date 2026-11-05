package plugin

import (
	"github.com/spf13/cobra"

	errUtils "github.com/cloudposse/atmos/errors"
	atmosio "github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/pkg/plugin/bootstrap"
	"github.com/cloudposse/atmos/pkg/ui"
)

func runCommand() *cobra.Command {
	return &cobra.Command{Use: "run <alias> -- [arguments...]", Short: "Run an installed plugin command", Args: cobra.MinimumNArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		if atmosConfig == nil {
			return errUtils.ErrPluginInvalidConfig
		}
		if len(args) > 1 && cmd.ArgsLenAtDash() != 1 {
			return errUtils.ErrPluginInvalidConfig
		}
		streams := atmosio.GetContext()
		return bootstrap.Run(cmd.Context(), atmosConfig, args[0], args[1:], bootstrap.Output{
			Stdout: streams.Data(), Stderr: streams.UI(), OnStart: displayCommand,
		})
	}}
}

func displayCommand(alias, name, description string) {
	ui.Infof("Running **%s** command `%s`.", alias, name)
	if description != "" {
		ui.Info(description)
	}
}
