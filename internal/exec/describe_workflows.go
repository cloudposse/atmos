package exec

import (
	"fmt"

	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/spf13/cobra"
)

// ExecuteDescribeWorkflowsCmd executes `atmos describe workflows` CLI command
func ExecuteDescribeWorkflowsCmd(cmd *cobra.Command, args []string) error {
	info, err := ProcessCommandLineArgs("terraform", cmd, args, nil)
	if err != nil {
		return err
	}

	cliConfig, err := cfg.InitCliConfig(info, true)
	if err != nil {
		return err
	}

	flags := cmd.Flags()

	format, err := flags.GetString("format")
	if err != nil {
		return err
	}

	if format != "" && format != "yaml" && format != "json" {
		return fmt.Errorf("invalid '--format' flag '%s'. Valid values are 'yaml' (default) and 'json'", format)
	}

	if format == "" {
		format = "yaml"
	}

	outputType, err := flags.GetString("output")
	if err != nil {
		return err
	}

	if outputType != "" && outputType != "list" && outputType != "map" && outputType != "all" {
		return fmt.Errorf("invalid '--output' flag '%s'. Valid values are 'list' (default), 'map' and 'all'", outputType)
	}

	if outputType == "" {
		outputType = "list"
	}

	describeWorkflowsList, describeWorkflowsMap, describeWorkflowsAll, err := ExecuteDescribeWorkflows(cliConfig)
	if err != nil {
		return err
	}

	if outputType == "list" {
		err = printOrWriteToFile(format, "", describeWorkflowsList)
	} else if outputType == "map" {
		err = printOrWriteToFile(format, "", describeWorkflowsMap)
	} else {
		err = printOrWriteToFile(format, "", describeWorkflowsAll)
	}
	if err != nil {
		return err
	}

	return nil
}
