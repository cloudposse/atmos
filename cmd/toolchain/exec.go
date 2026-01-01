package toolchain

import (
	"errors"
	"os/exec"

	"github.com/spf13/cobra"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/flags/compat"
	"github.com/cloudposse/atmos/toolchain"
)

var execCmd = &cobra.Command{
	Use:   "exec <tool@version> [-- args...]",
	Short: "Execute a tool with specified version",
	Long: `Execute a tool with a specific version, installing it if necessary.

Examples:
  # Execute terraform with version 1.5.0
  atmos toolchain exec terraform@1.5.0 -- version

  # Execute kubectl with version 1.28.0, passing additional arguments
  atmos toolchain exec kubectl@1.28.0 -- get pods -n default`,
	Args:         cobra.MinimumNArgs(1),
	SilenceUsage: true, // Don't show usage on error (tool's output is sufficient).
	RunE: func(cmd *cobra.Command, args []string) error {
		installer := toolchain.NewInstaller()
		err := toolchain.RunExecCommand(installer, args)
		if err != nil {
			// If the executed tool returned a non-zero exit code, exit with
			// that code without showing any error message.
			var exitErr *exec.ExitError
			if errors.As(err, &exitErr) {
				errUtils.OsExit(exitErr.ExitCode())
			}
		}
		return err
	},
}

// ExecCommandProvider implements CommandProvider for the toolchain exec command.
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
