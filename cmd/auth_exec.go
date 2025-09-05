package cmd

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"syscall"

	"github.com/cloudposse/atmos/internal/auth/cloud"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/spf13/cobra"
)

// authExecCmd executes a command with authentication environment variables
var authExecCmd = &cobra.Command{
	Use:   "exec [command] [args...]",
	Short: "Execute a command with authentication environment variables",
	Long:  "Execute a command with the authenticated identity's environment variables set.",
	Args:  cobra.MinimumNArgs(1),

	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: false},
	RunE: func(cmd *cobra.Command, args []string) error {
		// Load atmos configuration
		atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, true)
		if err != nil {
			return fmt.Errorf("failed to load atmos config: %w", err)
		}

		// Create auth manager
		authManager, err := createAuthManager(&atmosConfig.Auth)
		if err != nil {
			return fmt.Errorf("failed to create auth manager: %w", err)
		}

		// Get identity from flag or use default
		identityName, _ := cmd.Flags().GetString("identity")
		if identityName == "" {
			defaultIdentity, err := authManager.GetDefaultIdentity()
			if err != nil {
				return fmt.Errorf("no default identity configured and no identity specified: %w", err)
			}
			identityName = defaultIdentity
		}

		// Authenticate to ensure credentials are available
		ctx := context.Background()
		_, err = authManager.Authenticate(ctx, identityName)
		if err != nil {
			return fmt.Errorf("authentication failed: %w", err)
		}

		// Get the root provider for this identity (handles identity chaining correctly)
		providerName := authManager.GetProviderForIdentity(identityName)
		if providerName == "" {
			return fmt.Errorf("no provider found for identity %s", identityName)
		}

		// Get provider configuration to determine cloud provider kind
		var providerKind string
		if provider, exists := atmosConfig.Auth.Providers[providerName]; exists {
			providerKind = provider.Kind
		} else {
			// Handle AWS user identities which don't have a provider entry
			if identity, exists := atmosConfig.Auth.Identities[identityName]; exists && identity.Kind == "aws/user" {
				providerKind = "aws"
				providerName = "aws-user"
			} else {
				return fmt.Errorf("provider %s not found in configuration", providerName)
			}
		}

		// Create cloud provider manager and get environment variables
		cloudProviderManager := cloud.NewCloudProviderManager()
		envVars, err := cloudProviderManager.GetEnvironmentVariables(providerKind, providerName, identityName)
		if err != nil {
			return fmt.Errorf("failed to get environment variables: %w", err)
		}

		// Execute the command with authentication environment
		return executeCommandWithEnv(args, envVars)
	},
}

// executeCommandWithEnv executes a command with additional environment variables
func executeCommandWithEnv(args []string, envVars map[string]string) error {
	if len(args) == 0 {
		return fmt.Errorf("no command specified")
	}

	// Prepare the command
	cmdName := args[0]
	cmdArgs := args[1:]
	
	// Look for the command in PATH
	cmdPath, err := exec.LookPath(cmdName)
	if err != nil {
		return fmt.Errorf("command not found: %s", cmdName)
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
		// If it's an exit error, preserve the exit code
		if exitError, ok := err.(*exec.ExitError); ok {
			if status, ok := exitError.Sys().(syscall.WaitStatus); ok {
				os.Exit(status.ExitStatus())
			}
		}
		return fmt.Errorf("command execution failed: %w", err)
	}

	return nil
}

func init() {
	authExecCmd.Flags().StringP("identity", "i", "", "Specify the identity to use for authentication")
	authCmd.AddCommand(authExecCmd)
}
