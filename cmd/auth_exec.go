package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	errUtils "github.com/cloudposse/atmos/errors"
	cfg "github.com/cloudposse/atmos/pkg/config"
	envpkg "github.com/cloudposse/atmos/pkg/env"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui"
)

// authExecCmd executes a command with authentication environment variables.
var authExecCmd = &cobra.Command{
	Use:   "exec",
	Short: "Execute a command with authentication environment variables.",
	Long:  "Execute a command with the authenticated identity's environment variables set. Use `--` to separate Atmos flags from the command's native arguments.",
	Example: `  # Run terraform with the authenticated identity
  atmos auth exec -- terraform plan -var-file=env.tfvars`,
	Args:               cobra.MinimumNArgs(1),
	DisableFlagParsing: true,

	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: false},
	RunE:               executeAuthExecCommand,
}

// executeAuthExecCommand is the main execution function for auth exec command.
func executeAuthExecCommand(cmd *cobra.Command, args []string) error {
	handleHelpRequest(cmd, args)
	// Skip stack validation since auth exec only needs auth configuration, not stack manifests.
	checkAtmosConfig(WithStackValidation(false))

	return executeAuthExecCommandCore(cmd, args)
}

// executeAuthExecCommandCore contains the core business logic for auth exec, separated for testability.
func executeAuthExecCommandCore(cmd *cobra.Command, args []string) error {
	// Extract Atmos flags without using pflag parser to avoid issues with "--" end-of-flags marker.
	// When DisableFlagParsing is true, manually parsing can incorrectly treat "--" as a flag value.
	identityValue, commandArgs := extractIdentityFlag(args)

	// Validate command args before attempting authentication.
	if len(commandArgs) == 0 {
		return fmt.Errorf(errUtils.ErrWrapFormat, errUtils.ErrNoCommandSpecified, errUtils.ErrInvalidSubcommand)
	}

	// Load atmos configuration (processStacks=false since auth commands don't require stack manifests)
	atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, false)
	if err != nil {
		return fmt.Errorf(errUtils.ErrWrapFormat, errUtils.ErrFailedToInitializeAtmosConfig, err)
	}

	// Create auth manager
	authManager, err := createAuthManager(&atmosConfig.Auth, atmosConfig.CliConfigPath)
	if err != nil {
		return fmt.Errorf(errUtils.ErrWrapFormat, errUtils.ErrFailedToInitializeAuthManager, err)
	}

	// Get identity from extracted flag or use default.
	// identityValue will be:
	// - "" if --identity was not provided
	// - IdentityFlagSelectValue if --identity was provided without a value
	// - the actual value if --identity=value or --identity value was provided
	var identityName string
	if identityValue != "" {
		// Flag was explicitly provided on command line.
		identityName = identityValue
	} else {
		// Flag not provided on command line - fall back to viper (config/env).
		identityName = viper.GetString(IdentityFlagName)
	}

	// Check if user wants to interactively select identity.
	forceSelect := identityName == IdentityFlagSelectValue

	ctx := context.Background()
	if identityName == "" || forceSelect {
		defaultIdentity, err := authManager.GetDefaultIdentity(forceSelect)
		if err != nil {
			wrapped := fmt.Errorf(errUtils.ErrWrapFormat, errUtils.ErrNoDefaultIdentity, err)
			return maybeOfferProfileFallbackOnAuthConfigError(ctx, authManager, wrapped)
		}
		identityName = defaultIdentity
	}

	// Try to use cached credentials first (passive check, no prompts).
	// Only authenticate if cached credentials are not available or expired.
	_, err = authManager.GetCachedCredentials(ctx, identityName)
	if err != nil {
		log.Debug("No valid cached credentials found, authenticating", "identity", identityName, "error", err)
		// No valid cached credentials - perform full authentication.
		_, err = authManager.Authenticate(ctx, identityName)
		if err != nil {
			// Check for user cancellation - return clean error without wrapping.
			if errors.Is(err, errUtils.ErrUserAborted) {
				return errUtils.ErrUserAborted
			}
			return fmt.Errorf(errUtils.ErrWrapFormat, errUtils.ErrAuthenticationFailed, err)
		}
	}

	// Prepare shell environment with file-based credentials.
	// Start with current OS environment + global env from atmos.yaml and let PrepareShellEnvironment configure auth.
	// PrepareShellEnvironment sanitizes the env (removes IRSA/credential vars) and adds auth vars.
	baseEnv := envpkg.MergeGlobalEnv(os.Environ(), atmosConfig.Env)
	envList, err := authManager.PrepareShellEnvironment(ctx, identityName, baseEnv)
	if err != nil {
		return fmt.Errorf("failed to prepare command environment: %w", err)
	}

	// Execute the command with the sanitized environment directly.
	// The envList already includes os.Environ() (sanitized) + auth vars,
	// so we pass it as the complete subprocess environment.
	err = executeCommandWithEnv(commandArgs, envList)
	if err != nil {
		// For any subprocess error, provide a tip about refreshing credentials.
		// This helps users when AWS tokens are expired or invalid.
		printAuthExecTip(identityName)
		return err
	}
	return nil
}

