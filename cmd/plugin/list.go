package plugin

import (
	"github.com/spf13/cobra"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/plugin/bootstrap"
	"github.com/cloudposse/atmos/pkg/plugin/manage"
	pluginoutput "github.com/cloudposse/atmos/pkg/plugin/output"
)

func listCommand() *cobra.Command {
	parser := newOutputParser()
	command := &cobra.Command{Use: "list", Short: "List selected plugins and local path packages", Args: cobra.NoArgs}
	registerOutputFlags(command, parser)
	command.RunE = func(cmd *cobra.Command, args []string) error {
		values, err := parseOutputFlags(cmd, parser)
		if err != nil {
			return err
		}
		if atmosConfig == nil {
			return errUtils.ErrPluginInvalidConfig
		}
		result, err := manage.List(atmosConfig, bootstrap.ProjectDirectory(atmosConfig))
		if err != nil {
			return err
		}
		return pluginoutput.List(result, values.GetString("format"))
	}
	return command
}

func infoCommand() *cobra.Command {
	parser := newOutputParser()
	command := &cobra.Command{Use: "info <alias>", Short: "Inspect a plugin's source, digests, exports, or local path", Args: cobra.ExactArgs(1)}
	registerOutputFlags(command, parser)
	command.RunE = func(cmd *cobra.Command, args []string) error {
		values, err := parseOutputFlags(cmd, parser)
		if err != nil {
			return err
		}
		if atmosConfig == nil {
			return errUtils.ErrPluginInvalidConfig
		}
		result, err := manage.Info(atmosConfig, bootstrap.ProjectDirectory(atmosConfig), args[0])
		if err != nil {
			return err
		}
		return pluginoutput.Info(result, values.GetString("format"))
	}
	return command
}
