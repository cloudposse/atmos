package terraform

import (
	"github.com/spf13/cobra"
)

// RegisterTerraformCompletions registers all completion functions for a terraform subcommand.
// This includes:
//   - Component positional arg completion (first arg)
//   - Identity flag completion
//
// Note: Stack flag completion is handled by the flag handler's
// registerPersistentCompletions() method in pkg/flags/standard.go.
// Custom completion functions are configured in the flag definition.
func RegisterTerraformCompletions(tfCmd *cobra.Command) {
	// Component name completion for positional argument.
	// This is command-specific, so must be registered on each subcommand.
	tfCmd.ValidArgsFunction = componentsArgCompletion

	// Identity flag completion.
	addIdentityCompletion(tfCmd)
}
