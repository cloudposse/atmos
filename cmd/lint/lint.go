package lint

import (
	"github.com/spf13/cobra"

	"github.com/cloudposse/atmos/cmd/internal"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/flags/compat"
)

// lintCmd is the parent command for all lint subcommands.
var lintCmd = &cobra.Command{
	Use:   "lint",
	Short: "Lint configurations for quality and best practices",
	Long:  `This command lints Atmos stack configurations for anti-patterns, optimization opportunities, and structural issues.`,
	Args:  cobra.NoArgs,
}

func init() {
	// Register this command with the registry.
	// This happens during package initialization via blank import in cmd/root.go.
	internal.Register(&LintCommandProvider{})
}

// LintCommandProvider implements the CommandProvider interface.
type LintCommandProvider struct{}

// GetCommand returns the lint command.
func (l *LintCommandProvider) GetCommand() *cobra.Command {
	return lintCmd
}

// GetName returns the command name.
func (l *LintCommandProvider) GetName() string {
	return "lint"
}

// GetGroup returns the command group for help organization.
func (l *LintCommandProvider) GetGroup() string {
	return "Stack Introspection"
}

// GetFlagsBuilder returns nil since lint has no top-level flags.
func (l *LintCommandProvider) GetFlagsBuilder() flags.Builder {
	return nil
}

// GetPositionalArgsBuilder returns nil since lint uses cobra.NoArgs.
func (l *LintCommandProvider) GetPositionalArgsBuilder() *flags.PositionalArgsBuilder {
	return nil
}

// GetCompatibilityFlags returns nil since lint has no compatibility flags.
func (l *LintCommandProvider) GetCompatibilityFlags() map[string]compat.CompatibilityFlag {
	return nil
}

// GetAliases returns nil since lint has no command aliases.
func (l *LintCommandProvider) GetAliases() []internal.CommandAlias {
	return nil
}

// IsExperimental returns whether this command is experimental.
func (l *LintCommandProvider) IsExperimental() bool {
	return false
}
