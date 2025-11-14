package cmd

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/internal/exec"
	cfg "github.com/cloudposse/atmos/pkg/config"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
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
	defer perf.Track(nil, "cmd.executeAuthShellCommand")()

	handleHelpRequest(cmd, args)
	checkAtmosConfig()

	return executeAuthShellCommandCore(cmd, args)
}

// executeAuthShellCommandCore contains the core business logic for auth shell, separated for testability.
func executeAuthShellCommandCore(cmd *cobra.Command, args []string) error {
	defer perf.Track(nil, "cmd.executeAuthShellCommandCore")()

	// Extract Atmos flags without using pflag parser to avoid issues with "--" end-of-flags marker.
	// When DisableFlagParsing is true, manually parsing can incorrectly treat "--" as a flag value.
	identityValue, shellValue, shellArgs := extractAuthShellFlags(args)

	// Load atmos configuration (processStacks=false since auth commands don't require stack manifests)
	atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, false)
	if err != nil {
		return errors.Join(errUtils.ErrFailedToInitializeAtmosConfig, err)
	}
	atmosConfigPtr := &atmosConfig

	// Create auth manager.
	authManager, err := createAuthManager(&atmosConfig.Auth)
	if err != nil {
		return errors.Join(errUtils.ErrFailedToInitializeAuthManager, err)
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

	if identityName == "" || forceSelect {
		defaultIdentity, err := authManager.GetDefaultIdentity(forceSelect)
		if err != nil {
			return errors.Join(errUtils.ErrNoDefaultIdentity, err)
		}
		identityName = defaultIdentity
	}

	// Try to use cached credentials first (passive check, no prompts).
	// Only authenticate if cached credentials are not available or expired.
	ctx := context.Background()
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
			return errors.Join(errUtils.ErrAuthenticationFailed, err)
		}
	}

	// Prepare shell environment with file-based credentials.
	// Start with current OS environment and let PrepareShellEnvironment configure auth.
	envList, err := authManager.PrepareShellEnvironment(ctx, identityName, os.Environ())
	if err != nil {
		return fmt.Errorf("failed to prepare shell environment: %w", err)
	}

	// Get shell from extracted flag or viper.
	shell := shellValue
	if shell == "" {
		shell = viper.GetString(shellFlagName)
	}

	// Get provider name from the identity to display in shell messages.
	providerName := authManager.GetProviderForIdentity(identityName)

	// Execute the shell with authentication environment.
	// ExecAuthShellCommand expects env vars as a map, so convert the list.
	envMap := make(map[string]string)
	for _, envVar := range envList {
		if idx := strings.IndexByte(envVar, '='); idx >= 0 {
			key := envVar[:idx]
			value := envVar[idx+1:]
			envMap[key] = value
		}
	}

	return exec.ExecAuthShellCommand(atmosConfigPtr, identityName, providerName, envMap, shell, shellArgs)
}

// extractAuthShellFlags extracts --identity and --shell flags from args and returns the remaining shell args.
// This function properly handles the "--" end-of-flags marker similar to extractIdentityFlag.
func extractAuthShellFlags(args []string) (identityValue, shellValue string, shellArgs []string) {
	var identityFlagSeen bool
	var skipNext bool

	for i := 0; i < len(args); i++ {
		arg := args[i]

		// Handle skipping the next arg (it was consumed as a flag value).
		if skipNext {
			skipNext = false
			continue
		}

		// Once we see "--", everything after is shell args.
		if arg == "--" {
			// If --identity was seen but not yet assigned a value, use select value.
			if identityFlagSeen && identityValue == "" {
				identityValue = IdentityFlagSelectValue
			}
			// If --shell was seen but not yet assigned a value, leave it empty.
			// Everything after "--" is shell args.
			shellArgs = append(shellArgs, args[i+1:]...)
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

		// Check for --shell=value format.
		if strings.HasPrefix(arg, "--shell=") {
			shellValue = strings.TrimPrefix(arg, "--shell=")
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

		// Check for --shell flag.
		if arg == "--shell" {
			// Check if next arg exists and is not a flag or "--".
			if i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") && args[i+1] != "--" {
				// Next arg is the value.
				shellValue = args[i+1]
				skipNext = true
			}
			// If no value, leave shellValue empty (will use viper default).
			continue
		}

		// Not a recognized Atmos flag - treat as shell arg.
		shellArgs = append(shellArgs, arg)
	}

	// If --identity was seen but we never hit "--" and no value was set, use select value.
	if identityFlagSeen && identityValue == "" {
		identityValue = IdentityFlagSelectValue
	}

	return identityValue, shellValue, shellArgs
}

func init() {
	// NOTE: --identity flag is inherited from parent authCmd (PersistentFlags in cmd/auth.go:27).
	// DO NOT redefine it here - that would create a duplicate local flag that shadows the parent.

	authShellCmd.Flags().String(shellFlagName, "", "Specify the shell to use (defaults to $SHELL, then bash, then sh)")

	if err := viper.BindEnv(shellFlagName, "ATMOS_SHELL", "SHELL"); err != nil {
		log.Trace("Failed to bind shell environment variables", "error", err)
	}
	if err := viper.BindPFlag(shellFlagName, authShellCmd.Flags().Lookup(shellFlagName)); err != nil {
		log.Trace("Failed to bind shell flag", "error", err)
	}

	// Identity flag completion is already added by parent authCmd (cmd/auth.go:45).

	authCmd.AddCommand(authShellCmd)
}
