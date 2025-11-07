package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	e "github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/config"
	l "github.com/cloudposse/atmos/pkg/list"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui"
	"github.com/cloudposse/atmos/pkg/ui/theme"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// listComponentsCmd lists atmos components
var listComponentsCmd = &cobra.Command{
	Use:   "components",
	Short: "List all Atmos components or filter by stack",
	Long:  "List Atmos components, with options to filter results by specific stacks.",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Check Atmos configuration
		checkAtmosConfig()

		output, err := listComponents(cmd)
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
	AddStackCompletion(listComponentsCmd)
	listCmd.AddCommand(listComponentsCmd)
}

func listComponents(cmd *cobra.Command) ([]string, error) {
	flags := cmd.Flags()

	stackFlag, err := flags.GetString("stack")
	if err != nil {
		return nil, fmt.Errorf("error getting the `stack` flag: `%v`", err)
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

	output, err := l.FilterAndListComponents(stackFlag, stacksMap)
	return output, err
}
