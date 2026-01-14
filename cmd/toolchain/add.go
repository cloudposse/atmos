package toolchain

import (
	"github.com/spf13/cobra"

	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/flags/compat"
	"github.com/cloudposse/atmos/toolchain"
)

var addCmd = &cobra.Command{
	Use:   "add <tool@version>",
	Short: "Add a tool to .tool-versions file",
	Long:  `Add a tool and version to the .tool-versions file.`,
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		tool, version, err := toolchain.ParseToolVersionArg(args[0])
		if err != nil {
			return err
		}
		return toolchain.AddToolVersion(tool, version)
	},
}

// AddCommandProvider implements the CommandProvider interface.
type AddCommandProvider struct{}

func (a *AddCommandProvider) GetCommand() *cobra.Command {
	return addCmd
}

func (a *AddCommandProvider) GetName() string {
	return "add"
}

func (a *AddCommandProvider) GetGroup() string {
	return "Toolchain Commands"
}

func (a *AddCommandProvider) GetFlagsBuilder() flags.Builder {
	return nil
}

func (a *AddCommandProvider) GetPositionalArgsBuilder() *flags.PositionalArgsBuilder {
	return nil
}

func (a *AddCommandProvider) GetCompatibilityFlags() map[string]compat.CompatibilityFlag {
	return nil
}
