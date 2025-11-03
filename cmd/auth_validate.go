package cmd

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/cloudposse/atmos/pkg/auth/validation"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

var authValidateParser = flags.NewStandardOptionsBuilder().
	WithVerbose().
	Build()

// authValidateCmd validates the auth configuration.
var authValidateCmd = &cobra.Command{
	Use:   "validate",
	Short: "Validate authentication configuration",
	Long:  "Validate the authentication configuration in atmos.yaml for syntax and logical errors.",
	RunE:  executeAuthValidateCommand,
}

func executeAuthValidateCommand(cmd *cobra.Command, args []string) error {
	handleHelpRequest(cmd, args)

	// Parse flags using StandardOptions.
	opts, err := authValidateParser.Parse(context.Background(), args)
	if err != nil {
		return err
	}

	if opts.Verbose {
		u.PrintfMarkdown("**Validating authentication configuration...**\n")
	}

	// Load atmos config.
	atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, false)
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

func init() {
	// Register StandardOptions flags.
	authValidateParser.RegisterFlags(authValidateCmd)
	_ = authValidateParser.BindToViper(viper.GetViper())

	authCmd.AddCommand(authValidateCmd)
}
