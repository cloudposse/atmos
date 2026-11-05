package plugin

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/plugin/manage"
)

func updateCommand() *cobra.Command {
	parser := flags.NewStandardParser(flags.WithStringFlag("version", "", "", "Replace this package's version constraint"))
	command := &cobra.Command{Use: "update [alias]", Short: "Explicitly advance plugin versions within their constraints", Args: cobra.MaximumNArgs(1)}
	parser.RegisterFlags(command)
	command.RunE = func(cmd *cobra.Command, args []string) error {
		values := viper.New()
		if err := parser.BindFlagsToViper(cmd, values); err != nil {
			return err
		}
		constraint := values.GetString("version")
		if constraint != "" && len(args) == 0 {
			return errUtils.Build(fmt.Errorf("%w: --version needs a package alias", errUtils.ErrPluginInvalidConfig)).
				WithHint("A version constraint belongs to one package. Run `atmos plugin update <alias> --version <constraint>`; see `atmos plugin list` for aliases.").
				Err()
		}
		manager, err := newManager(cmd)
		if err != nil {
			return err
		}
		alias := ""
		if len(args) != 0 {
			alias = args[0]
		}
		if constraint != "" {
			declaration, found := atmosConfig.Plugins.Packages[alias]
			if !found {
				return notDeclared(alias)
			}
			return manager.Add(cmd.Context(), manage.AddOptions{Alias: alias, Source: declaration.Source, Constraint: constraint})
		}
		return manager.Restore(cmd.Context(), manage.RestoreOptions{Update: true, Alias: alias})
	}
	return command
}

func notDeclared(alias string) error {
	return errUtils.Build(fmt.Errorf("%w: %s", errUtils.ErrPluginNotInstalled, alias)).
		WithHintf("`%s` is not declared under `plugins.packages`. Run `atmos plugin list` for declared packages or `atmos plugin install <source> --as %s` to add it.", alias, alias).
		Err()
}
