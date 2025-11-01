package cmd

import (
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
	"github.com/cloudposse/atmos/pkg/flagparser"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/schema"
)

// authExecParser handles flag parsing for auth exec command.
var authExecParser *flagparser.PassThroughFlagParser

func init() {
	// Create parser with identity flag only (auth exec doesn't use stack/component flags).
	authExecParser = flagparser.NewPassThroughFlagParser(
		flagparser.WithStringFlag("identity", "i", "", "Specify the target identity to assume. Use without value to interactively select."),
	)

	// Set NoOptDefVal for identity flag to support --identity without value.
	registry := authExecParser.GetRegistry()
	if identityFlag := registry.Get("identity"); identityFlag != nil {
		if sf, ok := identityFlag.(*flagparser.StringFlag); ok {
			sf.NoOptDefVal = cfg.IdentityFlagSelectValue
		}
	}
}

// authExecCmd executes a command with authentication environment variables.
var authExecCmd = &cobra.Command{
	Use:   "exec",
	Short: "Execute a command with authentication environment variables.",
	Long:  "Execute a command with the authenticated identity's environment variables set. Use `--` to separate Atmos flags from the command's native arguments.",
	Example: `  # Run terraform with the authenticated identity
  atmos auth exec -- terraform plan -var-file=env.tfvars`,
	Args: cobra.ArbitraryArgs,
	RunE: executeAuthExecCommand,
}

// executeAuthExecCommand is the main execution function for auth exec command.
func executeAuthExecCommand(cmd *cobra.Command, args []string) error {
	handleHelpRequest(cmd, args)
	checkAtmosConfig()

	return executeAuthExecCommandCore(cmd, args)
}

// executeAuthExecCommandCore contains the core business logic for auth exec, separated for testability.
func executeAuthExecCommandCore(cmd *cobra.Command, args []string) error {
	// Parse args with flagparser to extract --identity and command args.
	ctx := cmd.Context()
	parsedConfig, err := authExecParser.Parse(ctx, args)
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

	// Command args are everything that's not an Atmos flag.
	commandArgs := parsedConfig.PassThroughArgs

	// Validate command args before attempting authentication.
	if len(commandArgs) == 0 {
		return errors.Join(errUtils.ErrNoCommandSpecified, errUtils.ErrInvalidSubcommand)
	}

	// Load atmos configuration (processStacks=false since auth commands don't require stack manifests)
	atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, false)
	if err != nil {
		return errors.Join(errUtils.ErrFailedToInitializeAtmosConfig, err)
	}

	// Create auth manager
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
		return fmt.Errorf("failed to prepare command environment: %w", err)
	}

	// Convert environment list to map for executeCommandWithEnv.
	envMap := make(map[string]string)
	for _, envVar := range envList {
		if idx := strings.IndexByte(envVar, '='); idx >= 0 {
			key := envVar[:idx]
			value := envVar[idx+1:]
			envMap[key] = value
		}
	}

	// Execute the command with authentication environment
	return executeCommandWithEnv(commandArgs, envMap)
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

func init() {
	// Register Atmos flags with Cobra using our parser.
	// This replaces the manual extractIdentityFlag() approach.
	authExecParser.RegisterFlags(authExecCmd)
	_ = authExecParser.BindToViper(viper.GetViper())

	// Set NoOptDefVal on the registered flag to support --identity without value.
	if identityFlag := authExecCmd.Flags().Lookup("identity"); identityFlag != nil {
		identityFlag.NoOptDefVal = cfg.IdentityFlagSelectValue
	}

	authCmd.AddCommand(authExecCmd)
}
