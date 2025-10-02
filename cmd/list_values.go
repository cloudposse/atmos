package cmd

import (
	"errors"
	"fmt"

	log "github.com/cloudposse/atmos/pkg/logger"
	pkgerrors "github.com/pkg/errors"
	"github.com/spf13/cobra"

	e "github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/config"
	l "github.com/cloudposse/atmos/pkg/list"
	listerrors "github.com/cloudposse/atmos/pkg/list/errors"
	fl "github.com/cloudposse/atmos/pkg/list/flags"
	f "github.com/cloudposse/atmos/pkg/list/format"
	listutils "github.com/cloudposse/atmos/pkg/list/utils"
	"github.com/cloudposse/atmos/pkg/schema"
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

// ProcessingOptions holds flags for processing templates and YAML functions.
type ProcessingOptions struct {
	Templates bool
	Functions bool
}

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

		// Check Atmos configuration
		checkAtmosConfig()

		output, err := listValues(cmd, args)
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
		// Check Atmos configuration
		checkAtmosConfig()

		// Set the query flag to .vars
		if err := cmd.Flags().Set("query", ".vars"); err != nil {
			return fmt.Errorf("failed to set query flag: %w", err)
		}

		output, err := listValues(cmd, args)
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
	// Add common flags
	fl.AddCommonListFlags(listValuesCmd)
	if queryFlag := listValuesCmd.PersistentFlags().Lookup("query"); queryFlag != nil {
		queryFlag.Usage = "Filter the results using YQ expressions"
	}

	// Add additional flags
	listValuesCmd.PersistentFlags().Bool("abstract", false, "Include abstract components")
	listValuesCmd.PersistentFlags().Bool("vars", false, "Show only vars (equivalent to `--query .vars`)")
	listValuesCmd.PersistentFlags().Bool("process-templates", true, "Enable/disable Go template processing in Atmos stack manifests when executing the command")
	listValuesCmd.PersistentFlags().Bool("process-functions", true, "Enable/disable YAML functions processing in Atmos stack manifests when executing the command")

	// Add common flags to vars command
	fl.AddCommonListFlags(listVarsCmd)
	if queryFlag := listVarsCmd.PersistentFlags().Lookup("query"); queryFlag != nil {
		queryFlag.Usage = "Filter the results using YQ expressions"
	}

	// Add abstract flag to vars command
	listVarsCmd.PersistentFlags().Bool("abstract", false, "Include abstract components")
	listVarsCmd.PersistentFlags().Bool("process-templates", true, "Enable/disable Go template processing in Atmos stack manifests when executing the command")
	listVarsCmd.PersistentFlags().Bool("process-functions", true, "Enable/disable YAML functions processing in Atmos stack manifests when executing the command")

	// Add stack pattern completion
	AddStackCompletion(listValuesCmd)
	AddStackCompletion(listVarsCmd)

	// Add commands to list command
	listCmd.AddCommand(listValuesCmd)
	listCmd.AddCommand(listVarsCmd)
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

// getListValuesFlags extracts and processes all flags needed for list values command.
func getListValuesFlags(cmd *cobra.Command) (*l.FilterOptions, *fl.ProcessingFlags, error) {
	// Get common flags
	commonFlags, err := fl.GetCommonListFlags(cmd)
	if err != nil {
		return nil, nil, fmt.Errorf(ErrFmtWrapErr, ErrGettingCommonFlags, err)
	}

	// Get additional flags
	abstractFlag, err := cmd.Flags().GetBool("abstract")
	if err != nil {
		return nil, nil, fmt.Errorf(ErrFmtWrapErr, ErrGettingAbstractFlag, err)
	}

	// Get vars flag and adjust query if needed
	varsFlag := getBoolFlagWithDefault(cmd, "vars", false)
	if varsFlag {
		commonFlags.Query = ".vars"
	}

	// Set appropriate default delimiter based on format
	if f.Format(commonFlags.Format) == f.FormatCSV && commonFlags.Delimiter == f.DefaultTSVDelimiter {
		commonFlags.Delimiter = f.DefaultCSVDelimiter
	}

	// Get processing flags
	processingFlags := fl.GetProcessingFlags(cmd)

	filterOptions := &l.FilterOptions{
		Query:           commonFlags.Query,
		IncludeAbstract: abstractFlag,
		MaxColumns:      commonFlags.MaxColumns,
		FormatStr:       commonFlags.Format,
		Delimiter:       commonFlags.Delimiter,
		StackPattern:    commonFlags.Stack,
	}

	return filterOptions, processingFlags, nil
}

// logNoValuesFoundMessage logs an appropriate message when no values or vars are found.
func logNoValuesFoundMessage(componentName string, query string) {
	if query == ".vars" {
		log.Info("No vars found", "component", componentName)
	} else {
		log.Info("No values found", "component", componentName)
	}
}

// prepareListValuesOptions prepares filter options based on component name and flags.
func prepareListValuesOptions(cmd *cobra.Command, componentName string) (*l.FilterOptions, *fl.ProcessingFlags, error) {
	// Get all flags and options
	filterOptions, processingFlags, err := getListValuesFlags(cmd)
	if err != nil {
		return nil, nil, err
	}

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

	return filterOptions, processingFlags, nil
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
	stacksMap, err := e.ExecuteDescribeStacks(&atmosConfig, "", nil, nil, nil, false, processingFlags.Templates, processingFlags.Functions, false, nil)
	if err != nil {
		return schema.AtmosConfiguration{}, nil, fmt.Errorf(ErrFmtWrapErr, ErrDescribingStacks, err)
	}

	return atmosConfig, stacksMap, nil
}

func listValues(cmd *cobra.Command, args []string) (string, error) {
	// Ensure we have a component name
	if len(args) == 0 {
		return "", ErrComponentNameRequired
	}
	componentName := args[0]

	// Prepare filter options and processing flags
	filterOptions, processingFlags, err := prepareListValuesOptions(cmd, componentName)
	if err != nil {
		return "", err
	}

	// Initialize Atmos config and get stacks
	_, stacksMap, err := initAtmosAndDescribeStacksForList(componentName, processingFlags)
	if err != nil {
		return "", err
	}

	// Log the filter options
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

	// Filter and list component values across stacks
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
