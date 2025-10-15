package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/cloudposse/atmos/pkg/auth/validation"
	cfg "github.com/cloudposse/atmos/pkg/config"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// authValidateCmd validates the auth configuration.
var authValidateCmd = &cobra.Command{
	Use:   "validate",
	Short: "Validate authentication configuration",
	Long:  "Validate the authentication configuration in atmos.yaml for syntax and logical errors.",

	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: false},
	RunE:               executeAuthValidateCommand,
}

func executeAuthValidateCommand(cmd *cobra.Command, args []string) error {
	if err := handleHelpRequest(cmd, args); err != nil {
		return err
	}
	// Get verbose flag
	verbose := viper.GetBool("auth.validate.verbose")
	if verbose {
		u.PrintfMarkdown("**Validating authentication configuration...**\n")
	}

	// Load atmos config
	atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, false)
	if err != nil {
		return fmt.Errorf("failed to load atmos config: %w", err)
	}

	// Create validator
	validator := validation.NewValidator()

	// Validate auth configuration
	if err := validator.ValidateAuthConfig(&atmosConfig.Auth); err != nil {
		u.PrintfMarkdown("**❌ Authentication configuration validation failed:**\n")
		u.PrintfMarkdown("%s\n", err.Error())
		return err
	}

	u.PrintfMarkdown("**✅ Authentication configuration is valid**\n")
	return nil
}

func init() {
	authValidateCmd.Flags().BoolP("verbose", "v", false, "Enable verbose output")
	if err := viper.BindPFlag("auth.validate.verbose", authValidateCmd.Flags().Lookup("verbose")); err != nil {
		log.Trace("Failed to bind auth.validate.verbose flag", "error", err)
	}
	viper.SetEnvPrefix("ATMOS")
	if err := viper.BindEnv("auth.validate.verbose"); err != nil {
		log.Trace("Failed to bind auth.validate.verbose environment variable", "error", err)
	}
	authCmd.AddCommand(authValidateCmd)
}
