package list

import (
	"errors"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	e "github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/flags/global"
	l "github.com/cloudposse/atmos/pkg/list"
	listerrors "github.com/cloudposse/atmos/pkg/list/errors"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/ui"
	utils "github.com/cloudposse/atmos/pkg/utils"
)

var settingsParser *flags.StandardParser

// SettingsOptions contains parsed flags for the settings command.
type SettingsOptions struct {
	global.Flags
	Format           string
	MaxColumns       int
	Delimiter        string
	Stack            string
	Query            string
	ProcessTemplates bool
	ProcessFunctions bool
}

// settingsCmd lists settings across stacks.
var settingsCmd = &cobra.Command{
	Use:   "settings [component]",
	Short: "List settings across stacks or for a specific component",
	Long:  "List settings configuration across all stacks or for a specific component",
	Example: "atmos list settings\n" +
		"atmos list settings c1\n" +
		"atmos list settings --query .terraform\n" +
		"atmos list settings --format json\n" +
		"atmos list settings --stack '*-dev-*'\n" +
		"atmos list settings --stack 'prod-*'",
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		// Get Viper instance for flag/env precedence.
		v := viper.GetViper()

		// Check Atmos configuration (honors --base-path, --config, --config-path, --profile).
		if err := checkAtmosConfig(cmd, v); err != nil {
			return err
		}

		// Parse flags using StandardParser with Viper precedence.
		if err := settingsParser.BindFlagsToViper(cmd, v); err != nil {
			return err
		}

		opts := &SettingsOptions{
			Flags:            flags.ParseGlobalFlags(cmd, v),
			Format:           v.GetString("format"),
			MaxColumns:       v.GetInt("max-columns"),
			Delimiter:        v.GetString("delimiter"),
			Stack:            v.GetString("stack"),
			Query:            v.GetString("query"),
			ProcessTemplates: v.GetBool("process-templates"),
			ProcessFunctions: v.GetBool("process-functions"),
		}

		output, err := listSettingsWithOptions(cmd, v, opts, args)
		if err != nil {
			return err
		}

		utils.PrintMessage(output)
		return nil
	},
}

func init() {
	// Create parser with common list flags plus processing flags
	settingsParser = newCommonListParser(
		flags.WithBoolFlag("process-templates", "", true, "Enable/disable Go template processing in Atmos stack manifests when executing the command"),
		flags.WithBoolFlag("process-functions", "", true, "Enable/disable YAML functions processing in Atmos stack manifests when executing the command"),
		flags.WithEnvVars("process-templates", "ATMOS_PROCESS_TEMPLATES"),
		flags.WithEnvVars("process-functions", "ATMOS_PROCESS_FUNCTIONS"),
	)

	// Register flags
	settingsParser.RegisterFlags(settingsCmd)

	// Add stack completion
	addStackCompletion(settingsCmd)

	// Bind flags to Viper for environment variable support
	if err := settingsParser.BindToViper(viper.GetViper()); err != nil {
		panic(err)
	}
}

// setupSettingsOptions sets up the filter options for settings listing.
func setupSettingsOptions(opts *SettingsOptions, componentFilter string) *l.FilterOptions {
	return &l.FilterOptions{
		Component:       "settings",
		ComponentFilter: componentFilter,
		Query:           opts.Query,
		IncludeAbstract: false,
		MaxColumns:      opts.MaxColumns,
		FormatStr:       opts.Format,
		Delimiter:       opts.Delimiter,
		StackPattern:    opts.Stack,
	}
}

// displayNoSettingsFoundMessage displays an appropriate message when no settings are found.
func displayNoSettingsFoundMessage(componentFilter string) {
	if componentFilter != "" {
		_ = ui.Info("No settings found for component: " + componentFilter)
	} else {
		_ = ui.Info("No settings found")
	}
}

func listSettingsWithOptions(cmd *cobra.Command, v *viper.Viper, opts *SettingsOptions, args []string) (string, error) {
	// Set default delimiter for CSV.
	setDefaultCSVDelimiter(&opts.Delimiter, opts.Format)

	componentFilter := getComponentFilter(args)

	// Initialize CLI config and auth manager (honors --base-path, --config, --config-path, --profile).
	atmosConfig, authManager, err := initConfigAndAuth(cmd, v)
	if err != nil {
		return "", err
	}

	// Validate component exists if filter is specified.
	if err := validateComponentFilter(&atmosConfig, componentFilter); err != nil {
		return "", err
	}

	// Execute describe stacks.
	stacksMap, err := e.ExecuteDescribeStacks(&atmosConfig, "", nil, nil, nil, false,
		opts.ProcessTemplates, opts.ProcessFunctions, false, nil, authManager)
	if err != nil {
		return "", &listerrors.DescribeStacksError{Cause: err}
	}

	log.Debug("Filtering settings",
		"query", opts.Query, "component", componentFilter,
		"maxColumns", opts.MaxColumns, "format", opts.Format,
		"stackPattern", opts.Stack, "processTemplates", opts.ProcessTemplates,
		"processYamlFunctions", opts.ProcessFunctions)

	filterOptions := setupSettingsOptions(opts, componentFilter)
	output, err := l.FilterAndListValues(stacksMap, filterOptions)
	if err != nil {
		var noValuesErr *listerrors.NoValuesFoundError
		if errors.As(err, &noValuesErr) {
			displayNoSettingsFoundMessage(componentFilter)
			return "", nil
		}
		return "", err
	}

	return output, nil
}
