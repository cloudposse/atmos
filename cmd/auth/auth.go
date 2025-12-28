package auth

import (
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/cloudposse/atmos/cmd/auth/user"
	"github.com/cloudposse/atmos/cmd/internal"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/flags/compat"
	"github.com/cloudposse/atmos/pkg/perf"
)

const (
	// IdentityFlagName is the name of the identity flag.
	IdentityFlagName = "identity"
	// IdentityFlagSelectValue is imported from cfg.IdentityFlagSelectValue.
	IdentityFlagSelectValue = cfg.IdentityFlagSelectValue
)

// authParser handles persistent flags for auth command.
var authParser *flags.StandardParser

// authCmd groups authentication-related subcommands.
var authCmd = &cobra.Command{
	Use:                "auth",
	Short:              "Authenticate with cloud providers and identity services.",
	Long:               "Obtain, refresh, and configure credentials from external identity providers such as AWS SSO, Vault, or OIDC. Provides the necessary authentication context for tools like Terraform and Helm to interact with cloud infrastructure.",
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: false},
	ValidArgsFunction:  ComponentsArgCompletion,
}

func init() {
	defer perf.Track(nil, "auth.init")()

	// Create parser with persistent identity flag.
	authParser = flags.NewStandardParser(
		flags.WithStringFlag(IdentityFlagName, "i", "", "Specify the target identity to assume. Use without value to interactively select."),
		flags.WithNoOptDefVal(IdentityFlagName, IdentityFlagSelectValue),
		flags.WithEnvVars(IdentityFlagName, "ATMOS_IDENTITY", "IDENTITY"),
	)

	// Register as persistent flags (inherited by subcommands).
	authParser.RegisterPersistentFlags(authCmd)

	// Bind to Viper for environment variable support.
	if err := authParser.BindToViper(viper.GetViper()); err != nil {
		panic(err)
	}

	// Add identity completion.
	AddIdentityCompletion(authCmd)

	// Add user subcommand (nested command structure).
	authCmd.AddCommand(user.AuthUserCmd)

	// Register this command with the registry.
	internal.Register(&AuthCommandProvider{})
}

// AuthCommandProvider implements the CommandProvider interface.
type AuthCommandProvider struct{}

// GetCommand returns the auth command.
func (a *AuthCommandProvider) GetCommand() *cobra.Command {
	return authCmd
}

// GetName returns the command name.
func (a *AuthCommandProvider) GetName() string {
	return "auth"
}

// GetGroup returns the command group for help organization.
func (a *AuthCommandProvider) GetGroup() string {
	return "Pro Features"
}

// GetFlagsBuilder returns the flags builder for this command.
func (a *AuthCommandProvider) GetFlagsBuilder() flags.Builder {
	return authParser
}

// GetPositionalArgsBuilder returns the positional args builder for this command.
func (a *AuthCommandProvider) GetPositionalArgsBuilder() *flags.PositionalArgsBuilder {
	return nil
}

// GetCompatibilityFlags returns compatibility flags for this command.
func (a *AuthCommandProvider) GetCompatibilityFlags() map[string]compat.CompatibilityFlag {
	return nil
}

// GetAliases returns command aliases.
func (a *AuthCommandProvider) GetAliases() []internal.CommandAlias {
	return nil
}

// GetIdentityFromFlags retrieves the identity flag value, handling the NoOptDefVal quirk.
// The identity flag uses NoOptDefVal="__SELECT__" which causes Viper to always return "__SELECT__"
// after parsing, even when the flag wasn't provided. This function checks the actual flag state
// to return the correct value.
//
// Returns:
//   - Empty string if --identity was not provided (use config/env default).
//   - IdentityFlagSelectValue if --identity was provided without a value (user wants selection).
//   - The actual value if --identity=value was provided.
func GetIdentityFromFlags(cmd *cobra.Command) string {
	defer perf.Track(nil, "auth.GetIdentityFromFlags")()

	flag := cmd.Flags().Lookup(IdentityFlagName)
	if flag == nil {
		// Check persistent flags if not found in local flags.
		flag = cmd.InheritedFlags().Lookup(IdentityFlagName)
	}

	if flag == nil || !flag.Changed {
		// Flag was not explicitly set on command line.
		// Return empty string to signal "use default from config/env".
		return ""
	}

	// Flag was explicitly set - return its value.
	// This could be:
	// - The actual value (--identity=prod or --identity prod).
	// - IdentityFlagSelectValue (--identity without value).
	return flag.Value.String()
}

// GetAuthCmd returns the auth command for subcommand registration.
func GetAuthCmd() *cobra.Command {
	return authCmd
}
