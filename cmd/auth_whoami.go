package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	homedir "github.com/mitchellh/go-homedir"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	errUtils "github.com/cloudposse/atmos/errors"
	authTypes "github.com/cloudposse/atmos/pkg/auth/types"
	cfg "github.com/cloudposse/atmos/pkg/config"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/schema"
)

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

	// Load atmos config and auth manager.
	authManager, err := loadAuthManager()
	if err != nil {
		return err
	}

	// Determine identity.
	identityName, err := identityFromFlagOrDefault(cmd, authManager)
	if err != nil {
		return err
	}

	// Query whoami.
	whoami, err := authManager.Whoami(context.Background(), identityName)
	if err != nil {
		errUtils.CheckErrorPrintAndExit(err, "", "")
	}

	// Output.
	if viper.GetString("auth.whoami.output") == "json" {
		return printWhoamiJSON(whoami)
	}
	printWhoamiHuman(whoami)
	return nil
}

func loadAuthManager() (authTypes.AuthManager, error) {
	atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, false)
	if err != nil {
		return nil, fmt.Errorf("%w: failed to load atmos config: %v", errUtils.ErrInvalidAuthConfig, err)
	}
	manager, err := createAuthManager(&atmosConfig.Auth)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", errUtils.ErrInvalidAuthConfig, err)
	}
	return manager, nil
}

func identityFromFlagOrDefault(cmd *cobra.Command, authManager authTypes.AuthManager) (string, error) {
	identityName, _ := cmd.Flags().GetString("identity")
	if identityName != "" {
		return identityName, nil
	}
	defaultIdentity, err := authManager.GetDefaultIdentity()
	if err != nil {
		return "", fmt.Errorf("%w: no default identity configured and no identity specified: %v", errUtils.ErrInvalidAuthConfig, err)
	}
	return defaultIdentity, nil
}

func printWhoamiJSON(whoami *authTypes.WhoamiInfo) error {
	// Redact home directory in environment variable values before output.
	redactedWhoami := *whoami
	// Never emit credentials in JSON output.
	redactedWhoami.Credentials = nil
	homeDir, _ := homedir.Dir()
	if whoami.Environment != nil {
		redactedWhoami.Environment = sanitizeEnvMap(whoami.Environment, homeDir)
	}
	jsonData, err := json.MarshalIndent(redactedWhoami, "", "  ")
	if err != nil {
		errUtils.CheckErrorAndPrint(errUtils.ErrInvalidAuthConfig, "Failed to marshal JSON", "")
		return errUtils.ErrInvalidAuthConfig
	}
	fmt.Println(string(jsonData))
	return nil
}

func printWhoamiHuman(whoami *authTypes.WhoamiInfo) {
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
}

// redactHomeDir replaces occurrences of the homeDir at the start of v with "~" to avoid leaking user paths.
func redactHomeDir(v string, homeDir string) string {
	if homeDir == "" {
		return v
	}
	// Ensure both have the same path separator.
	if strings.HasPrefix(v, homeDir+string(os.PathSeparator)) {
		return "~" + v[len(homeDir):]
	}
	if v == homeDir {
		return "~"
	}
	return v
}

// sanitizeEnvMap redacts secrets and user paths from environment variables.
func sanitizeEnvMap(in map[string]string, homeDir string) map[string]string {
	out := make(map[string]string, len(in))
	sensitive := []string{"SECRET", "TOKEN", "PASSWORD", "PRIVATE", "ACCESS_KEY", "SESSION"}
	for k, v := range in {
		redacted := redactHomeDir(v, homeDir)
		upperK := strings.ToUpper(k)
		for _, s := range sensitive {
			if strings.Contains(upperK, s) {
				redacted = "***REDACTED***"
				break
			}
		}
		out[k] = redacted
	}
	return out
}

func init() {
	authWhoamiCmd.Flags().StringP("output", "o", "", "Output format (json)")
	if err := viper.BindPFlag("auth.whoami.output", authWhoamiCmd.Flags().Lookup("output")); err != nil {
		log.Trace("Failed to bind auth.whoami.output flag", "error", err)
	}
	if err := viper.BindEnv("auth.whoami.output", "ATMOS_AUTH_WHOAMI_OUTPUT"); err != nil {
		log.Trace("Failed to bind auth.whoami.output environment variable", "error", err)
	}
	if err := authWhoamiCmd.RegisterFlagCompletionFunc("output", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return []string{"json"}, cobra.ShellCompDirectiveNoFileComp
	}); err != nil {
		log.Trace("Failed to register output flag completion", "error", err)
	}
	authCmd.AddCommand(authWhoamiCmd)
}
