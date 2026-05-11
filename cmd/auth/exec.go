package auth

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"syscall"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/cloudposse/atmos/cmd/internal"
	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth"
	cfg "github.com/cloudposse/atmos/pkg/config"
	envpkg "github.com/cloudposse/atmos/pkg/env"
	"github.com/cloudposse/atmos/pkg/flags"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
)

// execParser handles flags for the exec command.
var execParser *flags.StandardParser

// authExecCmd executes a command with authentication environment variables.
var authExecCmd = &cobra.Command{
	Use:   "exec -- <command> [args...]",
	Short: "Execute a command with authentication environment variables.",
	Long:  "Execute a command with the authenticated identity's environment variables set. Use `--` to separate Atmos flags from the command and its arguments.",
	Example: `  # Run terraform with the authenticated identity
  atmos auth exec -- terraform plan -var-file=env.tfvars

  # Run aws CLI with a specific identity
  atmos auth exec --identity prod -- aws s3 ls`,
	Args:               cobra.MinimumNArgs(0), // We validate after "--" separator.
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: false},
	RunE:               executeAuthExecCommand,
}

func init() {
	defer perf.Track(nil, "auth.exec.init")()

	// Create parser with no additional flags.
	// NOTE: --identity is inherited from parent authCmd as a persistent flag.
	execParser = flags.NewStandardParser()

	// Register flags with the command (none for exec, but required for consistency).
	execParser.RegisterFlags(authExecCmd)

	// Bind to Viper for environment variable support.
	if err := execParser.BindToViper(viper.GetViper()); err != nil {
		panic(err)
	}

	// Add to parent command.
	authCmd.AddCommand(authExecCmd)
}

// executeAuthExecCommand is the main execution function for auth exec command.
func executeAuthExecCommand(cmd *cobra.Command, args []string) error {
	defer perf.Track(nil, "auth.executeAuthExecCommand")()

	handleHelpRequest(cmd, args)
	// Skip stack validation since auth exec only needs auth configuration, not stack manifests.
	if err := internal.ValidateAtmosConfig(internal.WithStackValidation(false)); err != nil {
		return err
	}

	// Bind parsed flags to Viper for precedence.
	v := viper.GetViper()
	if err := execParser.BindFlagsToViper(cmd, v); err != nil {
		return err
	}

	// Get command args from SeparatedArgs (everything after "--").
	commandArgs := getSeparatedArgsForExec(cmd)

	// Validate command args before attempting authentication.
	if len(commandArgs) == 0 {
		return fmt.Errorf(errUtils.ErrWrapFormat, errUtils.ErrNoCommandSpecified, errUtils.ErrInvalidSubcommand)
	}

	// Prepare the authenticated environment.
	envList, err := prepareAuthenticatedEnv(cmd, v)
	if err != nil {
		return err
	}

	// Execute the command with authentication environment.
	return executeCommandWithEnv(commandArgs, envList)
}

// prepareAuthenticatedEnv loads config, authenticates, and prepares the shell
// environment. Returns the sanitized env list (os.Environ + global env + auth
// vars, with credential leaks removed) for direct use as the child process's
// Env so order is preserved and no duplicate-key collisions are introduced.
func prepareAuthenticatedEnv(cmd *cobra.Command, v *viper.Viper) ([]string, error) {
	defer perf.Track(nil, "auth.exec.prepareAuthenticatedEnv")()

	// Parse global flags and build ConfigAndStacksInfo to honor --base-path, --config, --config-path, --profile.
	configAndStacksInfo := BuildConfigAndStacksInfo(cmd, v)

	// Load atmos configuration (processStacks=false since auth commands don't require stack manifests).
	atmosConfig, err := cfg.InitCliConfig(configAndStacksInfo, false)
	if err != nil {
		return nil, fmt.Errorf(errUtils.ErrWrapFormat, errUtils.ErrFailedToInitializeAtmosConfig, err)
	}

	// Create auth manager.
	authManager, err := CreateAuthManager(&atmosConfig.Auth, atmosConfig.CliConfigPath)
	if err != nil {
		return nil, fmt.Errorf(errUtils.ErrWrapFormat, errUtils.ErrFailedToInitializeAuthManager, err)
	}

	// Try to use cached credentials first (passive check, no prompts).
	// Only authenticate if cached credentials are not available or expired.
	ctx := context.Background()

	// Get identity from flag or use default.
	identityName, err := resolveIdentityNameForExec(cmd, v, authManager)
	if err != nil {
		return nil, maybeOfferProfileFallbackOnAuthConfigError(ctx, authManager, err)
	}

	_, err = authManager.GetCachedCredentials(ctx, identityName)
	if err != nil {
		log.Debug("No valid cached credentials found, authenticating", "identity", identityName, "error", err)
		// No valid cached credentials - perform full authentication.
		_, err = authManager.Authenticate(ctx, identityName)
		if err != nil {
			// Check for user cancellation - return clean error without wrapping.
			if errors.Is(err, errUtils.ErrUserAborted) {
				return nil, errUtils.ErrUserAborted
			}
			return nil, fmt.Errorf(errUtils.ErrWrapFormat, errUtils.ErrAuthenticationFailed, err)
		}
	}

	// Prepare shell environment with file-based credentials.
	// Start with current OS environment + global env from atmos.yaml and let
	// PrepareShellEnvironment configure auth. The returned env list is already
	// sanitised (IRSA/credential leaks removed) — pass it through verbatim.
	baseEnv := envpkg.MergeGlobalEnv(os.Environ(), atmosConfig.Env)
	envList, err := authManager.PrepareShellEnvironment(ctx, identityName, baseEnv)
	if err != nil {
		return nil, fmt.Errorf(errUtils.ErrWrapFormat, errUtils.ErrPrepareShellEnvironment, err)
	}

	return envList, nil
}

