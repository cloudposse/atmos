package cloudformation

import (
	"github.com/spf13/cobra"
)

// newStackSetCmd is the `atmos aws cloudformation stackset` verb group:
// multi-account/multi-region deployment orchestration via a `kind:
// aws/stackset` provision target.
func newStackSetCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "stackset",
		Short: "Manage multi-account/multi-region StackSets",
		RunE:  func(cmd *cobra.Command, _ []string) error { return cmd.Usage() },
	}
	cmd.AddCommand(newOperationCommand("create", "stackset-create", "Create a StackSet (and its initial stack instances, if configured)"))
	cmd.AddCommand(newOperationCommand("update", "stackset-update", "Update a StackSet's template/parameters/capabilities"))
	cmd.AddCommand(newOperationCommand(subCommandDelete, "stackset-delete", "Delete every stack instance, then the StackSet itself"))
	cmd.AddCommand(newOperationCommand("instances", "stackset-instances", "List a StackSet's stack instances"))
	return cmd
}
