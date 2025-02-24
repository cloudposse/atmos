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

// listMetadataCmd lists metadata across stacks.
var listMetadataCmd = &cobra.Command{
	Use:   "metadata",
	Short: "List metadata across stacks",
	Long:  "List metadata information across all stacks",
	Example: "atmos list metadata\n" +
		"atmos list metadata --query .component\n" +
		"atmos list metadata --format json\n" +
		"atmos list metadata --stack '*-dev-*'\n" +
		"atmos list metadata --stack 'prod-*'",
	Run: func(cmd *cobra.Command, args []string) {
		// Check Atmos configuration
		checkAtmosConfig()
		output, err := listMetadata(cmd)
		if err != nil {
			log.Error("failed to list metadata", "error", err)
			return
		}

		fmt.Println(output)
	},
}

func init() {
	fl.AddCommonListFlags(listMetadataCmd)

	AddStackCompletion(listMetadataCmd)

	listCmd.AddCommand(listMetadataCmd)
}

func listMetadata(cmd *cobra.Command) (string, error) {
	commonFlags, err := fl.GetCommonListFlags(cmd)
	if err != nil {
		return "", &errors.QueryError{
			Query: "common flags",
			Cause: err,
		}
	}

	if f.Format(commonFlags.Format) == f.FormatCSV && commonFlags.Delimiter == f.DefaultTSVDelimiter {
		commonFlags.Delimiter = f.DefaultCSVDelimiter
	}

	// Initialize CLI config
	configAndStacksInfo := schema.ConfigAndStacksInfo{}
	atmosConfig, err := config.InitCliConfig(configAndStacksInfo, true)
	if err != nil {
		return "", fmt.Errorf("error initializing CLI config: %v", err)
	}

	// Get all stacks
	stacksMap, err := e.ExecuteDescribeStacks(atmosConfig, "", nil, nil, nil, false, false, false, false, nil)
	if err != nil {
		return "", fmt.Errorf("error describing stacks: %v", err)
	}

	// Use .metadata as the default query if none provided
	if commonFlags.Query == "" {
		commonFlags.Query = ".metadata"
	}

	output, err := l.FilterAndListValues(stacksMap, "", commonFlags.Query, false, commonFlags.MaxColumns, commonFlags.Format, commonFlags.Delimiter, commonFlags.Stack)
	if err != nil {
		if u.IsNoValuesFoundError(err) {
			return "", fmt.Errorf("no metadata found in any stacks with query '%s'", commonFlags.Query)
		}
		return "", fmt.Errorf("error filtering and listing metadata: %v", err)
	}

	return output, nil
}
