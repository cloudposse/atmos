package auth

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/cloudposse/atmos/cmd/internal"
	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/auth"
	cfg "github.com/cloudposse/atmos/pkg/config"
	envpkg "github.com/cloudposse/atmos/pkg/env"
	"github.com/cloudposse/atmos/pkg/flags"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

const (
	shellFlagName = "shell"
)

// shellParser handles flags for the shell command.
var shellParser *flags.StandardParser

//go:embed markdown/atmos_auth_shell_usage.md
var authShellUsageMarkdown string

// authShellCmd launches an interactive shell with authentication environment variables.
var authShellCmd = &cobra.Command{
	Use:   "shell [-- [shell args...]]",
	Short: "Launch an interactive shell with authentication environment variables.",
	Long: `The 'atmos auth shell' command authenticates with the specified identity and launches an interactive shell with the authentication environment variables configured.

In this shell, you can execute commands that require cloud credentials without needing to manually configure authentication. The shell will have all necessary environment variables (AWS_ACCESS_KEY_ID, AWS_SECRET_ACCESS_KEY, etc.) pre-configured based on your authenticated identity.

Use "--" to separate Atmos flags from shell arguments that should be passed through.`,
	Example:            authShellUsageMarkdown,
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: false},
	ValidArgsFunction:  cobra.NoFileCompletions,
	RunE:               executeAuthShellCommand,
}

func init() {
	defer perf.Track(nil, "auth.shell.init")()

	// Create parser with shell-specific flags.
	// NOTE: --identity is inherited from parent authCmd as a persistent flag.
	shellParser = flags.NewStandardParser(
		flags.WithStringFlag(shellFlagName, "", "", "Specify the shell to use (defaults to $SHELL, then bash, then sh)"),
		flags.WithEnvVars(shellFlagName, "ATMOS_SHELL", "SHELL"),
	)

	// Register flags with the command.
	shellParser.RegisterFlags(authShellCmd)

	// Bind to Viper for environment variable support.
	if err := shellParser.BindToViper(viper.GetViper()); err != nil {
		panic(err)
	}

	// Add to parent command.
	authCmd.AddCommand(authShellCmd)
}

// executeAuthShellCommand is the main execution function for auth shell command.
func executeAuthShellCommand(cmd *cobra.Command, args []string) error {
	defer perf.Track(nil, "auth.executeAuthShellCommand")()

	handleHelpRequest(cmd, args)
	if err := internal.ValidateAtmosConfig(); err != nil {
		return err
	}

	// Bind parsed flags to Viper for precedence.
	v := viper.GetViper()
	if err := shellParser.BindFlagsToViper(cmd, v); err != nil {
		return err
	}

	// Get shell args from SeparatedArgs (everything after "--").
	// Cobra's ArgsLenAtDash() tells us where "--" was, and ParseFlags populates this.
	shellArgs := getSeparatedArgs(cmd)

	// Parse global flags and build ConfigAndStacksInfo to honor --base-path, --config, --config-path, --profile.
	configAndStacksInfo := BuildConfigAndStacksInfo(cmd, v)

	// Load atmos configuration (processStacks=false since auth commands don't require stack manifests).
	atmosConfig, err := cfg.InitCliConfig(configAndStacksInfo, false)
	if err != nil {
		return fmt.Errorf(errUtils.ErrWrapFormat, errUtils.ErrFailedToInitializeAtmosConfig, err)
	}
	atmosConfigPtr := &atmosConfig

	// Create auth manager.
	authManager, err := CreateAuthManager(&atmosConfig.Auth)
	if err != nil {
		return fmt.Errorf(errUtils.ErrWrapFormat, errUtils.ErrFailedToInitializeAuthManager, err)
	}

	// Get identity from flag or use default.
	identityName, err := resolveIdentityNameForShell(cmd, v, authManager)
	if err != nil {
		return err
	}

	// Authenticate and prepare shell environment.
	shell := v.GetString(shellFlagName)
	envMap, providerName, err := prepareShellEnvironment(authManager, identityName, atmosConfigPtr)
	if err != nil {
		return err
	}

	return exec.ExecAuthShellCommand(atmosConfigPtr, identityName, providerName, envMap, shell, shellArgs)
}

// resolveIdentityNameForShell resolves the identity name from flags, viper, or prompts for selection.
func resolveIdentityNameForShell(cmd *cobra.Command, v *viper.Viper, authManager auth.AuthManager) (string, error) {
	defer perf.Track(nil, "auth.shell.resolveIdentityNameForShell")()

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

// prepareShellEnvironment authenticates and prepares the shell environment.
func prepareShellEnvironment(authManager auth.AuthManager, identityName string, atmosConfig *schema.AtmosConfiguration) (map[string]string, string, error) {
	defer perf.Track(nil, "auth.shell.prepareShellEnvironment")()

	// Try to use cached credentials first (passive check, no prompts).
	// Only authenticate if cached credentials are not available or expired.
	ctx := context.Background()
	_, err := authManager.GetCachedCredentials(ctx, identityName)
	if err != nil {
		log.Debug("No valid cached credentials found, authenticating", "identity", identityName, "error", err)
		// No valid cached credentials - perform full authentication.
		_, err = authManager.Authenticate(ctx, identityName)
		if err != nil {
			// Check for user cancellation - return clean error without wrapping.
			if errors.Is(err, errUtils.ErrUserAborted) {
				return nil, "", errUtils.ErrUserAborted
			}
			return nil, "", fmt.Errorf(errUtils.ErrWrapFormat, errUtils.ErrAuthenticationFailed, err)
		}
	}

	// Prepare shell environment with file-based credentials.
	// Start with current OS environment + global env from atmos.yaml (consistent with exec.go).
	baseEnv := envpkg.MergeGlobalEnv(os.Environ(), atmosConfig.Env)
	envList, err := authManager.PrepareShellEnvironment(ctx, identityName, baseEnv)
	if err != nil {
		return nil, "", fmt.Errorf("failed to prepare shell environment: %w", err)
	}

	// Get provider name from the identity to display in shell messages.
	providerName := authManager.GetProviderForIdentity(identityName)

	// Convert environment list to map.
	envMap := make(map[string]string)
	for _, envVar := range envList {
		if idx := strings.IndexByte(envVar, '='); idx >= 0 {
			key := envVar[:idx]
			value := envVar[idx+1:]
			envMap[key] = value
		}
	}

	return envMap, providerName, nil
}

// getSeparatedArgs returns args after "--" separator.
// Uses Cobra's ArgsLenAtDash() to properly handle the separator.
func getSeparatedArgs(cmd *cobra.Command) []string {
	defer perf.Track(nil, "auth.shell.getSeparatedArgs")()

	// Get args after command parsing.
	args := cmd.Flags().Args()
	dashIndex := cmd.Flags().ArgsLenAtDash()

	if dashIndex >= 0 && dashIndex < len(args) {
		return args[dashIndex:]
	}
	return nil
}
