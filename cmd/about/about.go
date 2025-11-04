package about

import (
	"github.com/spf13/cobra"

	"github.com/cloudposse/atmos/cmd/internal"
	"github.com/cloudposse/atmos/cmd/markdown"
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
