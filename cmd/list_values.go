package cmd

import (
	"context"
	"errors"
	"fmt"

	log "github.com/cloudposse/atmos/pkg/logger"
	pkgerrors "github.com/pkg/errors"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	e "github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/flags"
	l "github.com/cloudposse/atmos/pkg/list"
	listerrors "github.com/cloudposse/atmos/pkg/list/errors"
	fl "github.com/cloudposse/atmos/pkg/list/flags"
	f "github.com/cloudposse/atmos/pkg/list/format"
	listutils "github.com/cloudposse/atmos/pkg/list/utils"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

var (
	ErrInitializingCLIConfig = pkgerrors.New("error initializing CLI config")
	ErrDescribingStacks      = pkgerrors.New("error describing stacks")
	ErrComponentNameRequired = pkgerrors.New("component name is required")
	ErrInvalidArguments      = pkgerrors.New("invalid arguments: the command requires one argument 'component'")
)

// Error format strings.
const (
	ErrFmtWrapErr = "%w: %v" // Format for wrapping errors.
)

// ProcessingOptions holds flags for processing templates and YAML functions.
type ProcessingOptions struct {
	Templates bool
	Functions bool
}

// listValuesParser is created once at package initialization using builder pattern.
var listValuesParser = flags.NewStandardOptionsBuilder().
	WithStack(false).                                         // Optional stack flag for filtering.
	WithFormat([]string{"json", "yaml", "csv", "tsv"}, "yaml"). // Format flag with valid values and default.
	WithQuery().                                              // Query flag for YQ expressions.
	WithProcessTemplates(true).                               // Process templates (default true).
	WithProcessFunctions(true).                               // Process functions (default true).
	WithAbstract().                                           // Include abstract components flag.
	WithVars().                                               // Show only vars section flag.
	WithMaxColumns(0).                                        // Maximum columns for table output.
	WithDelimiter("").                                        // Delimiter for CSV/TSV output.
	Build()

// listVarsParser is created once at package initialization using builder pattern.
var listVarsParser = flags.NewStandardOptionsBuilder().
	WithStack(false).                                         // Optional stack flag for filtering.
	WithFormat([]string{"json", "yaml", "csv", "tsv"}, "yaml"). // Format flag with valid values and default.
	WithQuery().                                              // Query flag for YQ expressions (will be set to .vars).
	WithProcessTemplates(true).                               // Process templates (default true).
	WithProcessFunctions(true).                               // Process functions (default true).
	WithAbstract().                                           // Include abstract components flag.
	WithMaxColumns(0).                                        // Maximum columns for table output.
	WithDelimiter("").                                        // Delimiter for CSV/TSV output.
	Build()

// listValuesCmd lists component values across stacks.
var listValuesCmd = &cobra.Command{
	Use:   "values [component]",
	Short: "List component values across stacks",
	Long:  "List values for a component across all stacks where it is used",
	Example: "atmos list values vpc\n" +
		"atmos list values vpc --abstract\n" +
		"atmos list values vpc --query '.vars'\n" +
		"atmos list values vpc --query '.vars.region'\n" +
		"atmos list values vpc --format json\n" +
		"atmos list values vpc --format yaml\n" +
		"atmos list values vpc --format csv",
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) != 1 {
			return ErrInvalidArguments
		}

		// Check Atmos configuration.
		checkAtmosConfig()

		v := viper.New()
		_ = listValuesParser.BindFlagsToViper(cmd, v)

		// Parse command-line arguments and get strongly-typed options.
		opts, err := listValuesParser.Parse(context.Background(), args)
		if err != nil {
			return err
		}

		output, err := listValuesWithOptions(cmd, args, opts)
		if err != nil {
			return err
		}

		u.PrintMessage(output)
		return nil
	},
}

// listVarsCmd is an alias for 'list values --query .vars'.
var listVarsCmd = &cobra.Command{
	Use:   "vars [component]",
	Short: "List component vars across stacks (alias for `list values --query .vars`)",
	Long:  "List vars for a component across all stacks where it is used",
	Example: "atmos list vars vpc\n" +
		"atmos list vars vpc --abstract\n" +
		"atmos list vars vpc --max-columns 5\n" +
		"atmos list vars vpc --format json\n" +
		"atmos list vars vpc --format yaml\n" +
		"atmos list vars vpc --format csv",
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		// Check Atmos configuration.
		checkAtmosConfig()

		v := viper.New()
		_ = listVarsParser.BindFlagsToViper(cmd, v)

		// Set the query flag to .vars.
		if err := cmd.Flags().Set("query", ".vars"); err != nil {
			return fmt.Errorf("failed to set query flag: %w", err)
		}

		// Parse command-line arguments and get strongly-typed options.
		opts, err := listVarsParser.Parse(context.Background(), args)
		if err != nil {
			return err
		}

		// Override query to .vars for vars command.
		opts.Query = ".vars"

		output, err := listValuesWithOptions(cmd, args, opts)
		if err != nil {
			var componentVarsNotFoundErr *listerrors.ComponentVarsNotFoundError
			if errors.As(err, &componentVarsNotFoundErr) {
				log.Info("No vars found", "component", componentVarsNotFoundErr.Component)
				return nil
			}

			var noValuesErr *listerrors.NoValuesFoundError
			if errors.As(err, &noValuesErr) {
				log.Info("No values found for query '.vars'", "component", args[0])
				return nil
			}

			return err
		}

		u.PrintMessage(output)
		return nil
	},
}

