package cmd

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/cloudposse/atmos/internal/auth"
	"github.com/cloudposse/atmos/internal/auth/config"
	"github.com/cloudposse/atmos/internal/auth/credentials"
	"github.com/cloudposse/atmos/internal/auth/environment"
	"github.com/cloudposse/atmos/internal/auth/validation"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
	"github.com/spf13/cobra"
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

	// Get current authentication status
	ctx := context.Background()
	whoami, err := authManager.Whoami(ctx)
	if err != nil {
		u.PrintfMarkdown("**No active authentication session found**\n")
		u.PrintfMarkdown("Run `atmos auth login` to authenticate.\n")
		return nil
	}

	// Check if output should be JSON
	outputFormat, _ := cmd.Flags().GetString("output")
	if outputFormat == "json" {
		jsonData, err := json.MarshalIndent(whoami, "", "  ")
		if err != nil {
			return fmt.Errorf("failed to marshal JSON: %w", err)
		}
		fmt.Println(string(jsonData))
		return nil
	}

	// Display human-readable output
	u.PrintfMarkdown("**Current Authentication Status**\n\n")
	u.PrintfMarkdown("**Provider:** %s\n", whoami.Provider)
	u.PrintfMarkdown("**Identity:** %s\n", whoami.Identity)
	if whoami.Principal != "" {
		u.PrintfMarkdown("**Principal:** %s\n", whoami.Principal)
	}
	if whoami.Account != "" {
		u.PrintfMarkdown("**Account:** %s\n", whoami.Account)
	}
	if whoami.Region != "" {
		u.PrintfMarkdown("**Region:** %s\n", whoami.Region)
	}
	if whoami.Expiration != nil {
		u.PrintfMarkdown("**Expires:** %s\n", whoami.Expiration.Format("2006-01-02 15:04:05 MST"))
	}
	u.PrintfMarkdown("**Last Updated:** %s\n", whoami.LastUpdated.Format("2006-01-02 15:04:05 MST"))

	return nil
}

func init() {
	authWhoamiCmd.Flags().StringP("output", "o", "", "Output format (json)")
	authCmd.AddCommand(authWhoamiCmd)
}
