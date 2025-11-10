package cmd

import (
	"errors"

	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/spf13/cobra"

	e "github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/config"
	l "github.com/cloudposse/atmos/pkg/list"
	listerrors "github.com/cloudposse/atmos/pkg/list/errors"
	fl "github.com/cloudposse/atmos/pkg/list/flags"
	f "github.com/cloudposse/atmos/pkg/list/format"
	listutils "github.com/cloudposse/atmos/pkg/list/utils"
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
	RunE: func(cmd *cobra.Command, args []string) error {
		checkAtmosConfig()

		output, err := listMetadata(cmd, args)
		if err != nil {
			return err
		}

		utils.PrintMessage(output)
		return nil
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

// logNoMetadataFoundMessage logs an appropriate message when no metadata is found.
func logNoMetadataFoundMessage(componentFilter string) {
	if componentFilter != "" {
		log.Info("No metadata found", "component", componentFilter)
	} else {
		log.Info("No metadata found")
	}
}

// MetadataParams contains the parameters needed for listing metadata.
type MetadataParams struct {
	CommonFlags     *fl.CommonFlags
	ProcessingFlags *fl.ProcessingFlags
	ComponentFilter string
}

// initMetadataParams initializes and returns the parameters needed for listing metadata.
func initMetadataParams(cmd *cobra.Command, args []string) (*MetadataParams, error) {
	commonFlags, err := fl.GetCommonListFlags(cmd)
	if err != nil {
		return nil, &listerrors.QueryError{
			Query: "common flags",
			Cause: err,
		}
	}

	processingFlags := fl.GetProcessingFlags(cmd)

	if f.Format(commonFlags.Format) == f.FormatCSV && commonFlags.Delimiter == f.DefaultTSVDelimiter {
		commonFlags.Delimiter = f.DefaultCSVDelimiter
	}

	componentFilter := ""
	if len(args) > 0 {
		componentFilter = args[0]
	}

	return &MetadataParams{
		CommonFlags:     commonFlags,
		ProcessingFlags: processingFlags,
		ComponentFilter: componentFilter,
	}, nil
}

func listMetadata(cmd *cobra.Command, args []string) (string, error) {
	params, err := initMetadataParams(cmd, args)
	if err != nil {
		return "", err
	}

	// Initialize CLI config
	configAndStacksInfo := schema.ConfigAndStacksInfo{}
	atmosConfig, err := config.InitCliConfig(configAndStacksInfo, true)
	if err != nil {
		return "", &listerrors.InitConfigError{Cause: err}
	}

	if params.ComponentFilter != "" {
		if !listutils.CheckComponentExists(&atmosConfig, params.ComponentFilter) {
			return "", &listerrors.ComponentDefinitionNotFoundError{Component: params.ComponentFilter}
		}
	}

	// Get all stacks
	stacksMap, err := e.ExecuteDescribeStacks(&atmosConfig, "", nil, nil, nil, false,
		params.ProcessingFlags.Templates, params.ProcessingFlags.Functions, false, nil, nil)
	if err != nil {
		return "", &listerrors.DescribeStacksError{Cause: err}
	}

	log.Debug("Filtering metadata",
		"component", params.ComponentFilter, "query", params.CommonFlags.Query,
		"maxColumns", params.CommonFlags.MaxColumns, "format", params.CommonFlags.Format,
		"stackPattern", params.CommonFlags.Stack, "templates", params.ProcessingFlags.Templates)

	filterOptions := setupMetadataOptions(*params.CommonFlags, params.ComponentFilter)
	output, err := l.FilterAndListValues(stacksMap, filterOptions)
	if err != nil {
		var noValuesErr *listerrors.NoValuesFoundError
		if errors.As(err, &noValuesErr) {
			logNoMetadataFoundMessage(params.ComponentFilter)
			return "", nil
		}
		return "", err
	}

	return output, nil
}
