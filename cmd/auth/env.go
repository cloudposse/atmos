package auth

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/env"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/github/actions"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

const (
	// FormatFlagName is the name of the format flag for env command.
	FormatFlagName = "format"
	// OutputFileFlagName is the name of the output-file flag for env command.
	OutputFileFlagName = "output-file"
)

// SupportedFormats lists the supported output formats for env command.
// JSON is handled separately in the command, all other formats are delegated to pkg/env.
var SupportedFormats = []string{"json", "bash", "dotenv", "env", "github"}

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
		flags.WithStringFlag(FormatFlagName, "f", "bash", "Output format: bash, dotenv, env, github, json"),
		flags.WithStringFlag(OutputFileFlagName, "o", "", "Output file path (default: stdout, or $GITHUB_ENV for github format)"),
		flags.WithBoolFlag("login", "", false, "Trigger authentication if credentials are missing or expired"),
		flags.WithEnvVars(FormatFlagName, "ATMOS_AUTH_ENV_FORMAT"),
		flags.WithEnvVars(OutputFileFlagName, "ATMOS_AUTH_ENV_OUTPUT_FILE"),
		flags.WithValidValues(FormatFlagName, SupportedFormats...),
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

func executeAuthEnvCommand(cmd *cobra.Command, args []string) error {
	handleHelpRequest(cmd, args)

	defer perf.Track(nil, "auth.executeAuthEnvCommand")()

	// Bind parsed flags to Viper for precedence.
	v := viper.GetViper()
	if err := envParser.BindFlagsToViper(cmd, v); err != nil {
		return err
	}

	atmosConfig, authManager, err := loadAuthManagerForEnv(cmd, v)
	if err != nil {
		return err
	}

	// Resolve identity name (with --identity flag, viper env var, and default fallback).
	identityName, err := resolveIdentityNameForEnv(cmd, v, authManager)
	if err != nil {
		return err
	}

	// Optionally trigger authentication if credentials are missing or expired.
	if v.GetBool("login") {
		if loginErr := loginIfNeeded(cmd.Context(), authManager, identityName); loginErr != nil {
			return loginErr
		}
	}

	// Get environment variables using file-based credentials.
	envVars, err := authManager.GetEnvironmentVariables(identityName)
	if err != nil {
		return fmt.Errorf("failed to get environment variables: %w", err)
	}

	// Resolve format/output-file. github format auto-detects $GITHUB_ENV.
	format, outputFile, err := resolveEnvOutputTarget(v)
	if err != nil {
		return err
	}

	// Use unified env.Output() for all format/output combinations.
	return env.Output(
		envVars, format, outputFile,
		env.WithFileMode(env.CredentialFileMode),
		env.WithAtmosConfig(atmosConfig),
	)
}

// loadAuthManagerForEnv loads the atmos config (honouring global flags) and
// constructs the auth manager.
func loadAuthManagerForEnv(cmd *cobra.Command, v *viper.Viper) (*schema.AtmosConfiguration, auth.AuthManager, error) {
	defer perf.Track(nil, "auth.loadAuthManagerForEnv")()

	configAndStacksInfo := BuildConfigAndStacksInfo(cmd, v)
	atmosConfig, err := cfg.InitCliConfig(configAndStacksInfo, false)
	if err != nil {
		return nil, nil, fmt.Errorf(errUtils.ErrWrapFormat, errUtils.ErrFailedToInitializeAtmosConfig, err)
	}

	authManager, err := CreateAuthManager(&atmosConfig.Auth, atmosConfig.CliConfigPath)
	if err != nil {
		return nil, nil, fmt.Errorf(errUtils.ErrWrapFormat, errUtils.ErrFailedToInitializeAuthManager, err)
	}
	return &atmosConfig, authManager, nil
}

// resolveEnvOutputTarget reads --format and --output-file from Viper and
// auto-detects $GITHUB_ENV when --format=github is used without --output-file.
func resolveEnvOutputTarget(v *viper.Viper) (string, string, error) {
	format := v.GetString(FormatFlagName)
	if format == "" {
		format = "bash"
	}
	outputFile, err := resolveEnvOutputFile(format, v.GetString(OutputFileFlagName))
	if err != nil {
		return "", "", err
	}
	return format, outputFile, nil
}

