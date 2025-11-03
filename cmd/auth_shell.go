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
	"github.com/cloudposse/atmos/pkg/flagparser"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

const (
	shellFlagName = "shell"
)

//go:embed markdown/atmos_auth_shell_usage.md
var authShellUsageMarkdown string

// authShellParser handles flag parsing for auth shell command.
var authShellParser *flagparser.AuthParser

func init() {
	// Create parser with identity and shell flags.
	// Returns strongly-typed AuthInterpreter.
	authShellParser = flagparser.NewAuthShellParser()
}

// authShellCmd launches an interactive shell with authentication environment variables.
var authShellCmd = &cobra.Command{
	Use:   "shell",
	Short: "Launch an interactive shell with authentication environment variables.",
	Long: `The 'atmos auth shell' command authenticates with the specified identity and launches an interactive shell with the authentication environment variables configured.

In this shell, you can execute commands that require cloud credentials without needing to manually configure authentication. The shell will have all necessary environment variables (AWS_ACCESS_KEY_ID, AWS_SECRET_ACCESS_KEY, etc.) pre-configured based on your authenticated identity.`,
	Example:           authShellUsageMarkdown,
	Args:              cobra.ArbitraryArgs,
	ValidArgsFunction: cobra.NoFileCompletions,
	RunE:              executeAuthShellCommand,
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

	// Parse args with flagparser to extract --identity, --shell, and shell args.
	ctx := cmd.Context()
	if ctx == nil {
		ctx = context.Background()
	}
	interpreter, err := authShellParser.Parse(ctx, args)
	if err != nil {
		return err
	}

	// Get identity and shell from strongly-typed interpreter.
	identityValue := interpreter.Identity.Value()
	shellValue := interpreter.Shell

	// Shell args are everything that's not an Atmos flag.
	shellArgs := interpreter.GetPassThroughArgs()

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

	// Handle --identity flag for interactive selection.
	// If identity is "__SELECT__", prompt for interactive selection.
	var identityName string
	if interpreter.Identity.IsInteractiveSelector() || interpreter.Identity.IsEmpty() {
		forceSelect := interpreter.Identity.IsInteractiveSelector()
		defaultIdentity, err := authManager.GetDefaultIdentity(forceSelect)
		if err != nil {
			return errors.Join(errUtils.ErrNoDefaultIdentity, err)
		}
		identityName = defaultIdentity
	} else {
		identityName = identityValue
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
			return errors.Join(errUtils.ErrAuthenticationFailed, err)
		}
	}

	// Prepare shell environment with file-based credentials.
	// Start with current OS environment and let PrepareShellEnvironment configure auth.
	envList, err := authManager.PrepareShellEnvironment(ctx, identityName, os.Environ())
	if err != nil {
		return fmt.Errorf("failed to prepare shell environment: %w", err)
	}

	// Use shell from interpreter (already resolved via Viper precedence).
	shell := shellValue

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

func init() {
	// Register Atmos flags with Cobra using our parser.
	// This replaces the manual extractAuthShellFlags() approach.
	authShellParser.RegisterFlags(authShellCmd)
	_ = authShellParser.BindToViper(viper.GetViper())

	// Set NoOptDefVal on the registered identity flag to support --identity without value.
	if identityFlag := authShellCmd.Flags().Lookup("identity"); identityFlag != nil {
		identityFlag.NoOptDefVal = cfg.IdentityFlagSelectValue
	}

	authCmd.AddCommand(authShellCmd)
}
