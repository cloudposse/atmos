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

var stacksParser *flags.StandardParser

// StacksOptions contains parsed flags for the stacks command.
type StacksOptions struct {
	global.Flags
	Component string
}

// stacksCmd lists atmos stacks.
var stacksCmd = &cobra.Command{
	Use:   "stacks",
	Short: "List all Atmos stacks or stacks for a specific component",
	Long:  "This command lists all Atmos stacks, or filters the list to show only the stacks associated with a specified component.",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Check Atmos configuration
		if err := checkAtmosConfig(); err != nil {
			return err
		}

		// Parse flags using StandardParser with Viper precedence
		v := viper.GetViper()
		if err := stacksParser.BindFlagsToViper(cmd, v); err != nil {
			return err
		}

		opts := &StacksOptions{
			Flags:     flags.ParseGlobalFlags(cmd, v),
			Component: v.GetString("component"),
		}

		output, err := listStacksWithOptions(cmd, opts)
		if err != nil {
			return err
		}

		if len(output) == 0 {
			ui.Info("No stacks found")
			return nil
		}

		u.PrintMessageInColor(strings.Join(output, "\n")+"\n", theme.Colors.Success)
		return nil
	},
}

func init() {
	// Create parser with stacks-specific flags using functional options
	stacksParser = flags.NewStandardParser(
		flags.WithStringFlag("component", "c", "", "List all stacks that contain the specified component"),
		flags.WithEnvVars("component", "ATMOS_COMPONENT"),
	)

	// Register flags
	stacksParser.RegisterFlags(stacksCmd)

	// Bind flags to Viper for environment variable support
	if err := stacksParser.BindToViper(viper.GetViper()); err != nil {
		panic(err)
	}
}

func listStacksWithOptions(cmd *cobra.Command, opts *StacksOptions) ([]string, error) {
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

	output, err := l.FilterAndListStacks(stacksMap, opts.Component)
	return output, err
}
