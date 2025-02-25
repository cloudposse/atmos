package cmd

import (
	"fmt"

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

		fmt.Println(output)
	},
}

func init() {
	fl.AddCommonListFlags(listSettingsCmd)

	AddStackCompletion(listSettingsCmd)

	listCmd.AddCommand(listSettingsCmd)
}

func listSettings(cmd *cobra.Command) (string, error) {
	// Get common flags
	commonFlags, err := fl.GetCommonListFlags(cmd)
	if err != nil {
		return "", &errors.CommonFlagsError{Cause: err}
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
	stacksMap, err := e.ExecuteDescribeStacks(atmosConfig, "", nil, nil, nil, false, false, false, false, nil)
	if err != nil {
		return "", &errors.DescribeStacksError{Cause: err}
	}

	// Use empty query to avoid further processing since handleSpecialComponent will extract the settings
	output, err := l.FilterAndListValues(stacksMap, "settings", commonFlags.Query, false, commonFlags.MaxColumns, commonFlags.Format, commonFlags.Delimiter, commonFlags.Stack)
	if err != nil {
		if u.IsNoValuesFoundError(err) {
			return "", &errors.NoSettingsFoundError{Query: commonFlags.Query}
		}
		return "", &errors.SettingsFilteringError{Cause: err}
	}

	return output, nil
}
