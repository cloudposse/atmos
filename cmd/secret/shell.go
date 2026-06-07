package secret

import (
	"fmt"
	"strconv"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	errUtils "github.com/cloudposse/atmos/errors"
	envpkg "github.com/cloudposse/atmos/pkg/env"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/shell"
	"github.com/cloudposse/atmos/pkg/ui"
)

const shellFlagName = "shell"

var shellParser *flags.StandardParser

var shellCmd = &cobra.Command{
	Use:   "shell [-- [shell args...]]",
	Short: "Launch an interactive shell with declared secrets in the environment.",
	Long: "Resolve the declared secrets for a component in a stack and launch an interactive shell with " +
		"them set as environment variables. The environment variable name is the secret's declaration " +
		"name verbatim. Use `--` to pass arguments through to the shell.\n\n" +
		"Secret values are present in the shell's environment in cleartext and are NOT masked in the " +
		"shell's output.",
	Example: `  # Open a shell with the component's secrets in the environment
  atmos secret shell --stack=dev --component=app

  # Choose the shell binary
  atmos secret shell --stack=dev --component=app --shell=bash`,
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: false},
	ValidArgsFunction:  cobra.NoFileCompletions,
	RunE:               runSecretShell,
}

func init() {
	shellParser = flags.NewStandardParser(
		flags.WithStringFlag(shellFlagName, "", "", "Specify the shell to use (defaults to $SHELL, then bash, then sh)"),
		flags.WithEnvVars(shellFlagName, "ATMOS_SHELL", "SHELL"),
	)
	shellParser.RegisterFlags(shellCmd)
	if err := shellParser.BindToViper(viper.GetViper()); err != nil {
		panic(err)
	}
}

func runSecretShell(cmd *cobra.Command, _ []string) error {
	defer perf.Track(nil, "secret.runSecretShell")()

	if err := validateShellArgs(cmd); err != nil {
		return err
	}

	scope, err := parseScope(cmd)
	if err != nil {
		return err
	}

	v := viper.GetViper()
	if err := shellParser.BindFlagsToViper(cmd, v); err != nil {
		return err
	}
	shellOverride := v.GetString(shellFlagName)
	shellArgs := getSeparatedArgs(cmd)

	svc, atmosConfig, err := loadServiceAndConfigFn(scope)
	if err != nil {
		return err
	}

	env, count, err := resolveSecretEnv(svc, atmosConfig)
	if err != nil {
		return err
	}

	// Track nested-shell depth so prompts/tools can detect an Atmos-managed shell.
	atmosShellVal := shell.Level() + 1
	if err := shell.SetLevel(atmosShellVal); err != nil {
		return err
	}
	defer shell.DecrementLevel()
	env = envpkg.UpdateEnvVar(env, shell.LevelEnvVar, strconv.Itoa(atmosShellVal))

	ui.Info(fmt.Sprintf("Entering shell with %d secret(s) for component %q in stack %q (type 'exit' to return).",
		count, scope.Component, scope.Stack))

	shellCommand, shellCommandArgs := shell.Determine(shellOverride, shellArgs)
	runErr := startShellFn(shellCommand, shellCommandArgs, env)

	ui.Info(fmt.Sprintf("Exited shell for component %q in stack %q.", scope.Component, scope.Stack))
	return runErr
}

// validateShellArgs rejects positional args that aren't placed strictly after
// "--". Cobra's ArgsLenAtDash returns -1 when no "--" appears, 0 when "--" is
// the first non-flag token, and >0 when N positional args appeared before "--".
// Without this guard, getSeparatedArgs() silently drops anything before "--".
// Correct forms: `--shell bash` to choose a shell, and `-- -lc env` to pass
// shell args.
func validateShellArgs(cmd *cobra.Command) error {
	args := cmd.Flags().Args()
	dashIndex := cmd.ArgsLenAtDash()
	if len(args) > 0 && (dashIndex == -1 || dashIndex > 0) {
		return fmt.Errorf("%w: use `--shell` to choose a shell binary, and place shell args only after `--`",
			errUtils.ErrInvalidArguments)
	}
	return nil
}
