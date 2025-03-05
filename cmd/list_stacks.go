package cmd

import (
	"strings"

	log "github.com/charmbracelet/log"
	"github.com/spf13/cobra"

	e "github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/config"
	l "github.com/cloudposse/atmos/pkg/list"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/utils"
)

// listStacksCmd lists atmos stacks.
var listStacksCmd = &cobra.Command{
	Use:                "stacks",
	Short:              "List all Atmos stacks or stacks for a specific component",
	Long:               "This command lists all Atmos stacks, or filters the list to show only the stacks associated with a specified component.",
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: false},
	Args:               cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		// Check Atmos configuration.
		checkAtmosConfigFn()
		output, err := listStacksFn(cmd)
		if err != nil {
			log.Error("error filtering stacks", "error", err)
			return
		}
		utils.PrintMessage(strings.Join(output, "\n"))
	},
}

func init() {
	listStacksCmd.DisableFlagParsing = false
	listStacksCmd.PersistentFlags().StringP("component", "c", "", "List all stacks that contain the specified component.")
	listCmd.AddCommand(listStacksCmd)
}

var (
	listStacksFn       = listStacks
	checkAtmosConfigFn = checkAtmosConfig
)

func listStacks(cmd *cobra.Command) ([]string, error) {
	componentFlag, _ := cmd.Flags().GetString("component")
	configAndStacksInfo := schema.ConfigAndStacksInfo{}
	atmosConfig, err := config.InitCliConfig(configAndStacksInfo, true)
	if err != nil {
		log.Error("failed to initialize CLI config", "error", err)
		return nil, err
	}
	stacksMap, err := e.ExecuteDescribeStacks(atmosConfig, "", nil, nil, nil, false, false, false, false, nil)
	if err != nil {
		log.Error("failed to describe stacks", "error", err)
		return nil, err
	}

	output, err := l.FilterAndListStacks(stacksMap, componentFlag)
	return output, err
}
