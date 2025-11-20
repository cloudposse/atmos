package provision

import (
	"errors"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/cloudposse/atmos/cmd/internal"
	errUtils "github.com/cloudposse/atmos/errors"
	e "github.com/cloudposse/atmos/internal/exec"
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
	Stack string
}

// SetAtmosConfig sets the Atmos configuration for the provision command.
// This is called from root.go after atmosConfig is initialized.
func SetAtmosConfig(config *schema.AtmosConfiguration) {
	atmosConfigPtr = config
}

// provisionCmd represents the provision command.
var provisionCmd = &cobra.Command{
	Use:   "provision <type> <component> --stack <stack>",
	Short: "Provision infrastructure using Atmos components",
	Long: `Provision infrastructure resources using Atmos components. This command allows you to provision
different types of infrastructure (backend, component, etc.) in a specific stack.`,
	Example: `  atmos provision backend vpc --stack dev
  atmos provision component app --stack prod`,
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
			Flags: flags.ParseGlobalFlags(cmd, v),
			Stack: v.GetString("stack"),
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

		// Create describe component function that calls internal/exec.
		describeComponent := func(component, stack string) (map[string]any, error) {
			return e.ExecuteDescribeComponent(&e.ExecuteDescribeComponentParams{
				Component:            component,
				Stack:                stack,
				ProcessTemplates:     false,
				ProcessYamlFunctions: false,
				Skip:                 nil,
				AuthManager:          nil,
			})
		}

		// Execute provision command using pkg/provision.
		return provision.Provision(&atmosConfig, provisionerType, component, opts.Stack, describeComponent)
	},
}

func init() {
	provisionCmd.DisableFlagParsing = false

	// Create parser with provision-specific flags using functional options.
	// Note: Stack is validated in RunE to allow environment variable precedence.
	provisionParser = flags.NewStandardParser(
		flags.WithStringFlag("stack", "s", "", "Atmos stack"),
		flags.WithEnvVars("stack", "ATMOS_STACK"),
	)

	// Register flags with the command.
	provisionParser.RegisterFlags(provisionCmd)

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
