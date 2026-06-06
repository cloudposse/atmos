package secret

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/shell"
)

var execParser *flags.StandardParser

var execCmd = &cobra.Command{
	Use:   "exec -- <command> [args...]",
	Short: "Run a command with declared secrets injected as environment variables.",
	Long: "Resolve the declared secrets for a component in a stack and run a command with them set " +
		"as environment variables. The environment variable name is the secret's declaration name " +
		"verbatim. Use `--` to separate Atmos flags from the command and its arguments.\n\n" +
		"Secret values are written into the child process's environment in cleartext and are NOT " +
		"masked in the child's output.",
	Example: `  # Run a command with the component's secrets in the environment
  atmos secret exec --stack=dev --component=app -- env

  # Run a tool that reads secrets from the environment
  atmos secret exec --stack=prod --component=api -- ./deploy.sh`,
	Args:               cobra.MinimumNArgs(0), // Validated after the "--" separator.
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: false},
	RunE:               runSecretExec,
}

func init() {
	execParser = flags.NewStandardParser()
	execParser.RegisterFlags(execCmd)
	if err := execParser.BindToViper(viper.GetViper()); err != nil {
		panic(err)
	}
}

func runSecretExec(cmd *cobra.Command, _ []string) error {
	defer perf.Track(nil, "secret.runSecretExec")()

	scope, err := parseScope(cmd)
	if err != nil {
		return err
	}

	// Everything after "--" is the command to run.
	commandArgs := getSeparatedArgs(cmd)
	if len(commandArgs) == 0 {
		return fmt.Errorf(errUtils.ErrWrapFormat, errUtils.ErrNoCommandSpecified, errUtils.ErrInvalidSubcommand)
	}

	svc, atmosConfig, err := loadServiceAndConfig(scope)
	if err != nil {
		return err
	}

	env, _, err := resolveSecretEnv(svc, atmosConfig)
	if err != nil {
		return err
	}

	return shell.RunCommand(commandArgs, env)
}

// getSeparatedArgs returns the args after the "--" separator, using Cobra's
// ArgsLenAtDash() to locate it. Returns nil when no "--" was provided.
func getSeparatedArgs(cmd *cobra.Command) []string {
	args := cmd.Flags().Args()
	dashIndex := cmd.Flags().ArgsLenAtDash()

	if dashIndex >= 0 && dashIndex < len(args) {
		return args[dashIndex:]
	}
	return nil
}
