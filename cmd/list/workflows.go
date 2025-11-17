package list

import (
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/flags/global"
	l "github.com/cloudposse/atmos/pkg/list"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui/theme"
	u "github.com/cloudposse/atmos/pkg/utils"
)

var workflowsParser *flags.StandardParser

// WorkflowsOptions contains parsed flags for the workflows command.
type WorkflowsOptions struct {
	global.Flags
	File      string
	Format    string
	Delimiter string
}

// workflowsCmd lists atmos workflows.
var workflowsCmd = &cobra.Command{
	Use:   "workflows",
	Short: "List all Atmos workflows",
	Long:  "List Atmos workflows, with options to filter results by specific files.",
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := checkAtmosConfig(true); err != nil {
			return err
		}

		// Parse flags using StandardParser with Viper precedence
		v := viper.GetViper()
		if err := workflowsParser.BindFlagsToViper(cmd, v); err != nil {
			return err
		}

		opts := &WorkflowsOptions{
			Flags:     flags.ParseGlobalFlags(cmd, v),
			File:      v.GetString("file"),
			Format:    v.GetString("format"),
			Delimiter: v.GetString("delimiter"),
		}

		output, err := listWorkflowsWithOptions(opts)
		if err != nil {
			return err
		}

		u.PrintMessageInColor(output, theme.Colors.Success)
		return nil
	},
}

func init() {
	// Create parser with workflows-specific flags using functional options
	workflowsParser = flags.NewStandardParser(
		flags.WithStringFlag("file", "f", "", "Filter workflows by file (e.g., atmos list workflows -f workflow1)"),
		flags.WithStringFlag("format", "", "", "Output format (table, json, csv)"),
		flags.WithStringFlag("delimiter", "", "\t", "Delimiter for csv output"),
		flags.WithEnvVars("file", "ATMOS_WORKFLOW_FILE"),
		flags.WithEnvVars("format", "ATMOS_LIST_FORMAT"),
		flags.WithEnvVars("delimiter", "ATMOS_LIST_DELIMITER"),
	)

	// Register flags
	workflowsParser.RegisterFlags(workflowsCmd)

	// Bind flags to Viper for environment variable support
	if err := workflowsParser.BindToViper(viper.GetViper()); err != nil {
		panic(err)
	}
}

func listWorkflowsWithOptions(opts *WorkflowsOptions) (string, error) {
	configAndStacksInfo := schema.ConfigAndStacksInfo{}
	atmosConfig, err := config.InitCliConfig(configAndStacksInfo, false)
	if err != nil {
		return "", err
	}

	return l.FilterAndListWorkflows(opts.File, atmosConfig.Workflows.List, opts.Format, opts.Delimiter)
}
