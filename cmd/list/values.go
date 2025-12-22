package list

import (
	"errors"
	"fmt"

	log "github.com/cloudposse/atmos/pkg/logger"
	pkgerrors "github.com/pkg/errors"
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
	"github.com/cloudposse/atmos/pkg/ui"
	u "github.com/cloudposse/atmos/pkg/utils"
)

var (
	ErrGettingCommonFlags    = pkgerrors.New("error getting common flags")
	ErrGettingAbstractFlag   = pkgerrors.New("error getting abstract flag")
	ErrGettingVarsFlag       = pkgerrors.New("error getting vars flag")
	ErrInitializingCLIConfig = pkgerrors.New("error initializing CLI config")
	ErrDescribingStacks      = pkgerrors.New("error describing stacks")
	ErrComponentNameRequired = pkgerrors.New("component name is required")
	ErrInvalidArguments      = pkgerrors.New("invalid arguments: the command requires one argument 'component'")
)

// Error format strings.
const (
	ErrFmtWrapErr = "%w: %v" // Format for wrapping errors.
)

var (
	valuesParser *flags.StandardParser
	varsParser   *flags.StandardParser
)

// ValuesOptions contains parsed flags for the values command.
type ValuesOptions struct {
	global.Flags
	Format           string
	MaxColumns       int
	Delimiter        string
	Stack            string
	Query            string
	Abstract         bool
	Vars             bool
	ProcessTemplates bool
	ProcessFunctions bool
}

// valuesCmd lists component values across stacks.
var valuesCmd = &cobra.Command{
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

		// Get Viper instance for flag/env precedence.
		v := viper.GetViper()

		// Check Atmos configuration (honors --base-path, --config, --config-path, --profile).
		if err := checkAtmosConfig(cmd, v); err != nil {
			return err
		}

		// Parse flags using StandardParser with Viper precedence.
		if err := valuesParser.BindFlagsToViper(cmd, v); err != nil {
			return err
		}

		opts := &ValuesOptions{
			Flags:            flags.ParseGlobalFlags(cmd, v),
			Format:           v.GetString("format"),
			MaxColumns:       v.GetInt("max-columns"),
			Delimiter:        v.GetString("delimiter"),
			Stack:            v.GetString("stack"),
			Query:            v.GetString("query"),
			Abstract:         v.GetBool("abstract"),
			Vars:             v.GetBool("vars"),
			ProcessTemplates: v.GetBool("process-templates"),
			ProcessFunctions: v.GetBool("process-functions"),
		}

		output, err := listValuesWithOptions(cmd, opts, args)
		if err != nil {
			return err
		}

		u.PrintMessage(output)
		return nil
	},
}

// varsCmd is an alias for 'list values --query .vars'.
var varsCmd = &cobra.Command{
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
		// Get Viper instance for flag/env precedence.
		v := viper.GetViper()

		// Check Atmos configuration (honors --base-path, --config, --config-path, --profile).
		if err := checkAtmosConfig(cmd, v); err != nil {
			return err
		}

		// Parse flags using StandardParser with Viper precedence.
		if err := varsParser.BindFlagsToViper(cmd, v); err != nil {
			return err
		}

		opts := &ValuesOptions{
			Flags:            flags.ParseGlobalFlags(cmd, v),
			Format:           v.GetString("format"),
			MaxColumns:       v.GetInt("max-columns"),
			Delimiter:        v.GetString("delimiter"),
			Stack:            v.GetString("stack"),
			Query:            ".vars", // Always set to .vars for vars command
			Abstract:         v.GetBool("abstract"),
			Vars:             false,
			ProcessTemplates: v.GetBool("process-templates"),
			ProcessFunctions: v.GetBool("process-functions"),
		}

		output, err := listValuesWithOptions(cmd, opts, args)
		if err != nil {
			var componentVarsNotFoundErr *listerrors.ComponentVarsNotFoundError
			if errors.As(err, &componentVarsNotFoundErr) {
				_ = ui.Info("No vars found for component: " + componentVarsNotFoundErr.Component)
				return nil
			}

			var noValuesErr *listerrors.NoValuesFoundError
			if errors.As(err, &noValuesErr) {
				_ = ui.Info("No values found for query '.vars' for component: " + args[0])
				return nil
			}

			return err
		}

		u.PrintMessage(output)
		return nil
	},
}

func init() {
	// Create parser for values command using flag wrappers.
	valuesParser = NewListParser(
		WithFormatFlag,
		WithDelimiterFlag,
		WithStackFlag,
		WithQueryFlag,
		WithMaxColumnsFlag,
		WithAbstractFlag,
		WithProcessTemplatesFlag,
		WithProcessFunctionsFlag,
		// Add vars flag only for values command.
		func(options *[]flags.Option) {
			*options = append(*options,
				flags.WithBoolFlag("vars", "", false, "Show only vars (equivalent to --query .vars)"),
				flags.WithEnvVars("vars", "ATMOS_LIST_VARS"),
			)
		},
	)

	// Register flags for values command.
	valuesParser.RegisterFlags(valuesCmd)

	// Customize query flag usage for values command.
	if queryFlag := valuesCmd.PersistentFlags().Lookup("query"); queryFlag != nil {
		queryFlag.Usage = "Filter the results using YQ expressions"
	}

	// Add stack completion.
	addStackCompletion(valuesCmd)

	// Bind flags to Viper for environment variable support.
	if err := valuesParser.BindToViper(viper.GetViper()); err != nil {
		panic(err)
	}

	// Create parser for vars command (no vars flag, as it's always .vars).
	varsParser = NewListParser(
		WithFormatFlag,
		WithDelimiterFlag,
		WithStackFlag,
		WithQueryFlag,
		WithMaxColumnsFlag,
		WithAbstractFlag,
		WithProcessTemplatesFlag,
		WithProcessFunctionsFlag,
	)

	// Register flags for vars command.
	varsParser.RegisterFlags(varsCmd)

	// Customize query flag usage for vars command.
	if queryFlag := varsCmd.PersistentFlags().Lookup("query"); queryFlag != nil {
		queryFlag.Usage = "Filter the results using YQ expressions"
	}

	// Add stack completion.
	addStackCompletion(varsCmd)

	// Bind flags to Viper for environment variable support.
	if err := varsParser.BindToViper(viper.GetViper()); err != nil {
		panic(err)
	}
}

