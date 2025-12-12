package toolchain

import (
	"github.com/spf13/cobra"

	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/flags/compat"
	"github.com/cloudposse/atmos/toolchain"
)

var execCmd = &cobra.Command{
	Use:   "exec <tool@version> [args...]",
	Short: "Execute a tool with specified version",
	Long:  `Execute a tool with a specific version, installing it if necessary.`,
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		installer := toolchain.NewInstaller()
		return toolchain.RunExecCommand(installer, args)
	},
}

// ExecCommandProvider implements the CommandProvider interface.
type ExecCommandProvider struct{}

func (e *ExecCommandProvider) GetCommand() *cobra.Command {
	return execCmd
}

func (e *ExecCommandProvider) GetName() string {
	return "exec"
}

func (e *ExecCommandProvider) GetGroup() string {
	return "Toolchain Commands"
}

func (e *ExecCommandProvider) GetFlagsBuilder() flags.Builder {
	return nil
}

func (e *ExecCommandProvider) GetPositionalArgsBuilder() *flags.PositionalArgsBuilder {
	return nil
}

func (e *ExecCommandProvider) GetCompatibilityFlags() map[string]compat.CompatibilityFlag {
	return nil
}
