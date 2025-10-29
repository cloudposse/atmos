package ai

import (
	"github.com/spf13/cobra"

	"github.com/cloudposse/atmos/cmd/internal"
	"github.com/cloudposse/atmos/pkg/schema"
)

// aiCmd represents the ai command.
var aiCmd = &cobra.Command{
	Use:   "ai",
	Short: "AI-powered assistant for Atmos operations",
	Long: `AI-powered assistant that helps with Atmos infrastructure management.

The AI assistant provides intelligent help with:
- Understanding Atmos concepts and architecture
- Analyzing component and stack configurations
- Suggesting best practices for infrastructure management
- Debugging configuration issues
- Guiding through complex workflows
- Explaining Terraform integration patterns

The assistant has access to your current Atmos configuration and can:
- Describe components and their configurations
- List available components and stacks
- Validate stack configurations
- Generate Terraform plans (read-only)
- Access Atmos settings and configuration`,
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
