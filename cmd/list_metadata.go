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
	utils "github.com/cloudposse/atmos/pkg/utils"
)

// listMetadataCmd lists metadata across stacks.
var listMetadataCmd = &cobra.Command{
	Use:   "metadata [component]",
	Short: "List metadata across stacks",
	Long:  "List metadata information across all stacks or for a specific component",
	Example: "atmos list metadata\n" +
		"atmos list metadata c1\n" +
		"atmos list metadata --query .component\n" +
		"atmos list metadata --format json\n" +
		"atmos list metadata --stack '*-{dev,staging}-*'\n" +
		"atmos list metadata --stack 'prod-*'",
	Run: func(cmd *cobra.Command, args []string) {
		// Check Atmos configuration
		checkAtmosConfig()
		output, err := listMetadata(cmd, args)
		if err != nil {
			log.Error("failed to list metadata", "error", err)
			return
		}

		utils.PrintMessage(output)
	},
}

func init() {
	fl.AddCommonListFlags(listMetadataCmd)

	// Add template and function processing flags
	listMetadataCmd.PersistentFlags().Bool("process-templates", true, "Enable/disable Go template processing in Atmos stack manifests when executing the command")
	listMetadataCmd.PersistentFlags().Bool("process-functions", true, "Enable/disable YAML functions processing in Atmos stack manifests when executing the command")

	AddStackCompletion(listMetadataCmd)

	listCmd.AddCommand(listMetadataCmd)
}

// setupMetadataOptions sets up the filter options for metadata listing.
func setupMetadataOptions(commonFlags fl.CommonFlags, componentFilter string) *l.FilterOptions {
	query := commonFlags.Query
	if query == "" {
		query = ".metadata"
	}

	return &l.FilterOptions{
		Component:       l.KeyMetadata,
		ComponentFilter: componentFilter,
		Query:           query,
		IncludeAbstract: false,
		MaxColumns:      commonFlags.MaxColumns,
		FormatStr:       commonFlags.Format,
		Delimiter:       commonFlags.Delimiter,
		StackPattern:    commonFlags.Stack,
	}
}

// handleMetadataError handles specific metadata-related errors.
func handleMetadataError(err error, componentFilter, query, format string) (string, error) {
	if componentFilter != "" {
		_, isComponentMetadataError := err.(*errors.ComponentMetadataNotFoundError)
		_, isNoValuesError := err.(*errors.NoValuesFoundError)
		_, isNoMetadataError := err.(*errors.NoMetadataFoundError)

		if isComponentMetadataError || isNoValuesError || isNoMetadataError {
			log.Debug("No metadata found for component, returning empty result",
				"component", componentFilter,
				"error_type", fmt.Sprintf("%T", err))

			switch f.Format(format) {
			case f.FormatJSON, f.FormatYAML:
				return "{}", nil
			case f.FormatCSV, f.FormatTSV:
				return "", nil
			default:
				return fmt.Sprintf("No metadata found for component '%s'\n", componentFilter), nil
			}
		}
	}

	if u.IsNoValuesFoundError(err) {
		return "", &errors.NoMetadataFoundError{Query: query}
	}

	return "", &errors.MetadataFilteringError{Cause: err}
}

func listMetadata(cmd *cobra.Command, args []string) (string, error) {
	commonFlags, err := fl.GetCommonListFlags(cmd)
	if err != nil {
		return "", &errors.QueryError{
			Query: "common flags",
			Cause: err,
		}
	}

	// Get template and function processing flags
	processingFlags := fl.GetProcessingFlags(cmd)

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
	stacksMap, err := e.ExecuteDescribeStacks(atmosConfig, "", nil, nil, nil, false,
		processingFlags.Templates, processingFlags.Functions, false, nil)
	if err != nil {
		return "", &errors.DescribeStacksError{Cause: err}
	}

	componentFilter := ""
	if len(args) > 0 {
		componentFilter = args[0]
	}

	log.Info("Filtering metadata",
		"component", componentFilter, "query", commonFlags.Query,
		"maxColumns", commonFlags.MaxColumns, "format", commonFlags.Format,
		"stackPattern", commonFlags.Stack, "templates", processingFlags.Templates)

	filterOptions := setupMetadataOptions(*commonFlags, componentFilter)
	output, err := l.FilterAndListValues(stacksMap, filterOptions)
	if err != nil {
		return handleMetadataError(err, componentFilter, commonFlags.Query,
			commonFlags.Format)
	}

	return output, nil
}
