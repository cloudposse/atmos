package list

import (
	"errors"

	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	e "github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/flags/global"
	l "github.com/cloudposse/atmos/pkg/list"
	listerrors "github.com/cloudposse/atmos/pkg/list/errors"
	f "github.com/cloudposse/atmos/pkg/list/format"
	listutils "github.com/cloudposse/atmos/pkg/list/utils"
	"github.com/cloudposse/atmos/pkg/schema"
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

		output, err := listMetadataWithOptions(opts, args)
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

func listMetadataWithOptions(opts *MetadataOptions, args []string) (string, error) {
	// Set default delimiter for CSV
	if f.Format(opts.Format) == f.FormatCSV && opts.Delimiter == f.DefaultTSVDelimiter {
		opts.Delimiter = f.DefaultCSVDelimiter
	}

	componentFilter := ""
	if len(args) > 0 {
		componentFilter = args[0]
	}

	// Initialize CLI config
	configAndStacksInfo := schema.ConfigAndStacksInfo{}
	atmosConfig, err := config.InitCliConfig(configAndStacksInfo, true)
	if err != nil {
		return "", &listerrors.InitConfigError{Cause: err}
	}

	if componentFilter != "" {
		if !listutils.CheckComponentExists(&atmosConfig, componentFilter) {
			return "", &listerrors.ComponentDefinitionNotFoundError{Component: componentFilter}
		}
	}

	// Get all stacks
	stacksMap, err := e.ExecuteDescribeStacks(&atmosConfig, "", nil, nil, nil, false,
		opts.ProcessTemplates, opts.ProcessFunctions, false, nil, nil)
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
		var noValuesErr *listerrors.NoValuesFoundError
		if errors.As(err, &noValuesErr) {
			logNoMetadataFoundMessage(componentFilter)
			return "", nil
		}
		return "", err
	}

	return output, nil
}