func init() {
	// Register parser flags for listValuesCmd.
	listValuesParser.RegisterFlags(listValuesCmd)
	_ = listValuesParser.BindToViper(viper.GetViper())

	// Update query flag usage.
	if queryFlag := listValuesCmd.PersistentFlags().Lookup("query"); queryFlag != nil {
		queryFlag.Usage = "Filter the results using YQ expressions"
	}

	// Add stack pattern completion.
	AddStackCompletion(listValuesCmd)

	// Register parser flags for listVarsCmd.
	listVarsParser.RegisterFlags(listVarsCmd)
	_ = listVarsParser.BindToViper(viper.GetViper())

	// Update query flag usage.
	if queryFlag := listVarsCmd.PersistentFlags().Lookup("query"); queryFlag != nil {
		queryFlag.Usage = "Filter the results using YQ expressions"
	}

	// Add stack pattern completion.
	AddStackCompletion(listVarsCmd)

	// Add commands to list command.
	listCmd.AddCommand(listValuesCmd)
	listCmd.AddCommand(listVarsCmd)
}

// logNoValuesFoundMessage logs an appropriate message when no values or vars are found.
func logNoValuesFoundMessage(componentName string, query string) {
	if query == ".vars" {
		log.Info("No vars found", "component", componentName)
	} else {
		log.Info("No values found", "component", componentName)
	}
}

// initAtmosAndDescribeStacksForList initializes Atmos config and describes stacks.
func initAtmosAndDescribeStacksForList(componentName string, processingFlags *fl.ProcessingFlags) (schema.AtmosConfiguration, map[string]interface{}, error) {
	// Initialize CLI config
	configAndStacksInfo := schema.ConfigAndStacksInfo{}
	atmosConfig, err := config.InitCliConfig(configAndStacksInfo, true)
	if err != nil {
		return schema.AtmosConfiguration{}, nil, fmt.Errorf(ErrFmtWrapErr, ErrInitializingCLIConfig, err)
	}

	// Check if the component exists
	if !listutils.CheckComponentExists(&atmosConfig, componentName) {
		return schema.AtmosConfiguration{}, nil, &listerrors.ComponentDefinitionNotFoundError{Component: componentName}
	}

	// Get all stacks
	stacksMap, err := e.ExecuteDescribeStacks(&atmosConfig, "", nil, nil, nil, false, processingFlags.Templates, processingFlags.Functions, false, nil, nil)
	if err != nil {
		return schema.AtmosConfiguration{}, nil, fmt.Errorf(ErrFmtWrapErr, ErrDescribingStacks, err)
	}

	return atmosConfig, stacksMap, nil
}

// listValuesWithOptions lists component values using parsed options.
func listValuesWithOptions(cmd *cobra.Command, args []string, opts *flags.StandardOptions) (string, error) {
	// Ensure we have a component name.
	if len(args) == 0 {
		return "", ErrComponentNameRequired
	}
	componentName := args[0]

	// Use flags from parsed options (no direct Cobra access).
	// Vars flag takes precedence over query.
	if opts.Vars {
		opts.Query = ".vars"
	}

	// Set appropriate default delimiter based on format.
	delimiter := opts.Delimiter
	if f.Format(opts.Format) == f.FormatCSV && delimiter == "" {
		delimiter = f.DefaultCSVDelimiter
	} else if delimiter == "" {
		delimiter = f.DefaultTSVDelimiter
	}

	// Prepare filter options.
	filterOptions := &l.FilterOptions{
		Query:           opts.Query,
		IncludeAbstract: opts.Abstract,
		MaxColumns:      opts.MaxColumns,
		FormatStr:       opts.Format,
		Delimiter:       delimiter,
		StackPattern:    opts.Stack,
		ComponentFilter: componentName,
	}

	// For vars command (using .vars query), we clear the Component field
	// to let the system determine the correct query path.
	if filterOptions.Query == ".vars" {
		filterOptions.Component = ""
	} else {
		filterOptions.Component = componentName
	}

	// Initialize Atmos config and get stacks.
	processingFlags := &fl.ProcessingFlags{
		Templates: opts.ProcessTemplates,
		Functions: opts.ProcessYamlFunctions,
	}
	_, stacksMap, err := initAtmosAndDescribeStacksForList(componentName, processingFlags)
	if err != nil {
		return "", err
	}

	// Log the filter options.
	log.Debug("Filtering values",
		"component", componentName,
		"componentFilter", filterOptions.ComponentFilter,
		"query", filterOptions.Query,
		"includeAbstract", filterOptions.IncludeAbstract,
		"maxColumns", filterOptions.MaxColumns,
		"format", filterOptions.FormatStr,
		"stackPattern", filterOptions.StackPattern,
		"processTemplates", processingFlags.Templates,
		"processYamlFunctions", processingFlags.Functions)

	// Filter and list component values across stacks.
	output, err := l.FilterAndListValues(stacksMap, filterOptions)
	if err != nil {
		var noValuesErr *listerrors.NoValuesFoundError
		if errors.As(err, &noValuesErr) {
			logNoValuesFoundMessage(componentName, filterOptions.Query)
			return "", nil
		}
		return "", err
	}

	return output, nil
}
