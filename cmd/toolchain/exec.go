package toolchain

import (
	"errors"
	"os/exec"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/flags/compat"
	"github.com/cloudposse/atmos/pkg/toolchain"
)

var execParser *flags.StandardParser

var execCmd = &cobra.Command{
	Use:   "exec <tool@version> [-- args...]",
	Short: "Execute a tool with specified version",
	Long: `Execute a tool with a specific version, installing it if necessary.

Examples:
  # Execute terraform with version 1.5.0
  atmos toolchain exec terraform@1.5.0 -- version

  # Execute kubectl with version 1.28.0, passing additional arguments
  atmos toolchain exec kubectl@1.28.0 -- get pods -n default

  # Show what would be executed without actually running it
  atmos toolchain exec --dry-run terraform@1.5.0 -- version`,
	Args:         cobra.MinimumNArgs(1),
	SilenceUsage: true, // Don't show usage on error (tool's output is sufficient).
	RunE: func(cmd *cobra.Command, args []string) error {
		// Bind flags to Viper for precedence handling.
		v := viper.GetViper()
		if err := execParser.BindFlagsToViper(cmd, v); err != nil {
			return err
		}
		// Use IsBoolFlagExplicitlySet rather than v.GetBool: both "exec" and
		// "update" register a "dry-run" flag on the shared global Viper instance
		// under the same key, so v.BindEnv calls for one command's env var can
		// overwrite the other's binding for that key. IsBoolFlagExplicitlySet
		// checks this parser's own registry/env vars directly, avoiding the
		// cross-command collision.
		_, dryRun := execParser.IsBoolFlagExplicitlySet(cmd, "dry-run")

		installer := toolchain.NewInstaller()
		err := toolchain.RunExecCommandWithOptions(installer, args, dryRun)
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

func init() {
	// Create parser with exec-specific flags.
	execParser = flags.NewStandardParser(
		flags.WithBoolFlag("dry-run", "", false, "Show the resolved binary and arguments without executing the tool"),
		flags.WithEnvVars("dry-run", "ATMOS_TOOLCHAIN_DRY_RUN"),
	)

	// Register flags.
	execParser.RegisterFlags(execCmd)

	// Bind flags to Viper.
	if err := execParser.BindToViper(viper.GetViper()); err != nil {
		panic(err)
	}
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

// GetFlagsBuilder returns the flags builder for the exec command.
func (e *ExecCommandProvider) GetFlagsBuilder() flags.Builder {
	return execParser
}

// GetPositionalArgsBuilder returns nil as positional args are handled by Cobra validation.
func (e *ExecCommandProvider) GetPositionalArgsBuilder() *flags.PositionalArgsBuilder {
	return nil
}

// GetCompatibilityFlags returns nil as this command has no compatibility flags.
func (e *ExecCommandProvider) GetCompatibilityFlags() map[string]compat.CompatibilityFlag {
	return nil
}
