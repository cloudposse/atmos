package auth

import (
	"errors"
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	errUtils "github.com/cloudposse/atmos/errors"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/flags"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

const (
	// FormatFlagName is the name of the format flag for env command.
	FormatFlagName = "format"
)

// SupportedFormats lists the supported output formats for env command.
var SupportedFormats = []string{"json", "bash", "dotenv"}

// envParser handles flags for the env command.
var envParser *flags.StandardParser

// authEnvCmd exports authentication environment variables.
var authEnvCmd = &cobra.Command{
	Use:   "env",
	Short: "Export temporary cloud credentials as environment variables",
	Long:  "Outputs environment variables for the assumed identity, suitable for use by external tools such as Terraform or Helm.",

	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: false},
	RunE:               executeAuthEnvCommand,
}

func init() {
	defer perf.Track(nil, "auth.env.init")()

	// Create parser with env-specific flags.
	envParser = flags.NewStandardParser(
		flags.WithStringFlag(FormatFlagName, "f", "bash", "Output format: bash, json, dotenv"),
		flags.WithBoolFlag("login", "", false, "Trigger authentication if credentials are missing or expired"),
		flags.WithEnvVars(FormatFlagName, "ATMOS_AUTH_ENV_FORMAT"),
		flags.WithValidValues(FormatFlagName, "json", "bash", "dotenv"),
	)

	// Register flags with the command.
	envParser.RegisterFlags(authEnvCmd)

	// Bind to Viper for environment variable support.
	if err := envParser.BindToViper(viper.GetViper()); err != nil {
		panic(err)
	}

	// Register format flag completion.
	if err := authEnvCmd.RegisterFlagCompletionFunc(FormatFlagName, func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return SupportedFormats, cobra.ShellCompDirectiveNoFileComp
	}); err != nil {
		log.Trace("Failed to register format flag completion", "error", err)
	}

	// Add to parent command.
	authCmd.AddCommand(authEnvCmd)
}

//nolint:gocognit,nestif,funlen,revive,cyclop // complexity and length are reasonable for this use case
func executeAuthEnvCommand(cmd *cobra.Command, args []string) error {
	handleHelpRequest(cmd, args)

	defer perf.Track(nil, "auth.executeAuthEnvCommand")()

	// Bind parsed flags to Viper for precedence.
	v := viper.GetViper()
	if err := envParser.BindFlagsToViper(cmd, v); err != nil {
		return err
	}

	// Get output format.
	format := v.GetString(FormatFlagName)
	if !slices.Contains(SupportedFormats, format) {
		return fmt.Errorf("%w invalid format: %s", errUtils.ErrInvalidArgumentError, format)
	}

	// Get login flag.
	login := v.GetBool("login")

	// Load atmos configuration (processStacks=false since auth commands don't require stack manifests).
	atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, false)
	if err != nil {
		return fmt.Errorf("failed to load atmos config: %w", err)
	}

	// Create auth manager.
	authManager, err := CreateAuthManager(&atmosConfig.Auth)
	if err != nil {
		return fmt.Errorf("failed to create auth manager: %w", err)
	}

	// Get identity from flag or use default.
	// Use GetIdentityFromFlags which handles Cobra's NoOptDefVal quirk correctly.
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
			return fmt.Errorf("no default identity configured and no identity specified: %w", err)
		}
		identityName = defaultIdentity
	}

	// Get environment variables with optional authentication.
	var envVars map[string]string
	if login {
		// Try to use cached credentials first (passive check, no prompts).
		ctx := cmd.Context()
		if _, loginErr := authManager.GetCachedCredentials(ctx, identityName); loginErr != nil {
			// No valid cached credentials - perform full authentication.
			log.Debug("No valid cached credentials found, authenticating", "identity", identityName, "error", loginErr)
			if _, loginErr = authManager.Authenticate(ctx, identityName); loginErr != nil {
				// Check for user cancellation - return clean error without wrapping.
				if errors.Is(loginErr, errUtils.ErrUserAborted) {
					return errUtils.ErrUserAborted
				}
				// Wrap with ErrAuthenticationFailed sentinel while preserving original error.
				return fmt.Errorf(errUtils.ErrWrapFormat, errUtils.ErrAuthenticationFailed, loginErr)
			}
		}
	}

	// Get environment variables using file-based credentials.
	var envErr error
	envVars, envErr = authManager.GetEnvironmentVariables(identityName)
	if envErr != nil {
		return fmt.Errorf("failed to get environment variables: %w", envErr)
	}

	switch format {
	case "json":
		return outputEnvAsJSON(&atmosConfig, envVars)
	case "bash":
		return outputEnvAsExport(envVars)
	case "dotenv":
		return outputEnvAsDotenv(envVars)
	default:
		return outputEnvAsExport(envVars)
	}
}

// outputEnvAsJSON outputs environment variables as JSON.
func outputEnvAsJSON(atmosConfig *schema.AtmosConfiguration, envVars map[string]string) error {
	defer perf.Track(nil, "auth.outputEnvAsJSON")()

	return u.PrintAsJSON(atmosConfig, envVars)
}

// outputEnvAsExport outputs environment variables as shell export statements.
func outputEnvAsExport(envVars map[string]string) error {
	defer perf.Track(nil, "auth.outputEnvAsExport")()

	keys := make([]string, 0, len(envVars))
	for k := range envVars {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, key := range keys {
		value := envVars[key]
		// Escape single quotes for safe single-quoted shell literals: ' -> '\''
		safe := strings.ReplaceAll(value, "'", "'\\''")
		fmt.Printf("export %s='%s'\n", key, safe)
	}
	return nil
}

// outputEnvAsDotenv outputs environment variables in .env format.
func outputEnvAsDotenv(envVars map[string]string) error {
	defer perf.Track(nil, "auth.outputEnvAsDotenv")()

	keys := make([]string, 0, len(envVars))
	for k := range envVars {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, key := range keys {
		value := envVars[key]
		// Use the same safe single-quoted escaping as bash output.
		safe := strings.ReplaceAll(value, "'", "'\\''")
		fmt.Printf("%s='%s'\n", key, safe)
	}
	return nil
}
