package list

import (
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	e "github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/flags/global"
	l "github.com/cloudposse/atmos/pkg/list"
	listerrors "github.com/cloudposse/atmos/pkg/list/errors"
	utils "github.com/cloudposse/atmos/pkg/utils"
)

var metadataParser *flags.StandardParser

// MetadataOptions contains parsed flags for the metadata command.
type MetadataOptions struct {
	global.Flags
	Format           string
	MaxColumns       int
	Delimiter        string
	Stack            string
	Query            string
	ProcessTemplates bool
	ProcessFunctions bool
}

// metadataCmd lists metadata across stacks.
var metadataCmd = &cobra.Command{
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
		if err := checkAtmosConfig(); err != nil {
			return err
		}

		// Parse flags using StandardParser with Viper precedence
		v := viper.GetViper()
		if err := metadataParser.BindFlagsToViper(cmd, v); err != nil {
			return err
		}

		opts := &MetadataOptions{
			Flags:            flags.ParseGlobalFlags(cmd, v),
			Format:           v.GetString("format"),
			MaxColumns:       v.GetInt("max-columns"),
			Delimiter:        v.GetString("delimiter"),
			Stack:            v.GetString("stack"),
			Query:            v.GetString("query"),
			ProcessTemplates: v.GetBool("process-templates"),
			ProcessFunctions: v.GetBool("process-functions"),
		}

		output, err := listMetadataWithOptions(cmd, opts, args)
		if err != nil {
			return err
		}

		utils.PrintMessage(output)
		return nil
	},
}

func init() {
	// Create parser with common list flags plus processing flags
	metadataParser = newCommonListParser(
		flags.WithBoolFlag("process-templates", "", true, "Enable/disable Go template processing in Atmos stack manifests when executing the command"),
		flags.WithBoolFlag("process-functions", "", true, "Enable/disable YAML functions processing in Atmos stack manifests when executing the command"),
		flags.WithEnvVars("process-templates", "ATMOS_PROCESS_TEMPLATES"),
		flags.WithEnvVars("process-functions", "ATMOS_PROCESS_FUNCTIONS"),
	)

	// Register flags
	metadataParser.RegisterFlags(metadataCmd)

	// Add stack completion
	addStackCompletion(metadataCmd)

	// Bind flags to Viper for environment variable support
	if err := metadataParser.BindToViper(viper.GetViper()); err != nil {
		panic(err)
	}
}

// setupMetadataOptions sets up the filter options for metadata listing.
func setupMetadataOptions(opts *MetadataOptions, componentFilter string) *l.FilterOptions {
	query := opts.Query
	if query == "" {
		query = ".metadata"
	}

	return &l.FilterOptions{
		Component:       l.KeyMetadata,
		ComponentFilter: componentFilter,
		Query:           query,
		IncludeAbstract: false,
		MaxColumns:      opts.MaxColumns,
		FormatStr:       opts.Format,
		Delimiter:       opts.Delimiter,
		StackPattern:    opts.Stack,
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

func listMetadataWithOptions(cmd *cobra.Command, opts *MetadataOptions, args []string) (string, error) {
	// Set default delimiter for CSV.
	setDefaultCSVDelimiter(&opts.Delimiter, opts.Format)

	componentFilter := getComponentFilter(args)

	// Initialize CLI config and auth manager.
	atmosConfig, authManager, err := initConfigAndAuth(cmd)
	if err != nil {
		return "", err
	}

	// Validate component exists if filter is specified.
	if err := validateComponentFilter(&atmosConfig, componentFilter); err != nil {
		return "", err
	}

	// Get all stacks.
	stacksMap, err := e.ExecuteDescribeStacks(&atmosConfig, "", nil, nil, nil, false,
		opts.ProcessTemplates, opts.ProcessFunctions, false, nil, authManager)
	if err != nil {
		return "", &listerrors.DescribeStacksError{Cause: err}
	}

	log.Debug("Filtering metadata",
		"component", componentFilter, "query", opts.Query,
		"maxColumns", opts.MaxColumns, "format", opts.Format,
		"stackPattern", opts.Stack, "templates", opts.ProcessTemplates)

	filterOptions := setupMetadataOptions(opts, componentFilter)
	output, err := l.FilterAndListValues(stacksMap, filterOptions)
	if err != nil {
		return handleNoValuesError(err, componentFilter, logNoMetadataFoundMessage)
	}

	return output, nil
}
