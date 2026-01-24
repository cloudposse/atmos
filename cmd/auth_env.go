package cmd

import (
	"errors"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	errUtils "github.com/cloudposse/atmos/errors"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/env"
	"github.com/cloudposse/atmos/pkg/github/actions"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/schema"
)

// authEnvCmd exports authentication environment variables.
var authEnvCmd = &cobra.Command{
	Use:   "env",
	Short: "Export temporary cloud credentials as environment variables",
	Long:  "Outputs environment variables for the assumed identity, suitable for use by external tools such as Terraform or Helm.",

	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: false},
	RunE: func(cmd *cobra.Command, args []string) error {
		// Get output format and file from Viper (honors CLI > ENV > config > defaults).
		formatStr := viper.GetString("auth_env_format")
		if formatStr == "" {
			formatStr = "bash"
		}
		outputFile := viper.GetString("auth_env_output_file")

		// Get login flag.
		login, _ := cmd.Flags().GetBool("login")

		// Load atmos configuration (processStacks=false since auth commands don't require stack manifests).
		atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, false)
		if err != nil {
			return fmt.Errorf("failed to load atmos config: %w", err)
		}

		// Create auth manager.
		authManager, err := createAuthManager(&atmosConfig.Auth)
		if err != nil {
			return fmt.Errorf("failed to create auth manager: %w", err)
		}

		// Get identity from flag or use default.
		// Use GetIdentityFromFlags which handles Cobra's NoOptDefVal quirk correctly.
		identityName := GetIdentityFromFlags(cmd, os.Args)

		// Check if user wants to interactively select identity.
		forceSelect := identityName == IdentityFlagSelectValue

		if identityName == "" || forceSelect {
			defaultIdentity, err := authManager.GetDefaultIdentity(forceSelect)
			if err != nil {
				return fmt.Errorf("no default identity configured and no identity specified: %w", err)
			}
			identityName = defaultIdentity
		}

		var envVars map[string]string

		if login {
			// Try to use cached credentials first (passive check, no prompts).
			// Only authenticate if cached credentials are not available or expired.
			ctx := cmd.Context()
			_, err := authManager.GetCachedCredentials(ctx, identityName)
			if err != nil {
				log.Debug("No valid cached credentials found, authenticating", "identity", identityName, "error", err)
				// No valid cached credentials - perform full authentication.
				_, err = authManager.Authenticate(ctx, identityName)
				if err != nil {
					// Check for user cancellation - return clean error without wrapping.
					if errors.Is(err, errUtils.ErrUserAborted) {
						return errUtils.ErrUserAborted
					}
					// Wrap with ErrAuthenticationFailed sentinel while preserving original error.
					return fmt.Errorf(errUtils.ErrWrapFormat, errUtils.ErrAuthenticationFailed, err)
				}
			}

			// Get environment variables using file-based credentials.
			// This ensures we use valid credential files rather than potentially expired keyring credentials.
			envVars, err = authManager.GetEnvironmentVariables(identityName)
			if err != nil {
				return fmt.Errorf("failed to get environment variables: %w", err)
			}
		} else {
			// Get environment variables WITHOUT authentication/validation.
			// This allows users to see what environment variables would be set
			// even if they don't have valid credentials yet.
			envVars, err = authManager.GetEnvironmentVariables(identityName)
			if err != nil {
				return fmt.Errorf("failed to get environment variables: %w", err)
			}
		}

		// Handle GitHub format special case (requires file output).
		if formatStr == "github" && outputFile == "" {
			// Auto-detect GITHUB_ENV in GitHub Actions environment.
			outputFile = actions.GetEnvPath()
			if outputFile == "" {
				return errUtils.Build(errUtils.ErrRequiredFlagNotProvided).
					WithExplanation("--format=github requires GITHUB_ENV environment variable to be set, or use --output-file to specify a file path.").
					Err()
			}
		}

		// Use unified env.Output() for all format/output combinations.
		return env.Output(envVars, formatStr, outputFile,
			env.WithFileMode(env.CredentialFileMode),
			env.WithAtmosConfig(&atmosConfig),
		)
	},
}

func init() {
	authEnvCmd.Flags().StringP("format", "f", "bash", "Output format: bash, dotenv, github, json")
	authEnvCmd.Flags().StringP("output-file", "o", "", "Output file path (default: stdout, or $GITHUB_ENV for github format)")
	authEnvCmd.Flags().Bool("login", false, "Trigger authentication if credentials are missing or expired (default: false)")

	if err := viper.BindEnv("auth_env_format", "ATMOS_AUTH_ENV_FORMAT"); err != nil {
		log.Trace("Failed to bind auth_env_format environment variable", "error", err)
	}
	if err := viper.BindPFlag("auth_env_format", authEnvCmd.Flags().Lookup("format")); err != nil {
		log.Trace("Failed to bind auth_env_format flag", "error", err)
	}
	if err := viper.BindEnv("auth_env_output_file", "ATMOS_AUTH_ENV_OUTPUT_FILE"); err != nil {
		log.Trace("Failed to bind auth_env_output_file environment variable", "error", err)
	}
	if err := viper.BindPFlag("auth_env_output_file", authEnvCmd.Flags().Lookup("output-file")); err != nil {
		log.Trace("Failed to bind auth_env_output_file flag", "error", err)
	}

	// Register format completion using pkg/env supported formats plus JSON.
	// JSON is handled separately in the command, not via pkg/env.
	formatCompletions := make([]string, 0, len(env.SupportedFormats)+1)
	formatCompletions = append(formatCompletions, "json")
	for _, f := range env.SupportedFormats {
		formatCompletions = append(formatCompletions, string(f))
	}
	if err := authEnvCmd.RegisterFlagCompletionFunc("format", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return formatCompletions, cobra.ShellCompDirectiveNoFileComp
	}); err != nil {
		log.Trace("Failed to register format flag completion", "error", err)
	}

	// DO NOT bind identity flag to Viper - it breaks flag precedence.
	// Identity flag binding is handled in cmd/auth.go via BindEnv only.

	authCmd.AddCommand(authEnvCmd)
}
