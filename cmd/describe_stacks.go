package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	e "github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui/theme"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// describeStacksCmd describes atmos stacks with rich formatting options
var describeStacksCmd = &cobra.Command{
	Use:   "stacks",
	Short: "Show detailed information about Atmos stacks",
	Long:  "This command shows detailed information about Atmos stacks with rich formatting options for output customization. It supports filtering by stack, selecting specific fields, and transforming output using YQ expressions.",
	Example: "# Show detailed information for all stacks (colored YAML output)\n" +
		"atmos describe stacks\n\n" +
		"# Filter by a specific stack\n" +
		"atmos describe stacks -s dev\n\n" +
		"# Show specific fields in JSON format\n" +
		"atmos describe stacks --json name,components\n\n" +
		"# Show vars for a specific stack\n" +
		"atmos describe stacks -s dev --json name,components --jq '.dev.components.terraform.myapp.vars'\n\n" +
		"# Transform JSON output using YQ expressions\n" +
		"atmos describe stacks --json name,components --jq '.dev'\n\n" +
		"# List available JSON fields\n" +
		"atmos describe stacks --json",
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: false},
	Args:               cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		// Check Atmos configuration
		checkAtmosConfig()

		jsonFlag, _ := cmd.Flags().GetString("json")
		jqFlag, _ := cmd.Flags().GetString("jq")
		templateFlag, _ := cmd.Flags().GetString("template")
		stackFlag, _ := cmd.Flags().GetString("stack")

		// Validate that --json is provided when using --jq or --template
		if (jqFlag != "" || templateFlag != "") && jsonFlag == "" {
			u.PrintMessageInColor("Error: --json flag is required when using --jq or --template", theme.Colors.Error)
			return
		}

		// Validate that only one of --jq or --template is used
		if jqFlag != "" && templateFlag != "" {
			u.PrintMessageInColor("Error: cannot use both --jq and --template flags at the same time", theme.Colors.Error)
			return
		}

		configAndStacksInfo := schema.ConfigAndStacksInfo{}
		atmosConfig, err := config.InitCliConfig(configAndStacksInfo, true)
		if err != nil {
			u.PrintMessageInColor(fmt.Sprintf("Error initializing CLI config: %v", err), theme.Colors.Error)
			return
		}

		stacksMap, err := e.ExecuteDescribeStacks(atmosConfig, stackFlag, nil, nil, nil, false, false, false)
		if err != nil {
			u.PrintMessageInColor(fmt.Sprintf("Error describing stacks: %v", err), theme.Colors.Error)
			return
		}

		// If --json is provided with no value, show available fields
		if jsonFlag == "" && (cmd.Flags().Changed("json") || jqFlag != "" || templateFlag != "") {
			availableFields := []string{
				"name",
				"components",
				"terraform",
				"helmfile",
				"description",
				"namespace",
				"base_component",
				"vars",
				"env",
				"backend",
			}
			u.PrintMessageInColor("Available JSON fields:\n"+strings.Join(availableFields, "\n"), theme.Colors.Info)
			return
		}

		output, err := e.FormatStacksOutput(stacksMap, jsonFlag, jqFlag, templateFlag)
		if err != nil {
			u.PrintMessageInColor(fmt.Sprintf("Error formatting output: %v", err), theme.Colors.Error)
			return
		}

		// Only use colored output for non-JSON/template formats
		if jsonFlag == "" && jqFlag == "" && templateFlag == "" {
			u.PrintMessageInColor(output, theme.Colors.Success)
		} else {
			fmt.Println(output)
			u.LogErrorAndExit(err)
		}
	},
}

func init() {
	describeStacksCmd.DisableFlagParsing = false
	describeStacksCmd.Flags().StringP("stack", "s", "", "Filter by a specific stack")
	describeStacksCmd.Flags().String("json", "", "Comma-separated list of fields to include in JSON output")
	describeStacksCmd.Flags().String("jq", "", "JQ query to transform JSON output (requires --json)")
	describeStacksCmd.Flags().String("template", "", "Go template to format JSON output (requires --json)")
	describeCmd.AddCommand(describeStacksCmd)
}
