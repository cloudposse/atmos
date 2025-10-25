package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"syscall"

	"github.com/spf13/cobra"

	errUtils "github.com/cloudposse/atmos/errors"
	cfg "github.com/cloudposse/atmos/pkg/config"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/schema"
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
	checkAtmosConfig()

	return executeAuthExecCommandCore(cmd, args)
}

// executeAuthExecCommandCore contains the core business logic for auth exec, separated for testability.
func executeAuthExecCommandCore(cmd *cobra.Command, args []string) error {
	// Manually parse flags since DisableFlagParsing is true.
	if err := cmd.Flags().Parse(args); err != nil {
		return fmt.Errorf("%w: %v", errUtils.ErrInvalidSubcommand, err)
	}
	// Get the non-flag arguments (the actual command to execute).
	commandArgs := cmd.Flags().Args()

	// Validate command args before attempting authentication.
	if len(commandArgs) == 0 {
		return errors.Join(errUtils.ErrNoCommandSpecified, errUtils.ErrInvalidSubcommand)
	}

	// Load atmos configuration
	atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, true)
	if err != nil {
		return errors.Join(errUtils.ErrFailedToInitializeAtmosConfig, err)
	}

	// Create auth manager
	authManager, err := createAuthManager(&atmosConfig.Auth)
	if err != nil {
		return errors.Join(errUtils.ErrFailedToInitializeAuthManager, err)
	}

	// Get identity from flag or use default
	identityName, _ := cmd.Flags().GetString("identity")
	if identityName == "" {
		defaultIdentity, err := authManager.GetDefaultIdentity()
		if err != nil {
			return errors.Join(errUtils.ErrNoDefaultIdentity, err)
		}
		identityName = defaultIdentity
	}

	// Try to use cached credentials first (passive check, no prompts).
	// Only authenticate if cached credentials are not available or expired.
	ctx := context.Background()
	whoami, err := authManager.GetCachedCredentials(ctx, identityName)
	if err != nil {
		log.Debug("No valid cached credentials found, authenticating", "identity", identityName, "error", err)
		// No valid cached credentials - perform full authentication.
		whoami, err = authManager.Authenticate(ctx, identityName)
		if err != nil {
			// Check for user cancellation - return clean error without wrapping.
			if errors.Is(err, errUtils.ErrUserAborted) {
				return errUtils.ErrUserAborted
			}
			return errors.Join(errUtils.ErrAuthenticationFailed, err)
		}
	}

	// Get environment variables from authentication result
	envVars := whoami.Environment
	if envVars == nil {
		envVars = make(map[string]string)
	}

	// Execute the command with authentication environment
	return executeCommandWithEnv(commandArgs, envVars)
}

// executeCommandWithEnv executes a command with additional environment variables.
func executeCommandWithEnv(args []string, envVars map[string]string) error {
	if len(args) == 0 {
		return errors.Join(errUtils.ErrNoCommandSpecified, errUtils.ErrInvalidSubcommand)
	}

	// Prepare the command
	cmdName := args[0]
	cmdArgs := args[1:]

	// Look for the command in PATH
	cmdPath, err := exec.LookPath(cmdName)
	if err != nil {
		return errors.Join(errUtils.ErrCommandNotFound, err)
	}

	// Prepare environment variables
	env := os.Environ()
	for key, value := range envVars {
		env = append(env, fmt.Sprintf("%s=%s", key, value))
	}

	// Execute the command
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
		return errors.Join(errUtils.ErrSubcommandFailed, err)
	}

	return nil
}

func init() {
	authExecCmd.Flags().StringP("identity", "i", "", "Specify the identity to use for authentication")
	AddIdentityCompletion(authExecCmd)
	authCmd.AddCommand(authExecCmd)
}
