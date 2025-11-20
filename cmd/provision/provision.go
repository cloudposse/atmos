package provision

import (
	"errors"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/cloudposse/atmos/cmd/internal"
	errUtils "github.com/cloudposse/atmos/errors"
	e "github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/auth"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/flags/global"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/provision"
	"github.com/cloudposse/atmos/pkg/schema"
)

var (
	// AtmosConfigPtr will be set by SetAtmosConfig before command execution.
	atmosConfigPtr *schema.AtmosConfiguration
	// ProvisionParser handles flag parsing for the provision command.
	provisionParser *flags.StandardParser
)

// ProvisionOptions contains parsed flags for the provision command.
type ProvisionOptions struct {
	global.Flags
	Stack    string
	Identity string
}

// SetAtmosConfig sets the Atmos configuration for the provision command.
// This is called from root.go after atmosConfig is initialized.
func SetAtmosConfig(config *schema.AtmosConfiguration) {
	atmosConfigPtr = config
}

// provisionCmd represents the provision command.
var provisionCmd = &cobra.Command{
	Use:   "provision backend <component> --stack <stack>",
	Short: "Provision backend infrastructure for Terraform state storage",
	Long: `Provision backend infrastructure resources using Atmos components. Currently supports provisioning
S3 backends for Terraform state storage with opinionated, secure defaults (versioning, encryption, public access blocking).

This is designed for quick setup of state backends. For production use, consider migrating to the
terraform-aws-tfstate-backend module for more control over bucket configuration.`,
	Example: `  atmos provision backend vpc --stack dev
  atmos provision backend eks --stack prod`,
	Args:                  cobra.ExactArgs(2),
	FParseErrWhitelist:    struct{ UnknownFlags bool }{UnknownFlags: false},
	DisableFlagsInUseLine: false,
	RunE: func(cmd *cobra.Command, args []string) error {
		defer perf.Track(atmosConfigPtr, "provision.RunE")()

		if len(args) != 2 {
			return errUtils.ErrInvalidArguments
		}

		provisionerType := args[0]
		component := args[1]

		// Parse flags using StandardParser with Viper precedence.
		v := viper.GetViper()
		if err := provisionParser.BindFlagsToViper(cmd, v); err != nil {
			return err
		}

		opts := &ProvisionOptions{
			Flags:    flags.ParseGlobalFlags(cmd, v),
			Stack:    v.GetString("stack"),
			Identity: v.GetString("identity"),
		}

		if opts.Stack == "" {
			return errUtils.ErrRequiredFlagNotProvided
		}

		// Load atmos configuration.
		atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{
			ComponentFromArg: component,
			Stack:            opts.Stack,
		}, false)
		if err != nil {
			return errors.Join(errUtils.ErrFailedToInitConfig, err)
		}

		// Create AuthManager from identity flag if provided.
		// Use auth.CreateAndAuthenticateManager directly to avoid import cycle with cmd package.
		var authManager auth.AuthManager
		if opts.Identity != "" {
			authManager, err = auth.CreateAndAuthenticateManager(opts.Identity, &atmosConfig.Auth, cfg.IdentityFlagSelectValue)
			if err != nil {
				return err
			}
		}

		// Create describe component function that calls internal/exec.
		describeComponent := func(component, stack string) (map[string]any, error) {
			return e.ExecuteDescribeComponent(&e.ExecuteDescribeComponentParams{
				Component:            component,
				Stack:                stack,
				ProcessTemplates:     false,
				ProcessYamlFunctions: false,
				Skip:                 nil,
				AuthManager:          authManager,
			})
		}

		// Execute provision command using pkg/provision.
		return provision.Provision(&atmosConfig, provisionerType, component, opts.Stack, describeComponent, authManager)
	},
}

func init() {
	provisionCmd.DisableFlagParsing = false

	// Create parser with provision-specific flags using functional options.
	// Note: Stack and Identity are validated in RunE to allow environment variable precedence.
	provisionParser = flags.NewStandardParser(
		flags.WithStringFlag("stack", "s", "", "Atmos stack"),
		flags.WithStringFlag("identity", "i", "", "Specify the target identity to assume. Use without value to interactively select."),
		flags.WithEnvVars("stack", "ATMOS_STACK"),
		flags.WithEnvVars("identity", "ATMOS_IDENTITY", "IDENTITY"),
	)

	// Register flags with the command.
	provisionParser.RegisterFlags(provisionCmd)

	// Set NoOptDefVal for identity flag to enable optional flag value.
	// When --identity is used without a value, it will receive cfg.IdentityFlagSelectValue.
	identityFlag := provisionCmd.Flags().Lookup("identity")
	if identityFlag != nil {
		identityFlag.NoOptDefVal = cfg.IdentityFlagSelectValue
	}

	// Bind flags to Viper for environment variable support and precedence handling.
	if err := provisionParser.BindToViper(viper.GetViper()); err != nil {
		panic(err)
	}

	// Register this command with the registry.
	// This happens during package initialization via blank import in cmd/root.go.
	internal.Register(&ProvisionCommandProvider{})
}

// ProvisionCommandProvider implements the CommandProvider interface.
type ProvisionCommandProvider struct{}

// GetCommand returns the provision command.
func (p *ProvisionCommandProvider) GetCommand() *cobra.Command {
	return provisionCmd
}

// GetName returns the command name.
func (p *ProvisionCommandProvider) GetName() string {
	return "provision"
}

// GetGroup returns the command group for help organization.
func (p *ProvisionCommandProvider) GetGroup() string {
	return "Core Stack Commands"
}

// GetAliases returns a list of command aliases to register.
// The provision command has no aliases.
func (p *ProvisionCommandProvider) GetAliases() []internal.CommandAlias {
	return nil
}
