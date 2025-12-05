package list

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	e "github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/flags/global"
	l "github.com/cloudposse/atmos/pkg/list"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui"
	"github.com/cloudposse/atmos/pkg/ui/theme"
	u "github.com/cloudposse/atmos/pkg/utils"
)

var componentsParser *flags.StandardParser

// ComponentsOptions contains parsed flags for the components command.
type ComponentsOptions struct {
	global.Flags
	Stack string
}

// componentsCmd lists atmos components.
var componentsCmd = &cobra.Command{
	Use:   "components",
	Short: "List all Atmos components or filter by stack",
	Long:  "List Atmos components, with options to filter results by specific stacks.",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Check Atmos configuration
		if err := checkAtmosConfig(); err != nil {
			return err
		}

		// Parse flags using StandardParser with Viper precedence
		v := viper.GetViper()
		if err := componentsParser.BindFlagsToViper(cmd, v); err != nil {
			return err
		}

		opts := &ComponentsOptions{
			Flags: flags.ParseGlobalFlags(cmd, v),
			Stack: v.GetString("stack"),
		}

		output, err := listComponentsWithOptions(cmd, opts)
		if err != nil {
			return err
		}

		if len(output) == 0 {
			ui.Info("No components found")
			return nil
		}

		u.PrintMessageInColor(strings.Join(output, "\n")+"\n", theme.Colors.Success)
		return nil
	},
}

func init() {
	// Create parser with components-specific flags using functional options
	componentsParser = flags.NewStandardParser(
		flags.WithStringFlag("stack", "s", "", "Filter by stack name or pattern"),
		flags.WithEnvVars("stack", "ATMOS_STACK"),
	)

	// Register flags
	componentsParser.RegisterFlags(componentsCmd)

	// Bind flags to Viper for environment variable support
	if err := componentsParser.BindToViper(viper.GetViper()); err != nil {
		panic(err)
	}
}

func listComponentsWithOptions(cmd *cobra.Command, opts *ComponentsOptions) ([]string, error) {
	configAndStacksInfo := schema.ConfigAndStacksInfo{}
	atmosConfig, err := config.InitCliConfig(configAndStacksInfo, true)
	if err != nil {
		return nil, fmt.Errorf("error initializing CLI config: %w", err)
	}

	// Create AuthManager for authentication support.
	authManager, err := createAuthManagerForList(cmd, &atmosConfig)
	if err != nil {
		return nil, err
	}

	stacksMap, err := e.ExecuteDescribeStacks(&atmosConfig, "", nil, nil, nil, false, false, false, false, nil, authManager)
	if err != nil {
		return nil, fmt.Errorf("error describing stacks: %w", err)
	}

	output, err := l.FilterAndListComponents(opts.Stack, stacksMap)
	return output, err
}
