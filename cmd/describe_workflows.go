package cmd

import (
	"fmt"
	atmoserr "github.com/cloudposse/atmos/errors"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/cloudposse/atmos/internal/exec"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
)

// describeWorkflowsCmd executes 'atmos describe workflows' CLI commands
var describeWorkflowsCmd = &cobra.Command{
	Use:                "workflows",
	Short:              "List Atmos workflows and their associated files",
	Long:               "List all Atmos workflows, showing their associated files and workflow names for easy reference.",
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: false},
	Args:               cobra.NoArgs,
	Run:                getRunnableDescribeWorkflowsCmd(checkAtmosConfig, exec.ProcessCommandLineArgs, cfg.InitCliConfig, exec.NewDescribeWorkflowsExec()),
}

func getRunnableDescribeWorkflowsCmd(
	checkAtmosConfig func(opts ...AtmosValidateOption),
	processCommandLineArgs func(componentType string, cmd *cobra.Command, args []string, additionalArgsAndFlags []string) (schema.ConfigAndStacksInfo, error),
	initCliConfig func(info schema.ConfigAndStacksInfo, validate bool) (schema.AtmosConfiguration, error),
	describeWorkflowsExec exec.DescribeWorkflowsExec,
) func(cmd *cobra.Command, args []string) {
	return func(cmd *cobra.Command, args []string) {
		// Check Atmos configuration
		checkAtmosConfig()
		info, err := processCommandLineArgs("terraform", cmd, args, nil)
		atmoserr.PrintErrorMarkdownAndExit(err, "", "")
		atmosConfig, err := initCliConfig(info, true)
		atmoserr.PrintErrorMarkdownAndExit(err, "", "")
		describeWorkflowArgs := &exec.DescribeWorkflowsArgs{}
		err = flagsToDescribeWorkflowsArgs(cmd.Flags(), describeWorkflowArgs)
		atmoserr.PrintErrorMarkdownAndExit(err, "", "")
		pager, err := cmd.Flags().GetString("pager")
		atmoserr.PrintErrorMarkdownAndExit(err, "", "")
		atmosConfig.Settings.Terminal.Pager = pager
		err = describeWorkflowsExec.Execute(&atmosConfig, describeWorkflowArgs)
		atmoserr.PrintErrorMarkdownAndExit(err, "", "")
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

func setStringFlagIfChanged(flags *pflag.FlagSet, name string, target *string) error {
	if flags.Changed(name) {
		val, err := flags.GetString(name)
		if err != nil {
			return err
		}
		*target = val
	}
	return nil
}

var (
	ErrInvalidOutputType = fmt.Errorf("invalid output type specified. Valid values are 'list', 'map', and 'all'")
	ErrInvalidFormat     = fmt.Errorf("invalid format specified. Valid values are 'yaml' and 'json'")
)

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
