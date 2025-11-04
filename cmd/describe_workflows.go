package cmd

import (
	"context"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/cloudposse/atmos/internal/exec"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/schema"
)

var describeWorkflowsParser = flags.NewStandardOptionsBuilder().
	WithFormat([]string{"json", "yaml"}, "yaml").
	WithOutput([]string{"list", "map", "all"}, "list").
	WithQuery().
	Build()

// describeWorkflowsCmd executes 'atmos describe workflows' CLI commands.
var describeWorkflowsCmd = &cobra.Command{
	Use:   "workflows",
	Short: "List Atmos workflows and their associated files",
	Long:  "List all Atmos workflows, showing their associated files and workflow names for easy reference.",
	Args:  cobra.NoArgs,
	RunE:  getRunnableDescribeWorkflowsCmd(checkAtmosConfig, exec.ProcessCommandLineArgs, cfg.InitCliConfig, exec.NewDescribeWorkflowsExec()),
}

func getRunnableDescribeWorkflowsCmd(
	checkAtmosConfig func(opts ...AtmosValidateOption),
	processCommandLineArgs func(componentType string, cmd *cobra.Command, args []string, additionalArgsAndFlags []string) (schema.ConfigAndStacksInfo, error),
	initCliConfig func(info schema.ConfigAndStacksInfo, validate bool) (schema.AtmosConfiguration, error),
	describeWorkflowsExec exec.DescribeWorkflowsExec,
) func(cmd *cobra.Command, args []string) error {
	return func(cmd *cobra.Command, args []string) error {
		// Check Atmos configuration.
		checkAtmosConfig()

		// Parse flags using StandardOptions.
		opts, err := describeWorkflowsParser.Parse(context.Background(), args)
		if err != nil {
			return err
		}

		info, err := processCommandLineArgs("terraform", cmd, args, nil)
		if err != nil {
			return err
		}

		atmosConfig, err := initCliConfig(info, true)
		if err != nil {
			return err
		}

		// Build DescribeWorkflowsArgs from parsed options.
		// Format and output validation is now handled by the parser at parse time.
		describeWorkflowArgs := &exec.DescribeWorkflowsArgs{
			Format:     opts.Format,
			OutputType: opts.Output,
			Query:      opts.Query,
		}

		// Global --pager flag is now handled in cfg.InitCliConfig.
		err = describeWorkflowsExec.Execute(&atmosConfig, describeWorkflowArgs)
		return err
	}
}

func init() {
	// Register StandardOptions flags.
	describeWorkflowsParser.RegisterFlags(describeWorkflowsCmd)
	_ = describeWorkflowsParser.BindToViper(viper.GetViper())

	describeCmd.AddCommand(describeWorkflowsCmd)
}
