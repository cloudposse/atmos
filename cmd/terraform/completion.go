package terraform

import (
	"github.com/spf13/cobra"
)

// RegisterTerraformCompletions registers all completion functions for a terraform subcommand.
// This includes:
//   - Component positional arg completion (first arg)
//   - Stack flag completion (filtered by component if provided)
//   - Identity flag completion
func RegisterTerraformCompletions(tfCmd *cobra.Command) {
	// Component name completion for positional argument.
	tfCmd.ValidArgsFunction = componentsArgCompletion

	// Stack flag completion - filters by component if one was provided.
	if tfCmd.Flag("stack") != nil {
		_ = tfCmd.RegisterFlagCompletionFunc("stack", stackFlagCompletion)
	}

	// Identity flag completion.
	addIdentityCompletion(tfCmd)
}