// getBoolFlagWithDefault gets a boolean flag value or returns the default if error.
func getBoolFlagWithDefault(cmd *cobra.Command, flagName string, defaultValue bool) bool {
	if cmd.Flags().Lookup(flagName) == nil {
		return defaultValue
	}

	value, err := cmd.Flags().GetBool(flagName)
	if err != nil {
		log.Warn("Failed to get flag, using default",
			"flag", flagName,
			"default", defaultValue,
			"error", err)
		return defaultValue
	}

	return value
}

// getFilterOptionsFromValues converts ValuesOptions to FilterOptions.
func getFilterOptionsFromValues(opts *ValuesOptions) *l.FilterOptions {
	query := opts.Query
	if opts.Vars {
		query = ".vars"
	}

	// Set appropriate default delimiter based on format
	delimiter := opts.Delimiter
	if f.Format(opts.Format) == f.FormatCSV && delimiter == f.DefaultTSVDelimiter {
		delimiter = f.DefaultCSVDelimiter
	}

	return &l.FilterOptions{
		Query:           query,
		IncludeAbstract: opts.Abstract,
		MaxColumns:      opts.MaxColumns,
		FormatStr:       opts.Format,
		Delimiter:       delimiter,
		StackPattern:    opts.Stack,
	}
}

// displayNoValuesFoundMessage displays an appropriate message when no values or vars are found.
func displayNoValuesFoundMessage(componentName string, query string) {
	if query == ".vars" {
		_ = ui.Info("No vars found for component: " + componentName)
	} else {
		_ = ui.Info("No values found for component: " + componentName)
	}
}

// prepareListValuesOptions prepares filter options based on component name and ValuesOptions.
func prepareListValuesOptions(opts *ValuesOptions, componentName string) *l.FilterOptions {
	filterOptions := getFilterOptionsFromValues(opts)

	// Use ComponentFilter instead of Component for filtering by component name
	// This allows the extractComponentValues function to handle it properly
	filterOptions.ComponentFilter = componentName

	// For vars command (using .vars query), we clear the Component field
	// to let the system determine the correct query path
	if filterOptions.Query == ".vars" {
		// Using ComponentFilter with empty Component
		// lets the system build the correct YQ expression
		filterOptions.Component = ""
	} else {
		// For other cases where we're not querying vars, use the standard approach
		filterOptions.Component = componentName
	}

	// Log the component name
	log.Debug("Processing component",
		"component", componentName,
		"component_field", filterOptions.Component,
		"component_filter", filterOptions.ComponentFilter)

	return filterOptions
}

func listValuesWithOptions(cmd *cobra.Command, opts *ValuesOptions, args []string) (string, error) {
	// Ensure we have a component name
	if len(args) == 0 {
		return "", ErrComponentNameRequired
	}
	componentName := args[0]

	// Initialize CLI config
	configAndStacksInfo := schema.ConfigAndStacksInfo{}
	atmosConfig, err := config.InitCliConfig(configAndStacksInfo, true)
	if err != nil {
		return "", fmt.Errorf(ErrFmtWrapErr, ErrInitializingCLIConfig, err)
	}

	// Create AuthManager for authentication support.
	authManager, err := createAuthManagerForList(cmd, &atmosConfig)
	if err != nil {
		return "", err
	}

	// Check if the component exists
	if !listutils.CheckComponentExists(&atmosConfig, componentName) {
		return "", &listerrors.ComponentDefinitionNotFoundError{Component: componentName}
	}

	// Get all stacks
	stacksMap, err := e.ExecuteDescribeStacks(&atmosConfig, "", nil, nil, nil, false, opts.ProcessTemplates, opts.ProcessFunctions, false, nil, authManager)
	if err != nil {
		return "", fmt.Errorf(ErrFmtWrapErr, ErrDescribingStacks, err)
	}

	// Prepare filter options
	filterOptions := prepareListValuesOptions(opts, componentName)

	// Log the filter options
	log.Debug("Filtering values",
		"component", componentName,
		"componentFilter", filterOptions.ComponentFilter,
		"query", filterOptions.Query,
		"includeAbstract", filterOptions.IncludeAbstract,
		"maxColumns", filterOptions.MaxColumns,
		"format", filterOptions.FormatStr,
		"stackPattern", filterOptions.StackPattern,
		"processTemplates", opts.ProcessTemplates,
		"processYamlFunctions", opts.ProcessFunctions)

	// Filter and list component values across stacks
	output, err := l.FilterAndListValues(stacksMap, filterOptions)
	if err != nil {
		var noValuesErr *listerrors.NoValuesFoundError
		if errors.As(err, &noValuesErr) {
			displayNoValuesFoundMessage(componentName, filterOptions.Query)
			return "", nil
		}
		return "", err
	}

	return output, nil
}
