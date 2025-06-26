package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	atmoserr "github.com/cloudposse/atmos/errors"
	e "github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/config"
	l "github.com/cloudposse/atmos/pkg/list"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui/theme"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// listStacksCmd lists atmos stacks
var listStacksCmd = &cobra.Command{
	Use:                "stacks",
	Short:              "List all Atmos stacks or stacks for a specific component",
	Long:               "This command lists all Atmos stacks, or filters the list to show only the stacks associated with a specified component.",
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: false},
	Args:               cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		// Check Atmos configuration
		checkAtmosConfig()
		output, err := listStacks(cmd)
		atmoserr.PrintErrorMarkdownAndExit(err, "", "")
		u.PrintMessageInColor(strings.Join(output, "\n")+"\n", theme.Colors.Success)
	},
}

func init() {
	listStacksCmd.DisableFlagParsing = false
	listStacksCmd.PersistentFlags().StringP("component", "c", "", "List all stacks that contain the specified component.")
	listCmd.AddCommand(listStacksCmd)
}

func listStacks(cmd *cobra.Command) ([]string, error) {
	componentFlag, _ := cmd.Flags().GetString("component")
	configAndStacksInfo := schema.ConfigAndStacksInfo{}
	atmosConfig, err := config.InitCliConfig(configAndStacksInfo, true)
	if err != nil {
		return nil, fmt.Errorf("Error initializing CLI config: %v", err)
	}
	stacksMap, err := e.ExecuteDescribeStacks(atmosConfig, "", nil, nil, nil, false, false, false, false, nil)
	if err != nil {
		return nil, fmt.Errorf("Error describing stacks: %v", err)
	}

	output, err := l.FilterAndListStacks(stacksMap, componentFlag)
	return output, err
}
