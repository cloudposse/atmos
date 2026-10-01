package store

import (
	"github.com/spf13/cobra"

	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/ui"
)

var deleteParser *flags.StandardParser

var deleteCmd = &cobra.Command{
	Use:     "delete STORE KEY",
	Aliases: []string{"rm", "unset"},
	Short:   "Remove a value from a store.",
	Long:    "Remove a value from a configured store backend. Returns an error if the backend does not support deletion.",
	Args:    cobra.ExactArgs(2),
	RunE:    runStoreDelete,
}

func init() {
	deleteParser = flags.NewStandardParser(
		flags.WithBoolFlag("force", "f", false, "Delete without confirmation"),
	)
	deleteParser.RegisterFlags(deleteCmd)
}

func runStoreDelete(cmd *cobra.Command, args []string) error {
	defer perf.Track(nil, "store.runStoreDelete")()

	scope, err := parseStoreScope(cmd)
	if err != nil {
		return err
	}
	storeName, key := args[0], args[1]

	force, _ := cmd.Flags().GetBool("force")
	if !force {
		confirmed, confErr := confirmActionFn("Delete `" + key + "` from store `" + storeName + "`?")
		if confErr != nil {
			return confErr
		}
		if !confirmed {
			ui.Warning("Aborted")
			return nil
		}
	}

	svc, err := loadServiceFn(scope)
	if err != nil {
		return err
	}

	if err := svc.Delete(storeName, scope.Stack, scope.Component, key); err != nil {
		return err
	}

	ui.Successf("Deleted `%s` from store `%s`", key, storeName)
	return nil
}
