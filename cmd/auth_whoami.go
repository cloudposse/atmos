package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// authWhoamiCmd shows current authentication status
var authWhoamiCmd = &cobra.Command{
	Use:   "whoami",
	Short: "Show current authentication status",
	Long:  "Display information about the current effective authentication principal.",

	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: false},
	RunE: func(cmd *cobra.Command, args []string) error {
		return executeAuthWhoamiCommand(cmd, args)
	},
}

func executeAuthWhoamiCommand(cmd *cobra.Command, args []string) error {
	// Load atmos config
	atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, false)
	if err != nil {
		return fmt.Errorf("failed to load atmos config: %w", err)
	}

	// Create auth manager
	authManager, err := createAuthManager(&atmosConfig.Auth)
	if err != nil {
		return fmt.Errorf("failed to create auth manager: %w", err)
	}

	// Get default identity to check whoami status
	ctx := context.Background()
	defaultIdentity, err := authManager.GetDefaultIdentity()
	if err != nil {
		fmt.Fprintf(os.Stderr, "No default identity configured.\n")
		fmt.Fprintf(os.Stderr, "Configure auth in atmos.yaml and run `atmos auth login` to authenticate.\n")
		return nil
	}

	whoami, err := authManager.Whoami(ctx, defaultIdentity)
	if err != nil {
		fmt.Fprintf(os.Stderr, "No active authentication session found.\n")
		fmt.Fprintf(os.Stderr, "Run `atmos auth login` to authenticate.\n")
		return nil
	}

	// Check if output should be JSON
	outputFormat := viper.GetString("auth.whoami.output")
	if outputFormat == "json" {
		jsonData, err := json.MarshalIndent(whoami, "", "  ")
		if err != nil {
			return fmt.Errorf("failed to marshal JSON: %w", err)
		}
		fmt.Println(string(jsonData))
		return nil
	}

	// Display human-readable output
	fmt.Fprintf(os.Stderr, "Current Authentication Status\n\n")
	fmt.Fprintf(os.Stderr, "Provider: %s\n", whoami.Provider)
	fmt.Fprintf(os.Stderr, "Identity: %s\n", whoami.Identity)
	if whoami.Principal != "" {
		fmt.Fprintf(os.Stderr, "Principal: %s\n", whoami.Principal)
	}
	if whoami.Account != "" {
		fmt.Fprintf(os.Stderr, "Account: %s\n", whoami.Account)
	}
	if whoami.Region != "" {
		fmt.Fprintf(os.Stderr, "Region: %s\n", whoami.Region)
	}
	if whoami.Expiration != nil {
		fmt.Fprintf(os.Stderr, "Expires: %s\n", whoami.Expiration.Format("2006-01-02 15:04:05 MST"))
	}
	fmt.Fprintf(os.Stderr, "Last Updated: %s\n", whoami.LastUpdated.Format("2006-01-02 15:04:05 MST"))

	return nil
}

func init() {
	authWhoamiCmd.Flags().StringP("output", "o", "", "Output format (json)")
	_ = viper.BindPFlag("auth.whoami.output", authWhoamiCmd.Flags().Lookup("output"))
	_ = viper.BindEnv("auth.whoami.output", "ATMOS_AUTH_WHOAMI_OUTPUT")
	authCmd.AddCommand(authWhoamiCmd)
}
