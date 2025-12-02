package list

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	e "github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/auth"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/flags"
	l "github.com/cloudposse/atmos/pkg/list"
	listerrors "github.com/cloudposse/atmos/pkg/list/errors"
	f "github.com/cloudposse/atmos/pkg/list/format"
	listutils "github.com/cloudposse/atmos/pkg/list/utils"
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

// getIdentityFromCommand gets the identity value from --identity flag or ATMOS_IDENTITY env var.
// The --identity flag is inherited from the parent list command.
// Returns empty string if no identity is specified.
//
// Note: This is a simplified version of cmd.GetIdentityFromFlags that doesn't need
// to handle the NoOptDefVal quirk because list commands use persistent flags.
// We can't import cmd.GetIdentityFromFlags due to import cycle (cmd imports cmd/list).
func getIdentityFromCommand(cmd *cobra.Command) string {
	var value string

	// Check if flag was explicitly set.
	if cmd.Flags().Changed("identity") {
		value, _ = cmd.Flags().GetString("identity")
	} else {
		// Fall back to environment variable via Viper.
		value = viper.GetString("identity")
	}

	// Normalize boolean false representations to disabled sentinel value.
	return normalizeIdentityValue(value)
}

// normalizeIdentityValue converts boolean false representations to the disabled sentinel value.
// Recognizes: false, False, FALSE, 0, no, No, NO, off, Off, OFF.
// All other values are returned unchanged.
func normalizeIdentityValue(value string) string {
	if value == "" {
		return ""
	}

	switch strings.ToLower(value) {
	case "false", "0", "no", "off":
		return cfg.IdentityFlagDisabledValue
	default:
		return value
	}
}

// createAuthManagerForList creates an AuthManager for list commands.
// It uses the identity from --identity flag or ATMOS_IDENTITY env var.
// If no identity is specified, it loads stack configs for default identity.
// Returns nil AuthManager if no auth is configured (which is valid for many use cases).
func createAuthManagerForList(cmd *cobra.Command, atmosConfig *schema.AtmosConfiguration) (auth.AuthManager, error) {
	identityName := getIdentityFromCommand(cmd)

	// Create AuthManager with stack-level default identity loading.
	// When identityName is empty, this loads stack configs for auth.identities.*.default: true.
	authManager, err := auth.CreateAndAuthenticateManagerWithAtmosConfig(
		identityName,
		&atmosConfig.Auth,
		cfg.IdentityFlagSelectValue,
		atmosConfig,
	)
	if err != nil {
		return nil, err
	}

	return authManager, nil
}

// setDefaultCSVDelimiter sets the delimiter to comma if CSV format is used and delimiter is default TSV.
func setDefaultCSVDelimiter(delimiter *string, format string) {
	if f.Format(format) == f.FormatCSV && *delimiter == f.DefaultTSVDelimiter {
		*delimiter = f.DefaultCSVDelimiter
	}
}

// getComponentFilter extracts the component filter from command arguments.
func getComponentFilter(args []string) string {
	if len(args) > 0 {
		return args[0]
	}
	return ""
}

// initConfigAndAuth initializes CLI config and creates an auth manager.
func initConfigAndAuth(cmd *cobra.Command) (schema.AtmosConfiguration, auth.AuthManager, error) {
	configAndStacksInfo := schema.ConfigAndStacksInfo{}
	atmosConfig, err := cfg.InitCliConfig(configAndStacksInfo, true)
	if err != nil {
		return schema.AtmosConfiguration{}, nil, &listerrors.InitConfigError{Cause: err}
	}

	authManager, err := createAuthManagerForList(cmd, &atmosConfig)
	if err != nil {
		return schema.AtmosConfiguration{}, nil, err
	}

	return atmosConfig, authManager, nil
}

// validateComponentFilter validates that the component exists if a filter is specified.
func validateComponentFilter(atmosConfig *schema.AtmosConfiguration, componentFilter string) error {
	if componentFilter != "" && !listutils.CheckComponentExists(atmosConfig, componentFilter) {
		return &listerrors.ComponentDefinitionNotFoundError{Component: componentFilter}
	}
	return nil
}

// handleNoValuesError handles the NoValuesFoundError by logging an appropriate message.
// LogFunc is called with the componentFilter when no values are found.
func handleNoValuesError(err error, componentFilter string, logFunc func(string)) (string, error) {
	var noValuesErr *listerrors.NoValuesFoundError
	if errors.As(err, &noValuesErr) {
		logFunc(componentFilter)
		return "", nil
	}
	return "", err
}
