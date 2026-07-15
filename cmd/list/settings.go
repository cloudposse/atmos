package list

import (
	"errors"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	e "github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/data"
	"github.com/cloudposse/atmos/pkg/degradation"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/flags/global"
	l "github.com/cloudposse/atmos/pkg/list"
	listerrors "github.com/cloudposse/atmos/pkg/list/errors"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/ui"
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
	ErrorMode        string
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
			ErrorMode:        v.GetString("error-mode"),
		}

		output, collector, err := listSettingsWithOptions(cmd, v, opts, args)
		if err != nil {
			return err
		}
		defer printErrorModeSummary(opts.ErrorMode, collector)

		return data.Writeln(output)
	},
}

func init() {
	// Create parser with common list flags plus processing flags
	settingsParser = newCommonListParser(
		flags.WithBoolFlag("process-templates", "", true, "Enable/disable Go template processing in Atmos stack manifests when executing the command"),
		flags.WithBoolFlag("process-functions", "", true, "Enable/disable YAML functions processing in Atmos stack manifests when executing the command"),
		flags.WithEnvVars("process-templates", "ATMOS_PROCESS_TEMPLATES"),
		flags.WithEnvVars("process-functions", "ATMOS_PROCESS_FUNCTIONS"),
		flags.WithStringFlag("error-mode", "", "", "How to handle recoverable errors (e.g. a Terraform backend not yet provisioned): warn (degrade + summary, default), silent (degrade, no summary), or strict (fail immediately). Defaults to atmos.yaml's list.error_mode, or warn"),
		flags.WithEnvVars("error-mode", "ATMOS_LIST_ERROR_MODE"),
		flags.WithValidValues("error-mode", "strict", "warn", "silent"),
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
		ui.Info("No settings found for component: " + componentFilter)
	} else {
		ui.Info("No settings found")
	}
}

// listSettingsWithOptions returns the rendered output, the degradation.Collector used
// during describe-stacks processing (nil unless opts.ErrorMode is "warn"/"silent"), and an
// error. Callers should print the collector's summary (see printErrorModeSummary) only
// after writing output, so the end-of-command warning appears after the data.
func listSettingsWithOptions(cmd *cobra.Command, v *viper.Viper, opts *SettingsOptions, args []string) (string, *degradation.Collector, error) {
	// Set default delimiter for CSV.
	setDefaultCSVDelimiter(&opts.Delimiter, opts.Format)

	componentFilter := getComponentFilter(args)

	// Initialize CLI config and auth manager (honors --base-path, --config, --config-path, --profile).
	atmosConfig, authManager, err := initConfigAndAuth(cmd, v)
	if err != nil {
		return "", nil, err
	}

	// Validate component exists if filter is specified.
	if err := validateComponentFilter(&atmosConfig, componentFilter); err != nil {
		return "", nil, err
	}

	// Resolve --error-mode: explicit flag/env value wins, else atmos.yaml's
	// list.error_mode, else "warn".
	opts.ErrorMode = e.ResolveErrorMode(opts.ErrorMode, atmosConfig.List.ErrorMode)

	// Execute describe stacks.
	errOpts, collector := describeStacksErrorOptions(opts.ErrorMode)
	stacksMap, err := e.ExecuteDescribeStacksWithOptions(&atmosConfig, "", nil, nil, nil, false,
		opts.ProcessTemplates, opts.ProcessFunctions, false, nil, authManager, false,
		errOpts)
	if err != nil {
		return "", nil, &listerrors.DescribeStacksError{Cause: err}
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
			return "", collector, nil
		}
		return "", nil, err
	}

	return output, collector, nil
}
