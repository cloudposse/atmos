package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	e "github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/config"
	l "github.com/cloudposse/atmos/pkg/list"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui/theme"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// listStacksCmd lists atmos stacks
var listStacksCmd = &cobra.Command{
	Use:   "stacks",
	Short: "List all Atmos stacks or stacks for a specific component",
	Long:  "This command lists all Atmos stacks, or filters the list to show only the stacks associated with a specified component.",
	Example: "atmos list stacks\n" +
		"atmos list stacks -c <component>\n" +
		"atmos list stacks --format json\n" +
		"atmos list stacks --format csv --delimiter ','\n" +
		"atmos list stacks --format table",
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: false},
	Args:               cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		// Check Atmos configuration
		checkAtmosConfig()
		output, err := listStacks(cmd)
		if err != nil {
			u.PrintErrorMarkdownAndExit("Error filtering stacks", err, "")
			return
		}
		u.PrintMessageInColor(strings.Join(output, "\n")+"\n", theme.Colors.Success)
	},
}

func init() {
	listStacksCmd.DisableFlagParsing = false
	listStacksCmd.PersistentFlags().StringP("component", "c", "", "Filter stacks by component")
	listStacksCmd.PersistentFlags().StringP("format", "f", "", "Output format (table, json, csv)")
	listStacksCmd.PersistentFlags().StringP("delimiter", "d", "\t", "Delimiter for table and csv formats")
	listCmd.AddCommand(listStacksCmd)
}

func listStacks(cmd *cobra.Command) ([]string, error) {
	componentFlag, _ := cmd.Flags().GetString("component")
	formatFlag, _ := cmd.Flags().GetString("format")
	delimiterFlag, _ := cmd.Flags().GetString("delimiter")

	configAndStacksInfo := schema.ConfigAndStacksInfo{}
	atmosConfig, err := config.InitCliConfig(configAndStacksInfo, true)
	if err != nil {
		return nil, fmt.Errorf("Error initializing CLI config: %v", err)
	}

	stacksMap, err := e.ExecuteDescribeStacks(atmosConfig, "", nil, nil, nil, false, true, true, false, nil)
	if err != nil {
		return nil, fmt.Errorf("Error describing stacks: %v", err)
	}

	output, err := l.FilterAndListStacks(stacksMap, componentFlag, atmosConfig.Stacks.List, formatFlag, delimiterFlag)
	return []string{output}, err
}
