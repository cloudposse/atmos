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
// It provides the ability to execute tools with specific versions, installing them if necessary.
type ExecCommandProvider struct{}

// GetCommand returns the Cobra command for toolchain exec.
func (e *ExecCommandProvider) GetCommand() *cobra.Command {
	return execCmd
}

// GetName returns the command name.
func (e *ExecCommandProvider) GetName() string {
	return "exec"
}

// GetGroup returns the command group for help display.
func (e *ExecCommandProvider) GetGroup() string {
	return "Toolchain Commands"
}

// GetFlagsBuilder returns nil as this command has no flags.
func (e *ExecCommandProvider) GetFlagsBuilder() flags.Builder {
	return nil
}

// GetPositionalArgsBuilder returns nil as positional args are handled by Cobra validation.
func (e *ExecCommandProvider) GetPositionalArgsBuilder() *flags.PositionalArgsBuilder {
	return nil
}

// GetCompatibilityFlags returns nil as this command has no compatibility flags.
func (e *ExecCommandProvider) GetCompatibilityFlags() map[string]compat.CompatibilityFlag {
	return nil
}
