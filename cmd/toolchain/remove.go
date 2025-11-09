package toolchain

import (
	"github.com/spf13/cobra"

	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/flags/compat"
	"github.com/cloudposse/atmos/toolchain"
)

var removeCmd = &cobra.Command{
	Use:   "remove <tool[@version]>",
	Short: "Remove a tool or version from .tool-versions file",
	Long:  `Remove a tool or specific version from the .tool-versions file.`,
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		filePath := toolchain.GetToolVersionsFilePath()
		tool, version, err := toolchain.ParseToolVersionArg(args[0])
		if err != nil {
			return err
		}
		return toolchain.RemoveToolVersion(filePath, tool, version)
	},
}

// RemoveCommandProvider implements the CommandProvider interface.
type RemoveCommandProvider struct{}

func (r *RemoveCommandProvider) GetCommand() *cobra.Command {
	return removeCmd
}

func (r *RemoveCommandProvider) GetName() string {
	return "remove"
}

func (r *RemoveCommandProvider) GetGroup() string {
	return "Toolchain Commands"
}

func (r *RemoveCommandProvider) GetFlagsBuilder() flags.Builder {
	return nil
}

func (r *RemoveCommandProvider) GetPositionalArgsBuilder() *flags.PositionalArgsBuilder {
	return nil
}

func (r *RemoveCommandProvider) GetCompatibilityFlags() map[string]compat.CompatibilityFlag {
	return nil
}