// executeCommandWithEnv executes a command with a complete environment.
// The env parameter should be a fully prepared environment (e.g., from PrepareShellEnvironment).
// It is used directly as the subprocess environment without re-reading os.Environ().
func executeCommandWithEnv(args []string, env []string) error {
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

	// Execute the command with the provided environment directly.
	execCmd := exec.Command(cmdPath, cmdArgs...)
	execCmd.Env = env
	execCmd.Stdin = os.Stdin
	execCmd.Stdout = os.Stdout
	execCmd.Stderr = os.Stderr

	// Run the command and wait for completion
	err = execCmd.Run()
	if err != nil {
		// If it's an exit error, propagate as a typed error so the root can exit with the same code.
		if exitError, ok := err.(*exec.ExitError); ok {
			if status, ok := exitError.Sys().(syscall.WaitStatus); ok {
				// Return a typed error so the root can os.Exit(status.ExitStatus()).
				return errUtils.ExitCodeError{Code: status.ExitStatus()}
			}
		}
		return fmt.Errorf(errUtils.ErrWrapFormat, errUtils.ErrSubcommandFailed, err)
	}

	return nil
}

// extractIdentityFlag extracts the --identity flag value from args and returns the remaining command args.
// This function properly handles the "--" end-of-flags marker:
// - "--identity value -- cmd" -> identityValue="value", commandArgs=["cmd"].
// - "--identity -- cmd" -> identityValue=IdentityFlagSelectValue (user wants interactive selection), commandArgs=["cmd"].
// - "--identity" -> identityValue=IdentityFlagSelectValue, commandArgs=[].
// - "-- cmd" -> identityValue="", commandArgs=["cmd"].
// - "cmd" -> identityValue="", commandArgs=["cmd"].
func extractIdentityFlag(args []string) (identityValue string, commandArgs []string) {
	var identityFlagSeen bool
	var skipNext bool

	for i := 0; i < len(args); i++ {
		arg := args[i]

		// Handle skipping the next arg (it was consumed as a flag value).
		if skipNext {
			skipNext = false
			continue
		}

		// Once we see "--", everything after is command args.
		if arg == "--" {
			// If --identity was seen but not yet assigned a value, use select value.
			if identityFlagSeen && identityValue == "" {
				identityValue = IdentityFlagSelectValue
			}
			// Everything after "--" is command args.
			commandArgs = append(commandArgs, args[i+1:]...)
			break
		}

		// Check for --identity=value format.
		if strings.HasPrefix(arg, "--identity=") {
			identityValue = strings.TrimPrefix(arg, "--identity=")
			if identityValue == "" {
				identityValue = IdentityFlagSelectValue
			}
			identityFlagSeen = true
			continue
		}

		// Check for --identity or -i flag.
		if arg == "--identity" || arg == "-i" {
			identityFlagSeen = true
			// Check if next arg exists and is not a flag or "--".
			if i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") && args[i+1] != "--" {
				// Next arg is the value.
				identityValue = args[i+1]
				skipNext = true
			} else {
				// No value provided - user wants interactive selection.
				identityValue = IdentityFlagSelectValue
			}
			continue
		}

		// Not a recognized Atmos flag - treat as command arg.
		commandArgs = append(commandArgs, arg)
	}

	// If --identity was seen but we never hit "--" and no value was set, use select value.
	if identityFlagSeen && identityValue == "" {
		identityValue = IdentityFlagSelectValue
	}

	return identityValue, commandArgs
}

// printAuthExecTip prints a helpful tip when auth exec fails.
func printAuthExecTip(identityName string) {
	ui.Writeln("")
	ui.Info("Tip: If credentials are expired, refresh with:")
	ui.Writef("     atmos auth login --identity %s\n", identityName)
}

func init() {
	// NOTE: --identity flag is inherited from parent authCmd (PersistentFlags in cmd/auth.go:27).
	// DO NOT redefine it here - that would create a duplicate local flag that shadows the parent.
	// Identity flag completion is already added by parent authCmd (cmd/auth.go:45).
	authCmd.AddCommand(authExecCmd)
}
