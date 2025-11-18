package devcontainer

import (
	"github.com/spf13/cobra"

	"github.com/cloudposse/atmos/cmd/internal"
	"github.com/cloudposse/atmos/cmd/markdown"
	"github.com/cloudposse/atmos/pkg/schema"
)

// atmosConfigPtr will be set by SetAtmosConfig before command execution.
var atmosConfigPtr *schema.AtmosConfiguration

// SetAtmosConfig sets the Atmos configuration for the devcontainer command.
// This is called from root.go after atmosConfig is initialized.
func SetAtmosConfig(config *schema.AtmosConfiguration) {
	atmosConfigPtr = config
}

// devcontainerCmd represents the devcontainer command.
var devcontainerCmd = &cobra.Command{
	Use:   "devcontainer",
	Short: "Manage development containers",
	Long: `Manage development containers for your Atmos workflows.

Devcontainers provide isolated, reproducible development environments with all
required tools and dependencies pre-configured. This command supports both Docker
and Podman runtimes with automatic detection.`,
	Example: markdown.DevcontainerUsageMarkdown,
}

func init() {
	// Register this command with the registry.
	// This happens during package initialization via blank import in cmd/root.go.
	internal.Register(&DevcontainerCommandProvider{})
}

// DevcontainerCommandProvider implements the CommandProvider interface.
type DevcontainerCommandProvider struct{}

// GetCommand returns the devcontainer command.
func (d *DevcontainerCommandProvider) GetCommand() *cobra.Command {
	return devcontainerCmd
}

// GetName returns the command name.
func (d *DevcontainerCommandProvider) GetName() string {
	return "devcontainer"
}

// GetGroup returns the command group for help organization.
func (d *DevcontainerCommandProvider) GetGroup() string {
	return "Workflow Commands"
}

// GetAliases returns command aliases (none for devcontainer).
func (d *DevcontainerCommandProvider) GetAliases() []internal.CommandAlias {
	return nil
}
