package cmd

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	e "github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/flags"
	l "github.com/cloudposse/atmos/pkg/list"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui/theme"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// listStacksParser is created once at package initialization using builder pattern.
var listStacksParser *flags.StandardParser

// listStacksCmd lists atmos stacks
var listStacksCmd = &cobra.Command{
	Use:   "stacks",
	Short: "List all Atmos stacks or stacks for a specific component",
	Long:  "This command lists all Atmos stacks, or filters the list to show only the stacks associated with a specified component.",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Check Atmos configuration
		checkAtmosConfig()

		output, err := listStacks(cmd, args)
		if err != nil {
			return err
		}

		u.PrintMessageInColor(strings.Join(output, "\n")+"\n", theme.Colors.Success)
		return nil
	},
}

func init() {
	// Create parser with builder pattern - compile-time type safety!
	listStacksParser = flags.NewStandardOptionsBuilder().
		WithComponent(false). // Optional component flag → .Component field
		Build()

	listStacksCmd.DisableFlagParsing = false
	listStacksParser.RegisterFlags(listStacksCmd)
	_ = listStacksParser.BindToViper(viper.GetViper())
	listCmd.AddCommand(listStacksCmd)
}

func listStacks(cmd *cobra.Command, args []string) ([]string, error) {
	// Parse flags with Viper precedence (CLI > ENV > config > defaults)
	v := viper.New()
	_ = listStacksParser.BindFlagsToViper(cmd, v)

	// Parse command-line arguments and get strongly-typed options
	opts, err := listStacksParser.Parse(context.Background(), args)
	if err != nil {
		return nil, err
	}

	configAndStacksInfo := schema.ConfigAndStacksInfo{}

	atmosConfig, err := config.InitCliConfig(configAndStacksInfo, true)
	if err != nil {
		return nil, fmt.Errorf("error initializing CLI config: %v", err)
	}

	stacksMap, err := e.ExecuteDescribeStacks(&atmosConfig, "", nil, nil, nil, false, false, false, false, nil, nil)
	if err != nil {
		return nil, fmt.Errorf("error describing stacks: %v", err)
	}

	output, err := l.FilterAndListStacks(stacksMap, opts.Component)
	return output, err
}
