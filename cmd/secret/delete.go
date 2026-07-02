package secret

import (
	"github.com/spf13/cobra"

	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/ui"
)

var deleteParser *flags.StandardParser

var deleteCmd = &cobra.Command{
	Use:     "delete [NAME]",
	Aliases: []string{"rm"},
	Short:   "Remove a declared secret's value from its backend (or all with --all).",
	Args:    cobra.MaximumNArgs(1),
	RunE:    runSecretDelete,
}

func init() {
	deleteParser = flags.NewStandardParser(
		flags.WithBoolFlag("force", "f", false, "Delete without confirmation"),
		flags.WithBoolFlag("all", "", false, "Delete all declared secrets for the scope (resets a SOPS file to a clean state)"),
	)
	deleteParser.RegisterFlags(deleteCmd)
}

func runSecretDelete(cmd *cobra.Command, args []string) error {
	defer perf.Track(nil, "secret.runSecretDelete")()

	scope, err := parseScope(cmd, args)
	if err != nil {
		return err
	}

	all, _ := cmd.Flags().GetBool("all")
	if all {
		return runSecretDeleteAll(cmd, scope)
	}

	if len(args) != 1 {
		return ErrSecretNameRequired
	}
	name := args[0]

	force, _ := cmd.Flags().GetBool("force")
	if !force {
		confirmed, confErr := confirmActionFn("Delete secret `" + name + "` from its backend?")
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

	if err := svc.Delete(name); err != nil {
		return err
	}

	ui.Successf("Deleted secret `%s` for component `%s` in stack `%s`", name, scope.Component, scope.Stack)
	return nil
}

// runSecretDeleteAll clears every declared secret's value for the scope (used to reset a SOPS file).
func runSecretDeleteAll(cmd *cobra.Command, scope secretScope) error {
	force, _ := cmd.Flags().GetBool("force")
	if !force {
		confirmed, confErr := confirmActionFn("Delete ALL declared secrets for component `" + scope.Component + "` in stack `" + scope.Stack + "`?")
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

	count, err := svc.DeleteAll()
	if err != nil {
		return err
	}

	ui.Successf("Deleted %d declared secret(s) for component `%s` in stack `%s`", count, scope.Component, scope.Stack)
	return nil
}
