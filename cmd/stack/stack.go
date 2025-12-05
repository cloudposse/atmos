package stack

import (
	"github.com/spf13/cobra"

	"github.com/cloudposse/atmos/cmd/internal"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/flags/compat"
)

// stackCmd is the parent command for stack operations.
var stackCmd = &cobra.Command{
	Use:                "stack",
	Short:              "Stack configuration operations",
	Long:               `Commands for working with Atmos stack configurations, including format conversion.`,
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: false},
	Args:               cobra.NoArgs,
}

func init() {
	// Attach subcommands.
	stackCmd.AddCommand(convertCmd)

	// Register with registry.
	internal.Register(&StackCommandProvider{})
}

// StackCommandProvider implements the CommandProvider interface.
type StackCommandProvider struct{}

func (s *StackCommandProvider) GetCommand() *cobra.Command {
	return stackCmd
}

func (s *StackCommandProvider) GetName() string {
	return "stack"
}

func (s *StackCommandProvider) GetGroup() string {
	return "Configuration Management"
}

func (s *StackCommandProvider) GetFlagsBuilder() flags.Builder {
	return nil
}

func (s *StackCommandProvider) GetPositionalArgsBuilder() *flags.PositionalArgsBuilder {
	return nil
}

func (s *StackCommandProvider) GetCompatibilityFlags() map[string]compat.CompatibilityFlag {
	return nil
}

// GetAliases returns command aliases (none for stack command).
func (s *StackCommandProvider) GetAliases() []internal.CommandAlias {
	return nil
}
