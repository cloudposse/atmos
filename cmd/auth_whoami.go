package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/config/go-homedir"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var ErrNoActiveAuthSession = errors.New("no active authentication session found")

// authWhoamiCmd shows current authentication status.
var authWhoamiCmd = &cobra.Command{
	Use:   "whoami",
	Short: "Show current authentication status",
	Long:  "Display information about the current effective authentication principal.",

	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: false},
	RunE:               executeAuthWhoamiCommand,
}

func executeAuthWhoamiCommand(cmd *cobra.Command, args []string) error {
	handleHelpRequest(cmd, args)

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

	// Get identity from flag or use default
	identityName, _ := cmd.Flags().GetString("identity")
	if identityName == "" {
		defaultIdentity, err := authManager.GetDefaultIdentity()
		if err != nil {
			return fmt.Errorf("no default identity configured and no identity specified: %w", err)
		}
		identityName = defaultIdentity
	}

	if err != nil {
		fmt.Fprintf(os.Stderr, "No default identity configured.\n")
		fmt.Fprintf(os.Stderr, "Configure auth in atmos.yaml and run `atmos auth login` to authenticate.\n")
		return err
	}

	whoami, err := authManager.Whoami(ctx, identityName)
	if err != nil {
		errUtils.CheckErrorPrintAndExit(err, "", "")
	}

	// Check if output should be JSON
	outputFormat := viper.GetString("auth.whoami.output")
	if outputFormat == "json" {
		// Redact home directory in environment variable values before output
		redactedWhoami := *whoami
		homeDir, _ := homedir.Dir()
		if whoami.Environment != nil && homeDir != "" {
			redactedEnv := make(map[string]string, len(whoami.Environment))
			for k, v := range whoami.Environment {
				redactedEnv[k] = redactHomeDir(v, homeDir)
			}
			redactedWhoami.Environment = redactedEnv
		}
		jsonData, err := json.MarshalIndent(redactedWhoami, "", "  ")
		if err != nil {
			errUtils.CheckErrorAndPrint(errUtils.ErrInvalidAuthConfig, "Failed to marshal JSON", "")
			return errUtils.ErrInvalidAuthConfig
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

// redactHomeDir replaces occurrences of the homeDir at the start of v with "~" to avoid leaking user paths.
func redactHomeDir(v string, homeDir string) string {
	if homeDir == "" {
		return v
	}
	// Ensure both have the same path separator
	if strings.HasPrefix(v, homeDir+string(os.PathSeparator)) {
		return "~" + v[len(homeDir):]
	}
	if v == homeDir {
		return "~"
	}
	return v
}

func init() {
	authWhoamiCmd.Flags().StringP("output", "o", "", "Output format (json)")
	_ = viper.BindPFlag("auth.whoami.output", authWhoamiCmd.Flags().Lookup("output"))
	_ = viper.BindEnv("auth.whoami.output", "ATMOS_AUTH_WHOAMI_OUTPUT")
	authCmd.AddCommand(authWhoamiCmd)
}
