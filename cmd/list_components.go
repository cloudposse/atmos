package cmd

import (
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

// listComponentsCmd lists atmos components
var listComponentsCmd = &cobra.Command{
	Use:   "components",
	Short: "List all Atmos components or filter by stack",
	Long:  "List Atmos components, with options to filter results by specific stacks.",
	Args:  cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		// Check Atmos configuration
		checkAtmosConfig()
		output, err := listComponents(cmd)
		if err != nil {
			u.PrintErrorMarkdownAndExit("", err, "")
			return
		}

		u.PrintMessageInColor(strings.Join(output, "\n")+"\n", theme.Colors.Success)
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
		return nil, fmt.Errorf("Error getting the `stack` flag: `%v`", err)
	}

	configAndStacksInfo := schema.ConfigAndStacksInfo{}
	atmosConfig, err := config.InitCliConfig(configAndStacksInfo, true)
	if err != nil {
		return nil, fmt.Errorf("Error initializing CLI config: %v", err)
	}

	stacksMap, err := e.ExecuteDescribeStacks(atmosConfig, "", nil, nil, nil, false, false, false, false, nil)
	if err != nil {
		return nil, fmt.Errorf("Error describing stacks: %v", err)
	}

	output, err := l.FilterAndListComponents(stackFlag, stacksMap)
	if err != nil {
		return nil, err
	}

	selectorFlag, _ := flags.GetString("selector")
	if selectorFlag != "" {
		reqs, err := selector.Parse(selectorFlag)
		if err != nil {
			return nil, err
		}
		output = filterComponentsBySelector(output, stacksMap, reqs)
	}

	if len(output) == 0 {
		log.Info("No components matched selector.")
		return []string{}, nil
	}

	return output, nil
}

// filterComponentsBySelector filters components based on selector requirements
func filterComponentsBySelector(components []string, stacksMap map[string]any, reqs []selector.Requirement) []string {
	var filtered []string
	for _, comp := range components {
		if componentMatchesSelector(comp, stacksMap, reqs) {
			filtered = append(filtered, comp)
		}
	}
	return filtered
}

// componentMatchesSelector checks if a component matches the selector requirements
func componentMatchesSelector(comp string, stacksMap map[string]any, reqs []selector.Requirement) bool {
	for _, sdata := range stacksMap {
		smap, ok := sdata.(map[string]any)
		if !ok {
			continue
		}

		components, ok := smap["components"].(map[string]any)
		if !ok {
			continue
		}

		if found, matches := checkComponentInStack(comp, smap, components, reqs); found {
			return matches
		}
	}
	return false
}

// checkComponentInStack checks if component exists in stack and matches selector
func checkComponentInStack(comp string, smap map[string]any, components map[string]any, reqs []selector.Requirement) (found, matches bool) {
	for _, ctype := range []string{"terraform", "helmfile"} {
		if ctypeSection, ok := components[ctype].(map[string]any); ok {
			if _, ok := ctypeSection[comp]; ok {
				merged := selector.MergedLabels(smap, comp)
				return true, selector.Matches(merged, reqs)
			}
		}
	}
	return false, false
}
