package auth

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/cloudposse/atmos/pkg/auth/validation"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/perf"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// validateParser handles flags for the validate command.
var validateParser *flags.StandardParser

// authValidateCmd validates the auth configuration.
var authValidateCmd = &cobra.Command{
	Use:   "validate",
	Short: "Validate authentication configuration",
	Long:  "Validate the authentication configuration in atmos.yaml for syntax and logical errors.",

	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: false},
	RunE:               executeAuthValidateCommand,
}

func init() {
	defer perf.Track(nil, "auth.validate.init")()

	// Create parser with validate-specific flags.
	validateParser = flags.NewStandardParser(
		flags.WithBoolFlag("verbose", "v", false, "Enable verbose output"),
		flags.WithEnvVars("verbose", "ATMOS_AUTH_VALIDATE_VERBOSE"),
	)

	// Register flags with the command.
	validateParser.RegisterFlags(authValidateCmd)

	// Bind to Viper for environment variable support.
	if err := validateParser.BindToViper(viper.GetViper()); err != nil {
		panic(err)
	}

	// Add to parent command.
	authCmd.AddCommand(authValidateCmd)
}

func executeAuthValidateCommand(cmd *cobra.Command, args []string) error {
	handleHelpRequest(cmd, args)

	defer perf.Track(nil, "auth.executeAuthValidateCommand")()

	// Bind parsed flags to Viper for precedence.
	v := viper.GetViper()
	if err := validateParser.BindFlagsToViper(cmd, v); err != nil {
		return err
	}

	// Get verbose flag.
	verbose := v.GetBool("verbose")
	if verbose {
		u.PrintfMarkdown("**Validating authentication configuration...**\n")
	}

	// Parse global flags and build ConfigAndStacksInfo to honor --base-path, --config, --config-path, --profile.
	configAndStacksInfo := BuildConfigAndStacksInfo(cmd, v)

	// Load atmos config.
	atmosConfig, err := cfg.InitCliConfig(configAndStacksInfo, false)
	if err != nil {
		return fmt.Errorf("failed to load atmos config: %w", err)
	}

	// Create validator.
	validator := validation.NewValidator()

	// Validate auth configuration.
	if err := validator.ValidateAuthConfig(&atmosConfig.Auth); err != nil {
		u.PrintfMarkdown("**❌ Authentication configuration validation failed:**\n")
		u.PrintfMarkdown("%s\n", err.Error())
		return err
	}

	u.PrintfMarkdown("**✅ Authentication configuration is valid**\n")
	return nil
}