// resolveIdentityNameForExec resolves the identity name from flags, viper, or prompts for selection.
func resolveIdentityNameForExec(cmd *cobra.Command, v *viper.Viper, authManager auth.AuthManager) (string, error) {
	defer perf.Track(nil, "auth.exec.resolveIdentityNameForExec")()

	// Get identity from flag using the helper that handles NoOptDefVal quirk.
	identityName := GetIdentityFromFlags(cmd)

	// If flag wasn't provided, check Viper for env var fallback.
	if identityName == "" {
		identityName = v.GetString(IdentityFlagName)
	}

	// Check if user wants to interactively select identity.
	forceSelect := identityName == IdentityFlagSelectValue

	if identityName == "" || forceSelect {
		defaultIdentity, err := authManager.GetDefaultIdentity(forceSelect)
		if err != nil {
			return "", fmt.Errorf(errUtils.ErrWrapFormat, errUtils.ErrNoDefaultIdentity, err)
		}
		identityName = defaultIdentity
	}

	return identityName, nil
}

// getSeparatedArgsForExec returns args after "--" separator for the exec command.
// Uses Cobra's ArgsLenAtDash() to properly handle the separator.
func getSeparatedArgsForExec(cmd *cobra.Command) []string {
	defer perf.Track(nil, "auth.exec.getSeparatedArgsForExec")()

	// Get args after command parsing.
	args := cmd.Flags().Args()
	dashIndex := cmd.Flags().ArgsLenAtDash()

	if dashIndex >= 0 && dashIndex < len(args) {
		return args[dashIndex:]
	}
	return nil
}

// executeCommandWithEnv executes a command with the sanitised environment list
// produced by prepareAuthenticatedEnv. The list already includes the full
// environment (OS env + global env + auth vars, with credential leaks removed)
// and must be passed through to the child process verbatim — do NOT prepend
// os.Environ() or convert to a map (that would lose ordering and could collide
// on duplicate keys like Windows-style drive-scoped vars).
func executeCommandWithEnv(args []string, envList []string) error {
	defer perf.Track(nil, "auth.executeCommandWithEnv")()

	if len(args) == 0 {
		return fmt.Errorf(errUtils.ErrWrapFormat, errUtils.ErrNoCommandSpecified, errUtils.ErrInvalidSubcommand)
	}

	// Prepare the command.
	cmdName := args[0]
	cmdArgs := args[1:]

	// Look for the command in PATH.
	cmdPath, err := exec.LookPath(cmdName)
	if err != nil {
		return fmt.Errorf(errUtils.ErrWrapFormat, errUtils.ErrCommandNotFound, err)
	}

	// Execute the command.
	// #nosec G204 -- This is intentional: auth exec is designed to run user-specified commands.
	execCmd := exec.Command(cmdPath, cmdArgs...)
	execCmd.Env = envList
	execCmd.Stdin = os.Stdin
	execCmd.Stdout = os.Stdout
	execCmd.Stderr = os.Stderr

	// Run the command and wait for completion.
	err = execCmd.Run()
	if err != nil {
		// If it's an exit error, propagate as a typed error so the root can exit with the same code.
		var exitError *exec.ExitError
		if errors.As(err, &exitError) {
			if status, ok := exitError.Sys().(syscall.WaitStatus); ok {
				// Return a typed error so the root can os.Exit(status.ExitStatus()).
				return errUtils.ExitCodeError{Code: status.ExitStatus()}
			}
		}
		return fmt.Errorf(errUtils.ErrWrapFormat, errUtils.ErrSubcommandFailed, err)
	}

	return nil
}
