package auth

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth/validation"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/ui"
)

// validateParser handles flags for the validate command.
var validateParser *flags.StandardParser

// authValidateCmd validates the auth configuration.
var authValidateCmd = &cobra.Command{
	Use:   "validate",
	Short: "Validate authentication configuration",
	Long: "Validate authentication configuration for syntax and logical errors. Without --stack, validate atmos.yaml. " +
		"With --stack, validate each concrete component’s merged authentication configuration without authenticating or evaluating dynamic stack expressions.",
	Args: cobra.NoArgs,

	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: false},
	RunE:               executeAuthValidateCommand,
}

func init() {
	defer perf.Track(nil, "auth.validate.init")()

	// Create parser with validate-specific flags.
	validateParser = flags.NewStandardParser(
		flags.WithBoolFlag("verbose", "v", false, "Enable verbose output"),
		flags.WithStringFlag("stack", "s", "", "Validate merged auth for every concrete component in this stack"),
		flags.WithStringFlag("component", "c", "", "Limit scoped validation to this component (requires --stack)"),
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
		ui.MarkdownMessage("**Validating authentication configuration...**\n")
	}

	// Parse global flags and build ConfigAndStacksInfo to honor --base-path, --config, --config-path, --profile.
	configAndStacksInfo := BuildConfigAndStacksInfo(cmd, v)

	stack := v.GetString("stack")
	component := v.GetString("component")
	if component != "" && stack == "" {
		return fmt.Errorf("%w: --component requires --stack", errUtils.ErrInvalidArguments)
	}
	configAndStacksInfo.Stack = stack

	// Stack processing is only needed for explicit scoped validation.
	atmosConfig, err := cfg.InitCliConfig(configAndStacksInfo, stack != "")
	if err != nil {
		return fmt.Errorf(errUtils.ErrWrapFormat, errUtils.ErrFailedToInitializeAtmosConfig, err)
	}

	// Create validator.
	validator := validation.NewValidator()

	// Validate auth configuration.
	if stack != "" {
		err = validateStackAuth(&atmosConfig, stack, component)
	} else {
		err = validator.ValidateAuthConfig(&atmosConfig.Auth)
	}
	if err != nil {
		ui.MarkdownMessage("**❌ Authentication configuration validation failed:**\n")
		ui.MarkdownMessagef("%s\n", err.Error())
		return err
	}

	ui.MarkdownMessage("**✅ Authentication configuration is valid**\n")
	return nil
}
