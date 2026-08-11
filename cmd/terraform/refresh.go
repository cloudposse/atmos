package terraform

import (
	"github.com/spf13/cobra"

	"github.com/cloudposse/atmos/cmd/internal"
	h "github.com/cloudposse/atmos/pkg/hooks"
)

// refreshCmd represents the terraform refresh command.
var refreshCmd = &cobra.Command{
	Use:   "refresh",
	Short: "Update the state to match remote systems",
	Long: `Refresh the Terraform state, reconciling the local state with the actual infrastructure state.

For complete Terraform/OpenTofu documentation, see:
  https://developer.hashicorp.com/terraform/cli/commands/refresh
  https://opentofu.org/docs/cli/commands/refresh`,
	PreRunE: func(cmd *cobra.Command, args []string) error {
		return runBeforeHooks(h.BeforeTerraformRefresh, cmd, args)
	},
	RunE: func(cmd *cobra.Command, args []string) (runErr error) {
		// Reset before any early return so the deferred hook and PostRunE read
		// consistent state.
		wasMultiComponentExecution = false

		// On failure, run after hooks with error context. Cobra skips PostRunE on
		// error, so this is the only place the after.terraform.refresh hook fires
		// when a refresh fails. In multi-component mode the per-component hook
		// already fired for each component, so the global error call is
		// suppressed to avoid double-firing.
		defer func() {
			if runErr != nil && !wasMultiComponentExecution {
				runHooksOnErrorWithOutput(h.AfterTerraformRefresh, cmd, args, runErr, "")
			}
		}()

		return terraformRun(terraformCmd, cmd, args)
	},
	PostRunE: func(cmd *cobra.Command, args []string) error {
		// In multi-component mode, per-component hooks already fired inside the
		// affected/all/query dispatch. Calling them again here would double-fire.
		if wasMultiComponentExecution {
			return nil
		}
		return runHooksWithOutput(h.AfterTerraformRefresh, cmd, args, "")
	},
}

func init() {
	// Register completions for refreshCmd.
	RegisterTerraformCompletions(refreshCmd)

	// Register compat flags for this subcommand.
	internal.RegisterCommandCompatFlags("terraform", "refresh", RefreshCompatFlags())

	// Attach to parent terraform command.
	terraformCmd.AddCommand(refreshCmd)
}
