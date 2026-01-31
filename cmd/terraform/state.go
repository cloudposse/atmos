package terraform

import (
	"github.com/spf13/cobra"

	"github.com/cloudposse/atmos/cmd/internal"
	"github.com/cloudposse/atmos/pkg/flags/compat"
)

// stateCmd represents the terraform state command.
var stateCmd = &cobra.Command{
	Use:   "state",
	Short: "Advanced state management",
	Long: `Advanced commands for managing Terraform state.

For complete Terraform/OpenTofu documentation, see:
  https://developer.hashicorp.com/terraform/cli/commands/state
  https://opentofu.org/docs/cli/commands/state`,
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: true},
	RunE: func(cmd *cobra.Command, args []string) error {
		return terraformRun(terraformCmd, cmd, args)
	},
}

// stateSubcommands defines the terraform state sub-subcommands.
// Each entry is registered as a Cobra child command of stateCmd,
// enabling proper command tree routing instead of hardcoded argument parsing.
// The compatFunc provides per-subcommand compat flags for the command registry.
var stateSubcommands = []struct {
	name       string
	short      string
	compatFunc func() map[string]compat.CompatibilityFlag
}{
	{"list", "List resources in the Terraform state", StateListCompatFlags},
	{"mv", "Move an item in Terraform state", StateMvCompatFlags},
	{"pull", "Pull current state and output to stdout", StatePullCompatFlags},
	{"push", "Update remote state from a local state file", StatePushCompatFlags},
	{"replace-provider", "Replace provider in the state", StateReplaceProviderCompatFlags},
	{"rm", "Remove instances from the Terraform state", StateRmCompatFlags},
	{"show", "Show a resource in the Terraform state", StateShowCompatFlags},
}

func init() {
	// Register sub-subcommands for state (e.g., "state list", "state mv").
	for _, sub := range stateSubcommands {
		stateCmd.AddCommand(newTerraformPassthroughSubcommand(stateCmd, sub.name, sub.short))
		internal.RegisterCommandCompatFlags("terraform", "state-"+sub.name, sub.compatFunc())
	}

	// Register completions for stateCmd.
	RegisterTerraformCompletions(stateCmd)

	// Register compat flags for this subcommand.
	internal.RegisterCommandCompatFlags("terraform", "state", StateCompatFlags())

	// Attach to parent terraform command.
	terraformCmd.AddCommand(stateCmd)
}
