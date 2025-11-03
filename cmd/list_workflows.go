package cmd

import (
	"context"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/flags"
	l "github.com/cloudposse/atmos/pkg/list"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui/theme"
	u "github.com/cloudposse/atmos/pkg/utils"
)

var listWorkflowsParser = flags.NewStandardOptionsBuilder().
	WithFile().
	WithFormat("table", "table", "json", "csv").
	WithDelimiter("\t").
	Build()

// listWorkflowsCmd lists atmos workflows.
var listWorkflowsCmd = &cobra.Command{
	Use:   "workflows",
	Short: "List all Atmos workflows",
	Long:  "List Atmos workflows, with options to filter results by specific files.",
	RunE: func(cmd *cobra.Command, args []string) error {
		checkAtmosConfig(WithStackValidation(false))

		// Parse flags using StandardOptions.
		opts, err := listWorkflowsParser.Parse(context.Background(), args)
		if err != nil {
			return err
		}

		configAndStacksInfo := schema.ConfigAndStacksInfo{}
		atmosConfig, err := config.InitCliConfig(configAndStacksInfo, false)
		if err != nil {
			return err
		}

		output, err := l.FilterAndListWorkflows(opts.File, atmosConfig.Workflows.List, opts.Format, opts.Delimiter)
		if err != nil {
			return err
		}

		u.PrintMessageInColor(output, theme.Colors.Success)
		return nil
	},
}

func init() {
	// Register StandardOptions flags.
	listWorkflowsParser.RegisterFlags(listWorkflowsCmd)
	_ = listWorkflowsParser.BindToViper(viper.GetViper())

	listCmd.AddCommand(listWorkflowsCmd)
}
