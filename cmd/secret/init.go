package secret

import (
	"github.com/spf13/cobra"

	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/ui"
)

var initParser *flags.StandardParser

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Provision declared secrets, prompting for any that are missing.",
	Long:  "Scan the declared secrets for a component in a stack and interactively prompt for each required secret that is not yet initialized, writing the entered values to the configured backend.",
	Args:  cobra.NoArgs,
	RunE:  runSecretInit,
}

func init() {
	initParser = flags.NewStandardParser(
		flags.WithBoolFlag("force", "f", false, "Re-prompt and overwrite already-initialized secrets"),
		flags.WithBoolFlag("dry-run", "", false, "Show what would be initialized without prompting or writing"),
	)
	initParser.RegisterFlags(initCmd)
}

func runSecretInit(cmd *cobra.Command, args []string) error {
	defer perf.Track(nil, "secret.runSecretInit")()

	scope, err := parseScope(cmd)
	if err != nil {
		return err
	}
	force, _ := cmd.Flags().GetBool("force")
	dryRun, _ := cmd.Flags().GetBool("dry-run")

	svc, err := loadService(scope)
	if err != nil {
		return err
	}

	statuses := svc.Status()
	initialized := 0
	for i := range statuses {
		st := &statuses[i]
		needs := force || !st.Initialized
		if !needs {
			continue
		}
		if dryRun {
			ui.Infof("Would initialize `%s` (%s)", st.Declaration.Name, backendLabel(&st.Declaration))
			initialized++
			continue
		}

		ui.Infof("Initializing `%s` (%s)", st.Declaration.Name, backendLabel(&st.Declaration))
		value, promptErr := promptForSecretValue()
		if promptErr != nil {
			return promptErr
		}
		if err := svc.Set(st.Declaration.Name, value); err != nil {
			return err
		}
		initialized++
	}

	if dryRun {
		ui.Infof("Dry run: %d secret(s) would be initialized", initialized)
		return nil
	}
	ui.Successf("Initialized %d secret(s) for component `%s` in stack `%s`", initialized, scope.Component, scope.Stack)
	return nil
}
