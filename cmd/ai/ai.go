package ai

import (
	_ "embed"

	"github.com/spf13/cobra"

	"github.com/cloudposse/atmos/cmd/internal"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/flags/compat"
	"github.com/cloudposse/atmos/pkg/schema"
)

//go:embed markdown/atmos_ai.md
var aiLongMarkdown string

// aiCmd represents the ai command.
var aiCmd = &cobra.Command{
	Use:   "ai",
	Short: "AI-powered assistant for Atmos operations",
	Long:  aiLongMarkdown,
}

// isAIEnabled checks if AI features are enabled in the configuration.
func isAIEnabled(atmosConfig *schema.AtmosConfiguration) bool {
	return atmosConfig.Settings.AI.Enabled
}

func init() {
	// Attach subcommands to ai command.
	// These will be added in the init() functions of each subcommand file.

	// Register this built-in command with the registry.
	// This happens during package initialization via blank import.
	internal.Register(&AICommandProvider{})
}

// AICommandProvider implements the CommandProvider interface.
type AICommandProvider struct{}

func (a *AICommandProvider) GetCommand() *cobra.Command {
	return aiCmd
}

func (a *AICommandProvider) GetName() string {
	return "ai"
}

func (a *AICommandProvider) GetGroup() string {
	return "Other Commands"
}

// GetFlagsBuilder returns the flags builder for this command.
func (a *AICommandProvider) GetFlagsBuilder() flags.Builder {
	return nil
}

// GetPositionalArgsBuilder returns the positional args builder for this command.
func (a *AICommandProvider) GetPositionalArgsBuilder() *flags.PositionalArgsBuilder {
	return nil
}

// GetCompatibilityFlags returns compatibility flags for this command.
func (a *AICommandProvider) GetCompatibilityFlags() map[string]compat.CompatibilityFlag {
	return nil
}

// GetAliases returns a list of command aliases to register.
func (a *AICommandProvider) GetAliases() []internal.CommandAlias {
	return nil
}

// IsExperimental returns whether this command is experimental.
func (a *AICommandProvider) IsExperimental() bool {
	return true
}
