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
)

// Error format strings.
const (
	ErrFmtWrapErr = "%w: %v" // Format for wrapping errors.
)

// listValuesCmd lists component values across stacks.
var listValuesCmd = &cobra.Command{
	Use:   "values [component]",
	Short: "List component values across stacks",
	Long:  "List values for a component across all stacks where it is used",
	Example: "atmos list values vpc\n" +
		"atmos list values vpc --abstract\n" +
		"atmos list values vpc --query .vars\n" +
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
	Short: "List component vars across stacks (alias for 'list values --query .vars')",
	Long:  "List vars for a component across all stacks where it is used",
	Example: "atmos list vars vpc\n" +
		"atmos list vars vpc --abstract\n" +
		"atmos list vars vpc --max-columns 5\n" +
		"atmos list vars vpc --format json\n" +
		"atmos list vars vpc --format yaml\n" +
		"atmos list vars vpc --format csv",
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		// Set the query flag to .vars
		if err := cmd.Flags().Set("query", ".vars"); err != nil {
			log.Error("failed to set query flag", "error", err, "component", args[0])
			return
		}
		// Run the values command
		listValuesCmd.Run(cmd, args)
	},
}

func init() {
	// Add common flags
	fl.AddCommonListFlags(listValuesCmd)

	// Add additional flags
	listValuesCmd.PersistentFlags().Bool("abstract", false, "Include abstract components")
	listValuesCmd.PersistentFlags().Bool("vars", false, "Show only vars (equivalent to --query .vars)")

	// Add stack pattern completion
	AddStackCompletion(listValuesCmd)

	// Add commands to list command
	listCmd.AddCommand(listValuesCmd)
	listCmd.AddCommand(listVarsCmd)
}

func listValues(cmd *cobra.Command, args []string) (string, error) {
	// Get common flags
	commonFlags, err := fl.GetCommonListFlags(cmd)
	if err != nil {
		return "", fmt.Errorf(ErrFmtWrapErr, ErrGettingCommonFlags, err)
	}

	// Get additional flags
	abstractFlag, err := cmd.Flags().GetBool("abstract")
	if err != nil {
		return "", fmt.Errorf(ErrFmtWrapErr, ErrGettingAbstractFlag, err)
	}

	varsFlag, err := cmd.Flags().GetBool("vars")
	if err != nil {
		return "", fmt.Errorf(ErrFmtWrapErr, ErrGettingVarsFlag, err)
	}

	// Set appropriate default delimiter based on format
	if f.Format(commonFlags.Format) == f.FormatCSV && commonFlags.Delimiter == f.DefaultTSVDelimiter {
		commonFlags.Delimiter = f.DefaultCSVDelimiter
	}

	// If vars flag is set, override query
	if varsFlag {
		commonFlags.Query = ".vars"
	}

	component := args[0]

	// Initialize CLI config
	configAndStacksInfo := schema.ConfigAndStacksInfo{}
	atmosConfig, err := config.InitCliConfig(configAndStacksInfo, true)
	if err != nil {
		return "", fmt.Errorf(ErrFmtWrapErr, ErrInitializingCLIConfig, err)
	}

	// Get all stacks
	stacksMap, err := e.ExecuteDescribeStacks(atmosConfig, "", nil, nil, nil, false, false, false, false, nil)
	if err != nil {
		return "", fmt.Errorf(ErrFmtWrapErr, ErrDescribingStacks, err)
	}

	// Filter and list component values across stacks
	output, err := l.FilterAndListValues(stacksMap, &l.FilterOptions{
		Component:       component,
		Query:           commonFlags.Query,
		IncludeAbstract: abstractFlag,
		MaxColumns:      commonFlags.MaxColumns,
		FormatStr:       commonFlags.Format,
		Delimiter:       commonFlags.Delimiter,
		StackPattern:    commonFlags.Stack,
	})
	if err != nil {
		return "", err // Return error directly without wrapping
	}

	return output, nil
}
