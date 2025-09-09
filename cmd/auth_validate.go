package cmd

import (
	"fmt"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/internal/auth/validation"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
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
	handleHelpRequest(cmd, args)
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
		return errUtils.ErrInvalidAuthConfig
	}

	u.PrintfMarkdown("**✅ Authentication configuration is valid**\n")
	return nil
}

func init() {
	authValidateCmd.Flags().BoolP("verbose", "v", false, "Enable verbose output")
	_ = viper.BindPFlag("auth.validate.verbose", authValidateCmd.Flags().Lookup("verbose"))
	viper.SetEnvPrefix("ATMOS")
	_ = viper.BindEnv("auth.validate.verbose") // ATMOS_AUTH_VALIDATE_VERBOSE
	authCmd.AddCommand(authValidateCmd)
}
