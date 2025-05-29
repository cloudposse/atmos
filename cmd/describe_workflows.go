package cmd

import (
	"fmt"

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
		checkErrorAndExit(err)
		atmosConfig, err := initCliConfig(info, true)
		checkErrorAndExit(err)
		describeWorkflowArgs := &exec.DescribeWorkflowsArgs{}
		err = flagsToDescribeWorkflowsArgs(cmd.Flags(), describeWorkflowArgs)
		checkErrorAndExit(err)
		err = describeWorkflowsExec.Execute(&atmosConfig, describeWorkflowArgs)
		checkErrorAndExit(err)
	}
}

func init() {
	describeWorkflowsCmd.PersistentFlags().StringP("format", "f", "yaml", "Specify the output format (`yaml` is default)")
	describeWorkflowsCmd.PersistentFlags().StringP("output", "o", "list", "Specify the output type (`list` is default)")

	describeCmd.AddCommand(describeWorkflowsCmd)
}

func flagsToDescribeWorkflowsArgs(flags *pflag.FlagSet, describe *exec.DescribeWorkflowsArgs) error {
	var err error
	flagsKeyValue := map[string]any{
		"format": &describe.Format,
		"output": &describe.OutputType,
		"query":  &describe.Query,
	}

	for k := range flagsKeyValue {
		if !flags.Changed(k) {
			continue
		}
		switch v := flagsKeyValue[k].(type) {
		case *string:
			*v, err = flags.GetString(k)
		default:
			panic(fmt.Sprintf("unsupported type %T for flag %s", v, k))
		}
		checkFlagError(err)
	}
	format := describe.Format
	outputType := describe.OutputType

	if format != "" && format != "yaml" && format != "json" {
		return fmt.Errorf("invalid '--format' flag '%s'. Valid values are 'yaml' (default) and 'json'", format)
	}

	if format == "" {
		format = "yaml"
	}

	if outputType != "" && outputType != "list" && outputType != "map" && outputType != "all" {
		return fmt.Errorf("invalid '--output' flag '%s'. Valid values are 'list' (default), 'map' and 'all'", outputType)
	}

	if outputType == "" {
		outputType = "list"
	}
	describe.Format = format
	describe.OutputType = outputType
	return nil
}
