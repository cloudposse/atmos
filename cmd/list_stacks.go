package cmd

import (
	"errors"
	"fmt"
	"strings"

	log "github.com/charmbracelet/log"
	"github.com/spf13/cobra"

	e "github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/config"
	l "github.com/cloudposse/atmos/pkg/list"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/selector"
	"github.com/cloudposse/atmos/pkg/ui/theme"
	u "github.com/cloudposse/atmos/pkg/utils"
)

var (
	// Static errors for better error handling.
	ErrStacksInitConfig     = errors.New("error initializing CLI config")
	ErrStacksDescribeStacks = errors.New("error describing stacks")
)

// listStacksCmd lists atmos stacks.
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
		if err != nil {
			u.PrintErrorMarkdownAndExit("Error filtering stacks", err, "")
			return
		}
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
	selectorFlag, _ := cmd.Flags().GetString("selector")
	configAndStacksInfo := schema.ConfigAndStacksInfo{}
	atmosConfig, err := config.InitCliConfig(configAndStacksInfo, true)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrStacksInitConfig, err)
	}
	stacksMap, err := e.ExecuteDescribeStacks(atmosConfig, "", nil, nil, nil, false, false, false, false, nil)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrStacksDescribeStacks, err)
	}

	output, err := l.FilterAndListStacks(stacksMap, componentFlag)
	if err != nil {
		return nil, err
	}

	if selectorFlag != "" {
		var err error
		output, err = applyStacksSelector(output, stacksMap, selectorFlag)
		if err != nil {
			return nil, err
		}
	}

	if len(output) == 0 {
		log.Info("No stacks matched selector.")
		return []string{}, nil
	}

	return output, nil
}

// applyStacksSelector filters stacks based on label selector requirements.
func applyStacksSelector(stacks []string, stacksMap map[string]any, selectorStr string) ([]string, error) {
	reqs, err := selector.Parse(selectorStr)
	if err != nil {
		return nil, err
	}

	var filtered []string
	for _, stackName := range stacks {
		if sdata, ok := stacksMap[stackName].(map[string]any); ok {
			labels := selector.ExtractStackLabels(sdata)
			if selector.Matches(labels, reqs) {
				filtered = append(filtered, stackName)
			}
		}
	}
	return filtered, nil
}
