package plugin

import (
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/plugin/catalog"
	"github.com/cloudposse/atmos/pkg/ui"
)

func catalogSyncCommand() *cobra.Command {
	parser := flags.NewStandardParser(
		flags.WithStringFlag("inventory", "", "plugins/releases.yaml", "Repository-relative package inventory"),
		flags.WithStringFlag("head", "", "HEAD", "Committed revision containing the accepted inventory"),
		flags.WithBoolFlag("skip-missing", "", false, "Skip only releases absent from the registry"),
		flags.WithBoolFlag("verify-public", "", false, "Require public GHCR packages and verify anonymous access"),
	)
	command := &cobra.Command{Use: "sync <catalog>", Short: "Reconcile verified releases into a catalog in one publication", Args: cobra.ExactArgs(1)}
	parser.RegisterFlags(command)
	command.RunE = func(cmd *cobra.Command, args []string) error {
		values := viper.New()
		if err := parser.BindFlagsToViper(cmd, values); err != nil {
			return err
		}
		releases, err := catalog.ReadInventory(cmd.Context(), catalog.WithInventory(values.GetString("inventory")), catalog.WithHead(values.GetString("head")))
		if err != nil {
			return err
		}
		target, err := catalogReference(args[0])
		if err != nil {
			return err
		}
		manager, err := newManager(cmd)
		if err != nil {
			return err
		}
		var options []catalog.SyncOption
		if values.GetBool("skip-missing") {
			options = append(options, catalog.WithSkipMissing())
		}
		if values.GetBool("verify-public") {
			options = append(options, catalog.WithPublicVerification(newPublicVerifier()))
		}
		if err := catalog.NewSyncer(manager.Installer.Registry, manager.Installer.Descriptors).Sync(cmd.Context(), target, releases, options...); err != nil {
			return err
		}
		ui.Successf("Reconciled catalog `%s` from the committed package inventory.", target)
		return nil
	}
	return command
}
