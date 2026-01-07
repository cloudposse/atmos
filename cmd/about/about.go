package about

import (
	"github.com/spf13/cobra"

	"github.com/cloudposse/atmos/cmd/internal"
	"github.com/cloudposse/atmos/cmd/markdown"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/flags/compat"
	"github.com/cloudposse/atmos/pkg/ui"
)

// aboutCmd represents the about command.
var aboutCmd = &cobra.Command{
	Use:   "about",
	Short: "Learn about Atmos",
	Long:  `Display information about Atmos, its features, and benefits.`,
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return ui.Markdown(markdown.AboutMarkdown)
	},
}

func init() {
	// Register this command with the registry.
	// This happens during package initialization via blank import in cmd/root.go.
	internal.Register(&AboutCommandProvider{})
}

// AboutCommandProvider implements the CommandProvider interface.
type AboutCommandProvider struct{}

// GetCommand returns the about command.
func (a *AboutCommandProvider) GetCommand() *cobra.Command {
	return aboutCmd
}

// GetName returns the command name.
func (a *AboutCommandProvider) GetName() string {
	return "about"
}

// GetGroup returns the command group for help organization.
func (a *AboutCommandProvider) GetGroup() string {
	return "Other Commands"
}

// GetFlagsBuilder returns the flags builder for this command.
// About command has no flags.
func (a *AboutCommandProvider) GetFlagsBuilder() flags.Builder {
	return nil
}

// GetPositionalArgsBuilder returns the positional args builder for this command.
// About command has no positional arguments.
func (a *AboutCommandProvider) GetPositionalArgsBuilder() *flags.PositionalArgsBuilder {
	return nil
}

// GetCompatibilityFlags returns compatibility flags for this command.
// About command has no compatibility flags.
func (a *AboutCommandProvider) GetCompatibilityFlags() map[string]compat.CompatibilityFlag {
	return nil
}

// GetAliases returns command aliases.
// About command has no aliases.
func (a *AboutCommandProvider) GetAliases() []internal.CommandAlias {
	return nil
}

// IsExperimental returns whether this command is experimental.
func (a *AboutCommandProvider) IsExperimental() bool {
	return false
}
