package auth

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth"
	"github.com/cloudposse/atmos/pkg/auth/cloud/aws/credentialprocess"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/data"
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
	// LoginFlagName is the name of the login flag for env command.
	LoginFlagName = "login"
	// FormatCredentialProcess is the format that prints AWS credential_process JSON instead of
	// environment variables. It is an alias for `atmos aws credential-process`.
	FormatCredentialProcess = "credential-process"
)

// SupportedFormats lists the supported output formats for env command.
// JSON and credential-process are handled separately in the command, all other formats are
// delegated to pkg/env.
var SupportedFormats = []string{"json", "bash", "dotenv", "env", "github", FormatCredentialProcess}

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
		flags.WithStringFlag(FormatFlagName, "f", "bash", "Output format: bash, dotenv, env, github, json, credential-process"),
		flags.WithStringFlag(OutputFileFlagName, "o", "", "Output file path (default: stdout, or $GITHUB_ENV for github format)"),
		flags.WithBoolFlag(LoginFlagName, "", false, "Trigger authentication if credentials are missing or expired"),
		flags.WithStringFlag(credentialprocess.MinValidityFlagName, "", credentialprocess.FormatMinValidity(credentialprocess.DefaultMinValidity),
			"Minimum remaining validity to reuse cached credentials (credential-process format only)"),
		flags.WithEnvVars(credentialprocess.MinValidityFlagName, credentialprocess.MinValidityEnvVar),
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

	// credential-process prints a credential document, not environment variables, and has its own
	// identity resolution and validity rules, so it branches off before the environment-variable flow.
	if v.GetString(FormatFlagName) == FormatCredentialProcess {
		return executeCredentialProcessFormat(cmd, v, authManager)
	}

	// --min-validity only affects the credential-process format; silently ignoring it would mislead.
	if cmd.Flags().Changed(credentialprocess.MinValidityFlagName) {
		return errUtils.Build(errUtils.ErrInvalidFlagValue).
			WithExplanationf("--%s only applies to --format=%s.", credentialprocess.MinValidityFlagName, FormatCredentialProcess).
			WithHintf("Add `--format=%s`, or remove `--%s`.", FormatCredentialProcess, credentialprocess.MinValidityFlagName).
			Err()
	}

	// Resolve identity name (with --identity flag, viper env var, and default fallback).
	identityName, err := resolveIdentityNameForEnv(cmd, v, authManager)
	if err != nil {
		return err
	}

	// Optionally trigger authentication if credentials are missing or expired.
	if v.GetBool(LoginFlagName) {
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

// executeCredentialProcessFormat runs `--format=credential-process`, the alias of
// `atmos aws credential-process`. It follows that command's rules: the identity must be named or be
// the configured default (the interactive selector and the profile re-exec offer are never shown,
// because the AWS CLI captures stderr and a prompt would hang it), --min-validity and
// ATMOS_AWS_CREDENTIAL_PROCESS_MIN_VALIDITY control credential reuse, and --login=false is rejected.
func executeCredentialProcessFormat(cmd *cobra.Command, v *viper.Viper, authManager auth.AuthManager) error {
	defer perf.Track(nil, "auth.executeCredentialProcessFormat")()

	// Authentication is how this format gets credentials, so --login=false contradicts it. Failing is
	// clearer than silently authenticating anyway after the user asked not to.
	if cmd.Flags().Changed(LoginFlagName) && !v.GetBool(LoginFlagName) {
		return errUtils.Build(errUtils.ErrInvalidFlagValue).
			WithExplanationf("--login=false cannot be combined with --format=%s: the format must authenticate whenever the cached credentials are missing or about to expire.", FormatCredentialProcess).
			WithHint("Remove `--login=false`. To print environment variables without authenticating, omit `--format=credential-process`.").
			Err()
	}

	minValidity, err := credentialprocess.ParseMinValidity(v.GetString(credentialprocess.MinValidityFlagName))
	if err != nil {
		return err
	}

	// Use GetIdentityFromFlags which handles Cobra's NoOptDefVal quirk correctly.
	identityName := GetIdentityFromFlags(cmd)
	if identityName == "" {
		identityName = v.GetString(IdentityFlagName)
	}
	identityName, err = credentialprocess.ResolveIdentity(authManager, identityName)
	if err != nil {
		return err
	}

	return writeCredentialProcessDocument(cmd.Context(), v, authManager, identityName, minValidity)
}

// writeCredentialProcessDocument produces the AWS credential_process document for the identity and
// writes it to --output-file (owner-readable only, replacing existing content) or stdout. The output
// is byte-identical to `atmos aws credential-process`.
func writeCredentialProcessDocument(ctx context.Context, v *viper.Viper, authManager auth.AuthManager, identityName string, minValidity time.Duration) error {
	defer perf.Track(nil, "auth.writeCredentialProcessDocument")()

	if ctx == nil {
		ctx = context.Background()
	}

	doc, err := credentialprocess.Produce(ctx, authManager, identityName, credentialprocess.WithMinValidity(minValidity))
	if err != nil {
		return err
	}

	// Not resolveEnvOutputTarget: this format never falls back to $GITHUB_ENV.
	if outputFile := v.GetString(OutputFileFlagName); outputFile != "" {
		return credentialprocess.WriteFile(outputFile, doc)
	}

	// The credentials are the purpose of this command, so masking them would make the output unusable.
	// codeql[go/clear-text-logging]: intentional credential output for the AWS credential_process protocol.
	return data.WriteUnmasked(credentialprocess.Render(doc))
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
		return errUtils.EnsureAuthenticationFailed(err)
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
		// #nosec G104 -- intentional credential output via data.WriteUnmaskedf; see its godoc.
		// codeql[go/clear-text-logging]: intentional - data.WriteUnmaskedf exports credentials for shell sourcing
		if err := data.WriteUnmaskedf("export %s='%s'\n", key, safe); err != nil {
			return err
		}
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
		// #nosec G104 -- intentional credential output via data.WriteUnmaskedf; see its godoc.
		// codeql[go/clear-text-logging]: intentional - data.WriteUnmaskedf exports credentials for shell sourcing
		if err := data.WriteUnmaskedf("%s='%s'\n", key, safe); err != nil {
			return err
		}
	}
	return nil
}
