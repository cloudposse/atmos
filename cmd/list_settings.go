package cmd

import (
	log "github.com/charmbracelet/log"
	"github.com/spf13/cobra"

	e "github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/config"
	l "github.com/cloudposse/atmos/pkg/list"
	"github.com/cloudposse/atmos/pkg/list/errors"
	fl "github.com/cloudposse/atmos/pkg/list/flags"
	f "github.com/cloudposse/atmos/pkg/list/format"
	u "github.com/cloudposse/atmos/pkg/list/utils"
	"github.com/cloudposse/atmos/pkg/schema"
	utils "github.com/cloudposse/atmos/pkg/utils"
)

// listSettingsCmd lists settings across stacks.
var listSettingsCmd = &cobra.Command{
	Use:   "settings",
	Short: "List settings across stacks",
	Long:  "List settings configuration across all stacks",
	Example: "atmos list settings\n" +
		"atmos list settings --query .terraform\n" +
		"atmos list settings --format json\n" +
		"atmos list settings --stack '*-dev-*'\n" +
		"atmos list settings --stack 'prod-*'",
	Run: func(cmd *cobra.Command, args []string) {
		// Check Atmos configuration
		checkAtmosConfig()
		output, err := listSettings(cmd)
		if err != nil {
			log.Error("failed to list settings", "error", err)
			return
		}

		utils.PrintMessage(output)
	},
}

func init() {
	fl.AddCommonListFlags(listSettingsCmd)

	// Add template and function processing flags
	listSettingsCmd.PersistentFlags().Bool("process-templates", true, "Enable/disable Go template processing in Atmos stack manifests when executing the command")
	listSettingsCmd.PersistentFlags().Bool("process-functions", true, "Enable/disable YAML functions processing in Atmos stack manifests when executing the command")

	AddStackCompletion(listSettingsCmd)

	listCmd.AddCommand(listSettingsCmd)
}

func listSettings(cmd *cobra.Command) (string, error) {
	// Get common flags
	commonFlags, err := fl.GetCommonListFlags(cmd)
	if err != nil {
		return "", &errors.CommonFlagsError{Cause: err}
	}

	// Get template and function processing flags
	processTemplates := true
	if cmd.Flags().Lookup("process-templates") != nil {
		processTemplates, err = cmd.Flags().GetBool("process-templates")
		if err != nil {
			log.Warn("failed to get process-templates flag, using default true", "error", err)
		}
	}

	processYamlFunctions := true
	if cmd.Flags().Lookup("process-functions") != nil {
		processYamlFunctions, err = cmd.Flags().GetBool("process-functions")
		if err != nil {
			log.Warn("failed to get process-functions flag, using default true", "error", err)
		}
	}

	if f.Format(commonFlags.Format) == f.FormatCSV && commonFlags.Delimiter == f.DefaultTSVDelimiter {
		commonFlags.Delimiter = f.DefaultCSVDelimiter
	}

	// Initialize CLI config
	configAndStacksInfo := schema.ConfigAndStacksInfo{}
	atmosConfig, err := config.InitCliConfig(configAndStacksInfo, true)
	if err != nil {
		return "", &errors.InitConfigError{Cause: err}
	}

	// Get all stacks
	stacksMap, err := e.ExecuteDescribeStacks(atmosConfig, "", nil, nil, nil, false, processTemplates, processYamlFunctions, false, nil)
	if err != nil {
		return "", &errors.DescribeStacksError{Cause: err}
	}

	// Log the settings query
	log.Info("Filtering settings",
		"query", commonFlags.Query,
		"maxColumns", commonFlags.MaxColumns,
		"format", commonFlags.Format,
		"stackPattern", commonFlags.Stack,
		"processTemplates", processTemplates,
		"processYamlFunctions", processYamlFunctions)

	// Use empty query to avoid further processing since handleComponentProperties will extract the settings
	output, err := l.FilterAndListValues(stacksMap, &l.FilterOptions{
		Component:       "settings",
		Query:           commonFlags.Query,
		IncludeAbstract: false,
		MaxColumns:      commonFlags.MaxColumns,
		FormatStr:       commonFlags.Format,
		Delimiter:       commonFlags.Delimiter,
		StackPattern:    commonFlags.Stack,
	})
	if err != nil {
		if u.IsNoValuesFoundError(err) {
			return "", &errors.NoSettingsFoundError{Query: commonFlags.Query}
		}
		return "", &errors.SettingsFilteringError{Cause: err}
	}

	return output, nil
}
