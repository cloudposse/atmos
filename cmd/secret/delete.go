package secret

import (
	"github.com/spf13/cobra"

	"github.com/cloudposse/atmos/cmd/internal"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/ui"
)

var deleteParser *flags.StandardParser

var deleteCmd = &cobra.Command{
	Use:     "delete [NAME]",
	Aliases: []string{"rm", "unset"},
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
	internal.RegisterCommandMetadata(
		deleteCmd,
		internal.WithDocumentationURL("https://atmos.tools/cli/commands/secret/delete"),
	)
}

func runSecretDelete(cmd *cobra.Command, args []string) error {
	defer perf.Track(nil, "secret.runSecretDelete")()

	all, _ := cmd.Flags().GetBool("all")
	if all {
		scope, err := parseScopeForStack(cmd, args, "delete")
		if err != nil {
			return err
		}
		return runSecretDeleteAll(cmd, scope)
	}

	name := ""
	if len(args) > 0 {
		name = args[0]
	}
	scope, err := parseScopeForName(cmd, args, name, "delete")
	if err != nil {
		return err
	}
	if len(args) != 1 {
		return ErrSecretNameRequired
	}

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

	ui.Successf("Deleted secret `%s` for %s", name, describeScope(scope))
	return nil
}

// runSecretDeleteAll clears every declared secret's value for the scope (used to reset a SOPS file).
func runSecretDeleteAll(cmd *cobra.Command, scope secretScope) error {
	force, _ := cmd.Flags().GetBool("force")
	if !force {
		confirmed, confErr := confirmActionFn("Delete ALL declared secrets for " + describeScope(scope) + "?")
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

	ui.Successf("Deleted %d declared secret(s) for %s", count, describeScope(scope))
	return nil
}
