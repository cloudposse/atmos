package toolchain

import (
	"github.com/spf13/cobra"

	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/flags/compat"
	"github.com/cloudposse/atmos/toolchain"
)

var addCmd = &cobra.Command{
	Use:   "add <tool[@version]>...",
	Short: "Add tools to .tool-versions file",
	Long: `Add one or more tools and versions to the .tool-versions file.
If version is omitted, defaults to "latest".`,
	Args: cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		for _, arg := range args {
			tool, version, err := toolchain.ParseToolVersionArg(arg)
			if err != nil {
				return err
			}
			// Default to "latest" if no version specified.
			if version == "" {
				version = "latest"
			}
			if err := toolchain.AddToolVersion(tool, version); err != nil {
				return err
			}
		}
		return nil
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
