package cmd

import (
	"context"
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	errUtils "github.com/cloudposse/atmos/errors"
	cfg "github.com/cloudposse/atmos/pkg/config"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

var SupportedFormats = []string{"json", "bash", "dotenv"}

// authEnvCmd exports authentication environment variables.
var authEnvCmd = &cobra.Command{
	Use:   "env",
	Short: "Export temporary cloud credentials as environment variables",
	Long:  "Outputs environment variables for the assumed identity, suitable for use by external tools such as Terraform or Helm.",

	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: false},
	RunE: func(cmd *cobra.Command, args []string) error {
		// Get output format
		format, _ := cmd.Flags().GetString("format")
		if !slices.Contains(SupportedFormats, format) {
			return fmt.Errorf("%w invalid format: %s", errUtils.ErrInvalidArgumentError, format)
		}

		// Load atmos configuration (processStacks=false since auth commands don't require stack manifests)
		atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, false)
		if err != nil {
			return fmt.Errorf("failed to load atmos config: %w", err)
		}

		// Create auth manager
		authManager, err := createAuthManager(&atmosConfig.Auth)
		if err != nil {
			return fmt.Errorf("failed to create auth manager: %w", err)
		}

		// Get identity from flag or use default
		identityName, _ := cmd.Flags().GetString("identity")
		if identityName == "" {
			defaultIdentity, err := authManager.GetDefaultIdentity()
			if err != nil {
				return fmt.Errorf("no default identity configured and no identity specified: %w", err)
			}
			identityName = defaultIdentity
		}

		// Authenticate and get environment variables
		ctx := context.Background()
		whoami, err := authManager.Authenticate(ctx, identityName)
		if err != nil {
			return fmt.Errorf("authentication failed: %w", err)
		}

		// Get environment variables from authentication result
		envVars := whoami.Environment
		if envVars == nil {
			envVars = make(map[string]string)
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
		// Use the same safe single-quoted escaping as bash output
		safe := strings.ReplaceAll(value, "'", "'\\''")
		fmt.Printf("%s='%s'\n", key, safe)
	}
	return nil
}

func init() {
	authEnvCmd.Flags().StringP("format", "f", "bash", "Output format: bash, json, dotenv.")
	if err := viper.BindEnv("auth_env_format", "ATMOS_AUTH_ENV_FORMAT"); err != nil {
		log.Trace("Failed to bind auth_env_format environment variable", "error", err)
	}
	if err := viper.BindPFlag("auth_env_format", authEnvCmd.Flags().Lookup("format")); err != nil {
		log.Trace("Failed to bind auth_env_format flag", "error", err)
	}
	if err := authEnvCmd.RegisterFlagCompletionFunc("format", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return SupportedFormats, cobra.ShellCompDirectiveNoFileComp
	}); err != nil {
		log.Trace("Failed to register format flag completion", "error", err)
	}

	if err := viper.BindPFlag("identity", authCmd.PersistentFlags().Lookup("identity")); err != nil {
		log.Trace("Failed to bind identity flag", "error", err)
	}
	viper.MustBindEnv("identity", "ATMOS_IDENTITY")
	authCmd.AddCommand(authEnvCmd)
}
