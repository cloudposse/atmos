package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"slices"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	errUtils "github.com/cloudposse/atmos/errors"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/flags"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

var SupportedFormats = []string{"json", "bash", "dotenv"}

var authEnvParser = flags.NewStandardOptionsBuilder().
	WithFormat("bash").
	WithLogin().
	Build()

// authEnvCmd exports authentication environment variables.
var authEnvCmd = &cobra.Command{
	Use:   "env",
	Short: "Export temporary cloud credentials as environment variables",
	Long:  "Outputs environment variables for the assumed identity, suitable for use by external tools such as Terraform or Helm.",

	RunE: func(cmd *cobra.Command, args []string) error {
		// Parse flags using StandardOptions.
		opts, err := authEnvParser.Parse(context.Background(), args)
		if err != nil {
			return err
		}

		// Resolve format (use default if empty).
		format := opts.Format
		if format == "" {
			format = "bash" // Default format
		}

		// Validate format.
		if !slices.Contains(SupportedFormats, format) {
			return fmt.Errorf("%w invalid format: %s", errUtils.ErrInvalidArgumentError, format)
		}

		// Store resolved format back for use below.
		opts.Format = format

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

		if opts.Login {
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
					return fmt.Errorf("%w: %w", errUtils.ErrAuthenticationFailed, err)
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

		switch opts.Format {
		case "json":
			return outputEnvAsJSON(&atmosConfig, envVars)
		case "bash":
			return outputEnvAsExport(envVars)
		case "dotenv":
			return outputEnvAsDotenv(envVars)
		default:
			return outputEnvAsExport(envVars)
		}
	},
}

// outputEnvAsJSON outputs environment variables as JSON.
func outputEnvAsJSON(atmosConfig *schema.AtmosConfiguration, envVars map[string]string) error {
	return u.PrintAsJSON(atmosConfig, envVars)
}

// outputEnvAsExport outputs environment variables as shell export statements.
func outputEnvAsExport(envVars map[string]string) error {
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

func init() {
	// Register StandardOptions flags.
	authEnvParser.RegisterFlags(authEnvCmd)
	_ = authEnvParser.BindToViper(viper.GetViper())

	// Register flag completion for format.
	if err := authEnvCmd.RegisterFlagCompletionFunc("format", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return SupportedFormats, cobra.ShellCompDirectiveNoFileComp
	}); err != nil {
		log.Trace("Failed to register format flag completion", "error", err)
	}

	// DO NOT bind identity flag to Viper - it breaks flag precedence.
	// Identity flag binding is handled in cmd/auth.go via BindEnv only.

	authCmd.AddCommand(authEnvCmd)
}
