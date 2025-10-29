package ai

import (
	"github.com/spf13/cobra"
)

// AgentCmd represents the 'atmos ai agent' command.
// Exported for use by agent subpackage.
var AgentCmd = &cobra.Command{
	Use:   "agent",
	Short: "Manage AI agents",
	Long: `Manage community and custom AI agents.

Agents are specialized AI assistants that provide expert knowledge for specific domains.
You can install community-contributed agents from GitHub repositories and manage them
using this command.

Available Commands:
  install     Install an agent from a GitHub repository
  list        List installed agents
  uninstall   Remove an installed agent
  info        Show detailed information about an agent

Examples:
  # Install an agent from GitHub
  atmos ai agent install github.com/user/agent-name
  atmos ai agent install github.com/user/agent-name@v1.2.3

  # List all installed agents
  atmos ai agent list

  # Uninstall an agent
  atmos ai agent uninstall agent-name`,
}

func init() {
	// Add 'agent' subcommand to 'ai' command.
	aiCmd.AddCommand(AgentCmd)
}
