package list

import (
	"fmt"
	"path/filepath"

	"github.com/spf13/cobra"

	e "github.com/cloudposse/atmos/internal/exec"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/flags"
	l "github.com/cloudposse/atmos/pkg/list"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// checkAtmosConfig verifies that Atmos is properly configured.
// Returns an error instead of calling Exit to allow proper error handling in tests.
func checkAtmosConfig(skipStackCheck ...bool) error {
	atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, false)
	if err != nil {
		return err
	}

	// Allow skipping stack validation for commands that don't need it (e.g., workflows)
	if len(skipStackCheck) > 0 && skipStackCheck[0] {
		return nil
	}

	atmosConfigExists, err := u.IsDirectory(atmosConfig.StacksBaseAbsolutePath)
	if !atmosConfigExists || err != nil {
		return fmt.Errorf("atmos stacks directory not found at: %s", filepath.Join(atmosConfig.BasePath, atmosConfig.Stacks.BasePath))
	}

	return nil
}

// addStackCompletion adds the --stack flag with shell completion to a command.
func addStackCompletion(cobraCmd *cobra.Command) {
	if cobraCmd.Flag("stack") == nil {
		cobraCmd.PersistentFlags().StringP("stack", "s", "", "Filter by stack name or pattern")
	}
	cobraCmd.RegisterFlagCompletionFunc("stack", stackFlagCompletion)
}

// stackFlagCompletion provides shell completion for the --stack flag.
func stackFlagCompletion(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	// If a component was provided as the first argument, filter stacks by that component
	if len(args) > 0 && args[0] != "" {
		output, err := listStacksForComponent(args[0])
		if err != nil {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
		return output, cobra.ShellCompDirectiveNoFileComp
	}

	// Otherwise, list all stacks
	output, err := listAllStacks()
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	return output, cobra.ShellCompDirectiveNoFileComp
}

// listStacksForComponent returns stacks that contain the specified component.
func listStacksForComponent(component string) ([]string, error) {
	configAndStacksInfo := schema.ConfigAndStacksInfo{}
	atmosConfig, err := cfg.InitCliConfig(configAndStacksInfo, true)
	if err != nil {
		return nil, fmt.Errorf("%w", err)
	}

	stacksMap, err := e.ExecuteDescribeStacks(&atmosConfig, "", nil, nil, nil, false, false, false, false, nil, nil)
	if err != nil {
		return nil, fmt.Errorf("%w", err)
	}

	output, err := l.FilterAndListStacks(stacksMap, component)
	return output, err
}

// listAllStacks returns all available stacks.
func listAllStacks() ([]string, error) {
	configAndStacksInfo := schema.ConfigAndStacksInfo{}
	atmosConfig, err := cfg.InitCliConfig(configAndStacksInfo, true)
	if err != nil {
		return nil, fmt.Errorf("error initializing CLI config: %v", err)
	}

	stacksMap, err := e.ExecuteDescribeStacks(&atmosConfig, "", nil, nil, nil, false, false, false, false, nil, nil)
	if err != nil {
		return nil, fmt.Errorf("error describing stacks: %v", err)
	}

	output, err := l.FilterAndListStacks(stacksMap, "")
	return output, err
}

// newCommonListParser creates a StandardParser with common list flags.
// This replaces the pkg/list/flags.AddCommonListFlags pattern.
func newCommonListParser(additionalOptions ...flags.Option) *flags.StandardParser {
	// Start with common list flags
	options := []flags.Option{
		flags.WithStringFlag("format", "", "", "Output format: table, json, yaml, csv, tsv"),
		flags.WithIntFlag("max-columns", "", 0, "Maximum number of columns to display"),
		flags.WithStringFlag("delimiter", "", "", "Delimiter for CSV/TSV output"),
		flags.WithStringFlag("stack", "s", "", "Stack pattern to filter by"),
		flags.WithStringFlag("query", "", "", "YQ expression to filter values (e.g., .vars.region)"),
		flags.WithEnvVars("format", "ATMOS_LIST_FORMAT"),
		flags.WithEnvVars("delimiter", "ATMOS_LIST_DELIMITER"),
		flags.WithEnvVars("stack", "ATMOS_STACK"),
		flags.WithEnvVars("query", "ATMOS_LIST_QUERY"),
	}

	// Append any additional flags
	options = append(options, additionalOptions...)

	return flags.NewStandardParser(options...)
}
