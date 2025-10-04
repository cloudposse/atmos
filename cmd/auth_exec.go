package cmd

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"syscall"

	errUtils "github.com/cloudposse/atmos/errors"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/spf13/cobra"
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
	RunE: func(cmd *cobra.Command, args []string) error {
		handleHelpRequest(cmd, args)
		checkAtmosConfig()
		// Load atmos configuration
		atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, true)
		if err != nil {
			return fmt.Errorf(errUtils.ErrWrappingFormat, errUtils.ErrFailedToInitializeAtmosConfig, err)
		}

		// Create auth manager
		authManager, err := createAuthManager(&atmosConfig.Auth)
		if err != nil {
			return fmt.Errorf(errUtils.ErrWrappingFormat, errUtils.ErrFailedToInitializeAuthManager, err)
		}

		// Get identity from flag or use default
		identityName, _ := cmd.Flags().GetString("identity")
		if identityName == "" {
			defaultIdentity, err := authManager.GetDefaultIdentity()
			if err != nil {
				return fmt.Errorf(errUtils.ErrWrappingFormat, errUtils.ErrNoDefaultIdentity, err)
			}
			identityName = defaultIdentity
		}

		// Authenticate and get environment variables
		ctx := context.Background()
		whoami, err := authManager.Authenticate(ctx, identityName)
		if err != nil {
			return fmt.Errorf(errUtils.ErrWrappingFormat, errUtils.ErrAuthenticationFailed, err)
		}

		// Get environment variables from authentication result
		envVars := whoami.Environment
		if envVars == nil {
			envVars = make(map[string]string)
		}

		// Execute the command with authentication environment
		return executeCommandWithEnv(args, envVars)
	},
}

// executeCommandWithEnv executes a command with additional environment variables.
func executeCommandWithEnv(args []string, envVars map[string]string) error {
	if len(args) == 0 {
		return fmt.Errorf(errUtils.ErrWrappingFormat, errUtils.ErrNoCommandSpecified, errUtils.ErrInvalidSubcommand)
	}

	// Prepare the command
	cmdName := args[0]
	cmdArgs := args[1:]

	// Look for the command in PATH
	cmdPath, err := exec.LookPath(cmdName)
	if err != nil {
		return fmt.Errorf(errUtils.ErrWrappingFormat, errUtils.ErrCommandNotFound, err)
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
		// If it's an exit error, propagate as an error with exit status
		if exitError, ok := err.(*exec.ExitError); ok {
			if status, ok := exitError.Sys().(syscall.WaitStatus); ok {
				return fmt.Errorf(errUtils.ErrStringWrappingFormat, errUtils.ErrSubcommandFailed, fmt.Sprintf("exit status %d", status.ExitStatus()))
			}
		}
		return fmt.Errorf(errUtils.ErrWrappingFormat, errUtils.ErrSubcommandFailed, err)
	}

	return nil
}

func init() {
	authExecCmd.Flags().StringP("identity", "i", "", "Specify the identity to use for authentication")
	authCmd.AddCommand(authExecCmd)
}
