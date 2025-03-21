package cmd

import (
	"fmt"

	log "github.com/charmbracelet/log"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"

	e "github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/config"
	l "github.com/cloudposse/atmos/pkg/list"
	fl "github.com/cloudposse/atmos/pkg/list/flags"
	f "github.com/cloudposse/atmos/pkg/list/format"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

var (
	ErrGettingCommonFlags    = errors.New("error getting common flags")
	ErrGettingAbstractFlag   = errors.New("error getting abstract flag")
	ErrGettingVarsFlag       = errors.New("error getting vars flag")
	ErrInitializingCLIConfig = errors.New("error initializing CLI config")
	ErrDescribingStacks      = errors.New("error describing stacks")
	ErrComponentNameRequired = errors.New("component name is required")
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
	Run: func(cmd *cobra.Command, args []string) {
		if len(args) != 1 {
			log.Error("invalid arguments. The command requires one argument 'component'")
			return
		}

		// Check Atmos configuration
		checkAtmosConfig()
		output, err := listValues(cmd, args)
		if err != nil {
			log.Error(err.Error())
			return
		}

		u.PrintMessage(output)
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
	Run: func(cmd *cobra.Command, args []string) {
		// Check Atmos configuration
		checkAtmosConfig()

		// Set the query flag to .vars
		if err := cmd.Flags().Set("query", ".vars"); err != nil {
			log.Error("failed to set query flag", "error", err)
			return
		}

		// Run listValues with the component argument
		output, err := listValues(cmd, args)
		if err != nil {
			// Use IsNoValuesFoundError instead of string matching
			if l.IsNoValuesFoundError(err) && len(args) > 0 {
				log.Error("no values found for component with query",
					"component", args[0],
					"query", ".vars")
				return
			}
			log.Error(err.Error())
			return
		}

		u.PrintMessage(output)
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
		log.Warn("failed to get flag, using default",
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

func listValues(cmd *cobra.Command, args []string) (string, error) {
	// Ensure we have a component name
	if len(args) == 0 {
		return "", ErrComponentNameRequired
	}
	component := args[0]

	// Get all flags and options
	filterOptions, processingFlags, err := getListValuesFlags(cmd)
	if err != nil {
		return "", err
	}

	// Set component in filter options
	filterOptions.Component = component

	// Log the component name
	log.Debug("Processing component", "component", component)

	// Initialize CLI config
	configAndStacksInfo := schema.ConfigAndStacksInfo{}
	atmosConfig, err := config.InitCliConfig(configAndStacksInfo, true)
	if err != nil {
		return "", fmt.Errorf(ErrFmtWrapErr, ErrInitializingCLIConfig, err)
	}

	// Get all stacks
	stacksMap, err := e.ExecuteDescribeStacks(atmosConfig, "", nil, nil, nil, false, processingFlags.Templates, processingFlags.Functions, false, nil)
	if err != nil {
		return "", fmt.Errorf(ErrFmtWrapErr, ErrDescribingStacks, err)
	}

	// Log the filter options
	log.Info("Filtering values",
		"component", component,
		"query", filterOptions.Query,
		"includeAbstract", filterOptions.IncludeAbstract,
		"maxColumns", filterOptions.MaxColumns,
		"format", filterOptions.FormatStr,
		"stackPattern", filterOptions.StackPattern,
		"processTemplates", processingFlags.Templates,
		"processYamlFunctions", processingFlags.Functions)

	// Filter and list component values across stacks
	return l.FilterAndListValues(stacksMap, filterOptions)
}
