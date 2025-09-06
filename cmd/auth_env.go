package cmd

import (
	"context"
	"fmt"

	"github.com/cloudposse/atmos/internal/auth/cloud"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
	"github.com/spf13/cobra"
)

// authEnvCmd exports authentication environment variables
var authEnvCmd = &cobra.Command{
	Use:   "env",
	Short: "Export authentication environment variables",
	Long:  "Export environment variables for the authenticated identity to use with external tools.",

	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: false},
	RunE: func(cmd *cobra.Command, args []string) error {
		// Load atmos configuration
		atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, true)
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

		// Authenticate to ensure credentials are available
		ctx := context.Background()
		_, err = authManager.Authenticate(ctx, identityName)
		if err != nil {
			return fmt.Errorf("authentication failed: %w", err)
		}

		// Get the root provider for this identity (handles identity chaining correctly)
		providerName := authManager.GetProviderForIdentity(identityName)
		if providerName == "" {
			return fmt.Errorf("no provider found for identity %s", identityName)
		}

		// Get provider kind from the identity (eliminates special case handling)
		providerKind, err := authManager.GetProviderKindForIdentity(identityName)
		if err != nil {
			return fmt.Errorf("failed to get provider kind for identity %s: %w", identityName, err)
		}

		// Create cloud provider manager and get environment variables
		cloudProviderManager := cloud.NewCloudProviderManager()
		envVars, err := cloudProviderManager.GetEnvironmentVariables(providerKind, providerName, identityName)
		if err != nil {
			return fmt.Errorf("failed to get environment variables: %w", err)
		}

		// Get output format
		format, _ := cmd.Flags().GetString("format")

		switch format {
		case "json":
			return outputEnvAsJSON(envVars)
		case "export":
			return outputEnvAsExport(envVars)
		case "dotenv":
			return outputEnvAsDotenv(envVars)
		default:
			return outputEnvAsExport(envVars)
		}
	},
}

// outputEnvAsJSON outputs environment variables as JSON
func outputEnvAsJSON(envVars map[string]string) error {
	return u.PrintAsJSON(nil, envVars)
}

// outputEnvAsExport outputs environment variables as shell export statements
func outputEnvAsExport(envVars map[string]string) error {
	for key, value := range envVars {
		fmt.Printf("export %s=\"%s\"\n", key, value)
	}
	return nil
}

// outputEnvAsDotenv outputs environment variables in .env format
func outputEnvAsDotenv(envVars map[string]string) error {
	for key, value := range envVars {
		fmt.Printf("%s=%s\n", key, value)
	}
	return nil
}

func init() {
	authEnvCmd.Flags().StringP("format", "f", "export", "Output format: export, json, dotenv")
	authCmd.AddCommand(authEnvCmd)
}
