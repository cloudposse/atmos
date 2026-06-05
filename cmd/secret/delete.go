package secret

import (
	"github.com/spf13/cobra"

	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/ui"
)

var deleteParser *flags.StandardParser

var deleteCmd = &cobra.Command{
	Use:     "delete NAME",
	Aliases: []string{"rm"},
	Short:   "Remove a declared secret's value from its backend.",
	Args:    cobra.ExactArgs(1),
	RunE:    runSecretDelete,
}

func init() {
	deleteParser = flags.NewStandardParser(
		flags.WithBoolFlag("force", "f", false, "Delete without confirmation"),
	)
	deleteParser.RegisterFlags(deleteCmd)
}

func runSecretDelete(cmd *cobra.Command, args []string) error {
	defer perf.Track(nil, "secret.runSecretDelete")()

	scope, err := parseScope(cmd)
	if err != nil {
		return err
	}
	name := args[0]

	force, _ := cmd.Flags().GetBool("force")
	if !force {
		confirmed, confErr := confirmAction("Delete secret `" + name + "` from its backend?")
		if confErr != nil {
			return confErr
		}
		if !confirmed {
			ui.Warning("Aborted")
			return nil
		}
	}

	svc, err := loadService(scope)
	if err != nil {
		return err
	}

	if err := svc.Delete(name); err != nil {
		return err
	}

	ui.Successf("Deleted secret `%s` for component `%s` in stack `%s`", name, scope.Component, scope.Stack)
	return nil
}
