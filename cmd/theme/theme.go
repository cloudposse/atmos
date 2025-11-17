package theme

import (
	"github.com/spf13/cobra"

	"github.com/cloudposse/atmos/cmd/internal"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/flags/compat"
	"github.com/cloudposse/atmos/pkg/schema"
)

// atmosConfigPtr will be set by SetAtmosConfig before command execution.
var atmosConfigPtr *schema.AtmosConfiguration

// SetAtmosConfig sets the Atmos configuration for the theme commands.
// This is called from root.go after atmosConfig is initialized.
func SetAtmosConfig(config *schema.AtmosConfiguration) {
	atmosConfigPtr = config
}

// themeCmd manages terminal theme configuration.
var themeCmd = &cobra.Command{
	Use:   "theme",
	Short: "Manage terminal themes for Atmos CLI",
	Long:  "Configure and preview terminal themes that control the appearance of CLI output, tables, and markdown rendering.",
	Example: `# List all available themes
atmos theme list

# List only recommended themes
atmos theme list --recommended

# Show details and preview a specific theme
atmos theme show Dracula`,
}

func init() {
	// Register this command with the registry.
	// This happens during package initialization via blank import in cmd/root.go.
	internal.Register(&ThemeCommandProvider{})
}

// ThemeCommandProvider implements the CommandProvider interface.
type ThemeCommandProvider struct{}

// GetCommand returns the theme command.
func (t *ThemeCommandProvider) GetCommand() *cobra.Command {
	return themeCmd
}

// GetName returns the command name.
func (t *ThemeCommandProvider) GetName() string {
	return "theme"
}

// GetGroup returns the command group for help organization.
func (t *ThemeCommandProvider) GetGroup() string {
	return "Other Commands"
}

func (t *ThemeCommandProvider) GetFlagsBuilder() flags.Builder {
	return nil
}

func (t *ThemeCommandProvider) GetPositionalArgsBuilder() *flags.PositionalArgsBuilder {
	return nil
}

func (t *ThemeCommandProvider) GetCompatibilityFlags() map[string]compat.CompatibilityFlag {
	return nil
}
