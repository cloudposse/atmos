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

var listComponentsParser = flags.NewStandardOptionsBuilder().
	WithStack(false).
	Build()

// listComponentsCmd lists atmos components.
var listComponentsCmd = &cobra.Command{
	Use:   "components",
	Short: "List all Atmos components or filter by stack",
	Long:  "List Atmos components, with options to filter results by specific stacks.",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Check Atmos configuration.
		checkAtmosConfig()

		// Parse flags using StandardOptions.
		opts, err := listComponentsParser.Parse(context.Background(), args)
		if err != nil {
			return err
		}

		configAndStacksInfo := schema.ConfigAndStacksInfo{}
		atmosConfig, err := config.InitCliConfig(configAndStacksInfo, true)
		if err != nil {
			return fmt.Errorf("error initializing CLI config: %v", err)
		}

		stacksMap, err := e.ExecuteDescribeStacks(&atmosConfig, "", nil, nil, nil, true, true, false, false, nil, nil)
		if err != nil {
			return fmt.Errorf("error describing stacks: %v", err)
		}

		output, err := l.FilterAndListComponents(opts.Stack, stacksMap)
		if err != nil {
			return err
		}

		u.PrintMessageInColor(strings.Join(output, "\n")+"\n", theme.Colors.Success)
		return nil
	},
}

func init() {
	// Register StandardOptions flags.
	listComponentsParser.RegisterFlags(listComponentsCmd)
	_ = listComponentsParser.BindToViper(viper.GetViper())

	// Add stack completion.
	_ = listComponentsCmd.RegisterFlagCompletionFunc("stack", stackFlagCompletion)

	listCmd.AddCommand(listComponentsCmd)
}

// listComponents is a helper function used by ComponentsArgCompletion.
// It replicates the main command logic for use in shell completion.
func listComponents(cmd *cobra.Command) ([]string, error) {
	// Parse flags using StandardOptions.
	opts, err := listComponentsParser.Parse(context.Background(), []string{})
	if err != nil {
		return nil, err
	}

	configAndStacksInfo := schema.ConfigAndStacksInfo{}
	atmosConfig, err := config.InitCliConfig(configAndStacksInfo, true)
	if err != nil {
		return nil, fmt.Errorf("error initializing CLI config: %v", err)
	}

	stacksMap, err := e.ExecuteDescribeStacks(&atmosConfig, "", nil, nil, nil, true, true, false, false, nil, nil)
	if err != nil {
		return nil, fmt.Errorf("error describing stacks: %v", err)
	}

	return l.FilterAndListComponents(opts.Stack, stacksMap)
}
