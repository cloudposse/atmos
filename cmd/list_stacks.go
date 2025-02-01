package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	e "github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui/theme"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// listStacksCmd lists atmos stacks
var listStacksCmd = &cobra.Command{
	Use:   "stacks",
	Short: "List all Atmos stacks",
	Long:  "This command lists all Atmos stacks. For detailed filtering and output formatting, use 'atmos describe stacks'.",
	Example: "# List all stacks\n" +
		"atmos list stacks\n\n" +
		"# For detailed stack information and filtering, use:\n" +
		"atmos describe stacks",
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: false},
	Args:               cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		// Check Atmos configuration
		checkAtmosConfig()

		configAndStacksInfo := schema.ConfigAndStacksInfo{}
		atmosConfig, err := config.InitCliConfig(configAndStacksInfo, true)
		if err != nil {
			u.PrintMessageInColor(fmt.Sprintf("Error initializing CLI config: %v", err), theme.Colors.Error)
			return
		}

		stacksMap, err := e.ExecuteDescribeStacks(atmosConfig, "", nil, nil, nil, false, false, false)
		if err != nil {
			u.PrintMessageInColor(fmt.Sprintf("Error describing stacks: %v", err), theme.Colors.Error)
			return
		}

		// Simple list of stack names
		var stackNames []string
		for stackName := range stacksMap {
			stackNames = append(stackNames, stackName)
		}

		if len(stackNames) == 0 {
			u.PrintMessageInColor("No stacks found\n", theme.Colors.Warning)
			return
		}

		output := "Available stacks:\n"
		for _, name := range stackNames {
			output += fmt.Sprintf("  %s\n", name)
		}
		u.PrintMessageInColor(output, theme.Colors.Success)
	},
}

func init() {
	listStacksCmd.DisableFlagParsing = false
	listCmd.AddCommand(listStacksCmd)
}
