package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/cloudposse/atmos/internal/exec"
	e "github.com/cloudposse/atmos/internal/exec"
	cfg "github.com/cloudposse/atmos/pkg/config"
)

// describeWorkflowsCmd executes 'atmos describe workflows' CLI commands
var describeWorkflowsCmd = &cobra.Command{
	Use:                "workflows",
	Short:              "List Atmos workflows and their associated files",
	Long:               "List all Atmos workflows, showing their associated files and workflow names for easy reference.",
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: false},
	Args:               cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		checkAtmosConfig()
		info, err := exec.ProcessCommandLineArgs("terraform", cmd, args, nil)
		checkErrorAndExit(err)
		atmosConfig, err := cfg.InitCliConfig(info, true)
		checkErrorAndExit(err)
		describeWorkflowArgs := &e.DescribeWorkflowsArgs{}
		err = flagsToDescribeWorkflowsArgs(cmd.Flags(), describeWorkflowArgs)
		checkErrorAndExit(err)
		err = exec.NewDescribeWorkflowsExec().Execute(&atmosConfig, describeWorkflowArgs)
		checkErrorAndExit(err)
	},
}

func init() {
	describeWorkflowsCmd.PersistentFlags().StringP("format", "f", "yaml", "Specify the output format (`yaml` is default)")
	describeWorkflowsCmd.PersistentFlags().StringP("output", "o", "list", "Specify the output type (`list` is default)")

	describeCmd.AddCommand(describeWorkflowsCmd)
}

func flagsToDescribeWorkflowsArgs(flags *pflag.FlagSet, describe *e.DescribeWorkflowsArgs) error {
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
		case *bool:
			*v, err = flags.GetBool(k)
		case *[]string:
			*v, err = flags.GetStringSlice(k)
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