// resolveIdentityNameForEnv resolves the identity from the --identity flag,
// $ATMOS_IDENTITY, or the configured default. Wraps the no-default error with
// the profile-fallback dispatcher so a stale base profile is recoverable.
func resolveIdentityNameForEnv(cmd *cobra.Command, v *viper.Viper, authManager auth.AuthManager) (string, error) {
	defer perf.Track(nil, "auth.resolveIdentityNameForEnv")()

	// Use GetIdentityFromFlags which handles Cobra's NoOptDefVal quirk correctly.
	identityName := GetIdentityFromFlags(cmd)
	if identityName == "" {
		identityName = v.GetString(IdentityFlagName)
	}

	forceSelect := identityName == IdentityFlagSelectValue
	if identityName != "" && !forceSelect {
		return identityName, nil
	}

	defaultIdentity, err := authManager.GetDefaultIdentity(forceSelect)
	if err != nil {
		wrapped := fmt.Errorf(errUtils.ErrWrapFormat, errUtils.ErrNoDefaultIdentity, err)
		return "", maybeOfferProfileFallbackOnAuthConfigError(cmd.Context(), authManager, wrapped)
	}
	return defaultIdentity, nil
}

// loginIfNeeded triggers authentication when cached credentials are absent or
// expired. Returns ErrUserAborted unwrapped on Ctrl+C/ESC.
func loginIfNeeded(ctx context.Context, authManager auth.AuthManager, identityName string) error {
	defer perf.Track(nil, "auth.env.loginIfNeeded")()

	if _, err := authManager.GetCachedCredentials(ctx, identityName); err == nil {
		return nil
	} else {
		log.Debug("No valid cached credentials found, authenticating", "identity", identityName, "error", err)
	}

	if _, err := authManager.Authenticate(ctx, identityName); err != nil {
		if errors.Is(err, errUtils.ErrUserAborted) {
			return errUtils.ErrUserAborted
		}
		return fmt.Errorf(errUtils.ErrWrapFormat, errUtils.ErrAuthenticationFailed, err)
	}
	return nil
}

// resolveEnvOutputFile fills in the destination file for `--format=github`
// when no --output-file was provided by reading $GITHUB_ENV. Other formats
// pass through unchanged.
func resolveEnvOutputFile(format, outputFile string) (string, error) {
	if format != "github" || outputFile != "" {
		return outputFile, nil
	}
	resolved := actions.GetEnvPath()
	if resolved == "" {
		return "", errUtils.Build(errUtils.ErrRequiredFlagNotProvided).
			WithExplanation("--format=github requires GITHUB_ENV environment variable to be set, or use --output-file to specify a file path.").
			Err()
	}
	return resolved, nil
}

// outputEnvAsExport outputs environment variables as shell export statements.
// Retained for unit-test coverage; production flow goes through env.Output().
func outputEnvAsExport(envVars map[string]string) error {
	defer perf.Track(nil, "auth.outputEnvAsExport")()

	keys := make([]string, 0, len(envVars))
	for k := range envVars {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, key := range keys {
		value := envVars[key]
		// Escape single quotes for safe single-quoted shell literals: ' -> '\''.
		safe := strings.ReplaceAll(value, "'", "'\\''")
		// The purpose of this command is to output credentials for shell sourcing.
		// This is intentional - similar to `aws configure export-credentials`.
		// #nosec G104 -- intentional credential output
		// codeql[go/clear-text-logging]: intentional - this command exports credentials for shell sourcing
		fmt.Printf("export %s='%s'\n", key, safe)
	}
	return nil
}

// outputEnvAsDotenv outputs environment variables in .env format.
// Retained for unit-test coverage; production flow goes through env.Output().
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
		// The purpose of this command is to output credentials for shell sourcing.
		// This is intentional - similar to `aws configure export-credentials`.
		// #nosec G104 -- intentional credential output
		// codeql[go/clear-text-logging]: intentional - this command exports credentials for shell sourcing
		fmt.Printf("%s='%s'\n", key, safe)
	}
	return nil
}
