package cmd

import (
	"context"
	_ "embed"
	"errors"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/internal/exec"
	cfg "github.com/cloudposse/atmos/pkg/config"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/schema"
)

const (
	shellFlagName = "shell"
)

//go:embed markdown/atmos_auth_shell_usage.md
var authShellUsageMarkdown string

// authShellCmd launches an interactive shell with authentication environment variables.
var authShellCmd = &cobra.Command{
	Use:   "shell",
	Short: "Launch an interactive shell with authentication environment variables.",
	Long: `The 'atmos auth shell' command authenticates with the specified identity and launches an interactive shell with the authentication environment variables configured.

In this shell, you can execute commands that require cloud credentials without needing to manually configure authentication. The shell will have all necessary environment variables (AWS_ACCESS_KEY_ID, AWS_SECRET_ACCESS_KEY, etc.) pre-configured based on your authenticated identity.`,
	Example:            authShellUsageMarkdown,
	DisableFlagParsing: true,
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: false},
	ValidArgsFunction:  cobra.NoFileCompletions,
	RunE:               executeAuthShellCommand,
}

// executeAuthShellCommand is the main execution function for auth shell command.
func executeAuthShellCommand(cmd *cobra.Command, args []string) error {
	handleHelpRequest(cmd, args)
	checkAtmosConfig()

	return executeAuthShellCommandCore(cmd, args)
}

// executeAuthShellCommandCore contains the core business logic for auth shell, separated for testability.
func executeAuthShellCommandCore(cmd *cobra.Command, args []string) error {
	// Manually parse flags since DisableFlagParsing is true.
	if err := cmd.Flags().Parse(args); err != nil {
		return fmt.Errorf("%w: %v", errUtils.ErrInvalidSubcommand, err)
	}

	// Get the non-flag arguments (shell arguments after --).
	shellArgs := cmd.Flags().Args()

	// Load atmos configuration.
	atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, true)
	if err != nil {
		return errors.Join(errUtils.ErrFailedToInitializeAtmosConfig, err)
	}

	// Create auth manager.
	authManager, err := createAuthManager(&atmosConfig.Auth)
	if err != nil {
		return errors.Join(errUtils.ErrFailedToInitializeAuthManager, err)
	}

	// Get identity from flag or use default.
	identityName, _ := cmd.Flags().GetString("identity")
	if identityName == "" {
		defaultIdentity, err := authManager.GetDefaultIdentity()
		if err != nil {
			return errors.Join(errUtils.ErrNoDefaultIdentity, err)
		}
		identityName = defaultIdentity
	}

	// Authenticate and get environment variables.
	ctx := context.Background()
	whoami, err := authManager.Authenticate(ctx, identityName)
	if err != nil {
		return errors.Join(errUtils.ErrAuthenticationFailed, err)
	}

	// Get environment variables from authentication result.
	envVars := whoami.Environment
	if envVars == nil {
		envVars = make(map[string]string)
	}

	// Get shell from flag/viper.
	shell := viper.GetString(shellFlagName)

	// Execute the shell with authentication environment.
	return exec.ExecAuthShellCommand(&atmosConfig, identityName, envVars, shell, shellArgs)
}

func init() {
	authShellCmd.Flags().StringP("identity", "i", "", "Specify the identity to use for authentication")
	authShellCmd.Flags().String(shellFlagName, "", "Specify the shell to use (defaults to $SHELL, then bash, then sh)")

	if err := viper.BindEnv(shellFlagName, "ATMOS_SHELL", "SHELL"); err != nil {
		log.Trace("Failed to bind shell environment variables", "error", err)
	}
	if err := viper.BindPFlag(shellFlagName, authShellCmd.Flags().Lookup(shellFlagName)); err != nil {
		log.Trace("Failed to bind shell flag", "error", err)
	}

	authCmd.AddCommand(authShellCmd)
}
