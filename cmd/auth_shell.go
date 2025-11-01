package cmd

import (
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
var authShellParser *flagparser.PassThroughFlagParser

func init() {
	// Create parser with identity and shell flags.
	authShellParser = flagparser.NewPassThroughFlagParser(
		flagparser.WithStringFlag("identity", "i", "", "Specify the target identity to assume. Use without value to interactively select."),
		flagparser.WithStringFlag("shell", "s", "", "Specify the shell to launch (default: $SHELL or /bin/sh)"),
	)

	// Set NoOptDefVal for identity flag to support --identity without value.
	registry := authShellParser.GetRegistry()
	if identityFlag := registry.Get("identity"); identityFlag != nil {
		if sf, ok := identityFlag.(*flagparser.StringFlag); ok {
			sf.NoOptDefVal = cfg.IdentityFlagSelectValue
		}
	}
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
	parsedConfig, err := authShellParser.Parse(ctx, args)
	if err != nil {
		return err
	}

	// Get identity from parsed config.
	var identityValue string
	if id, ok := parsedConfig.AtmosFlags["identity"]; ok {
		if idStr, ok := id.(string); ok {
			identityValue = idStr
		}
	}

	// Get shell from parsed config.
	var shellValue string
	if sh, ok := parsedConfig.AtmosFlags["shell"]; ok {
		if shStr, ok := sh.(string); ok {
			shellValue = shStr
		}
	}

	// Shell args are everything that's not an Atmos flag.
	shellArgs := parsedConfig.PassThroughArgs

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
