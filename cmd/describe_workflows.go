package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/cloudposse/atmos/internal/exec"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
)

var (
	ErrInvalidOutputType = fmt.Errorf("invalid output type specified. Valid values are 'list', 'map', and 'all'")
	ErrInvalidFormat     = fmt.Errorf("invalid format specified. Valid values are 'yaml' and 'json'")
)

// describeWorkflowsCmd executes 'atmos describe workflows' CLI commands.
var describeWorkflowsCmd = &cobra.Command{
	Use:                "workflows",
	Short:              "List Atmos workflows and their associated files",
	Long:               "List all Atmos workflows, showing their associated files and workflow names for easy reference.",
	Args:               cobra.NoArgs,
	RunE:               getRunnableDescribeWorkflowsCmd(checkAtmosConfig, exec.ProcessCommandLineArgs, cfg.InitCliConfig, exec.NewDescribeWorkflowsExec()),
}

func getRunnableDescribeWorkflowsCmd(
	checkAtmosConfig func(opts ...AtmosValidateOption),
	processCommandLineArgs func(componentType string, cmd *cobra.Command, args []string, additionalArgsAndFlags []string) (schema.ConfigAndStacksInfo, error),
	initCliConfig func(info schema.ConfigAndStacksInfo, validate bool) (schema.AtmosConfiguration, error),
	describeWorkflowsExec exec.DescribeWorkflowsExec,
) func(cmd *cobra.Command, args []string) error {
	return func(cmd *cobra.Command, args []string) error {
		// Check Atmos configuration
		checkAtmosConfig()

		info, err := processCommandLineArgs("terraform", cmd, args, nil)
		if err != nil {
			return err
		}

		atmosConfig, err := initCliConfig(info, true)
		if err != nil {
			return err
		}

		describeWorkflowArgs := &exec.DescribeWorkflowsArgs{}
		err = flagsToDescribeWorkflowsArgs(cmd.Flags(), describeWorkflowArgs)
		if err != nil {
			return err
		}

		// Global --pager flag is now handled in cfg.InitCliConfig
		err = describeWorkflowsExec.Execute(&atmosConfig, describeWorkflowArgs)
		return err
	}
}

func init() {
	describeWorkflowsCmd.PersistentFlags().StringP("format", "f", "yaml", "Specify the output format (`yaml` is default)")
	describeWorkflowsCmd.PersistentFlags().StringP("output", "o", "list", "Specify the output type (`list` is default)")

	describeCmd.AddCommand(describeWorkflowsCmd)
}

func flagsToDescribeWorkflowsArgs(flags *pflag.FlagSet, describe *exec.DescribeWorkflowsArgs) error {
	if err := setStringFlagIfChanged(flags, "format", &describe.Format); err != nil {
		return err
	}
	if err := setStringFlagIfChanged(flags, "output", &describe.OutputType); err != nil {
		return err
	}
	if err := setStringFlagIfChanged(flags, "query", &describe.Query); err != nil {
		return err
	}

	if err := validateAndSetDefaults(describe); err != nil {
		return err
	}

	return nil
}

func validateAndSetDefaults(describe *exec.DescribeWorkflowsArgs) error {
	if describe.Format == "" {
		describe.Format = "yaml"
	} else if describe.Format != "yaml" && describe.Format != "json" {
		return ErrInvalidFormat
	}

	if describe.OutputType == "" {
		describe.OutputType = "list"
	} else if describe.OutputType != "list" && describe.OutputType != "map" && describe.OutputType != "all" {
		return ErrInvalidOutputType
	}

	return nil
}
