package exec

import (
	"fmt"

	u "github.com/cloudposse/atmos/pkg/utils"

	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/spf13/cobra"
)

// ExecuteDescribeWorkflowsCmd executes `atmos describe workflows` CLI command
func ExecuteDescribeWorkflowsCmd(cmd *cobra.Command, args []string) error {
	info, err := ProcessCommandLineArgs("terraform", cmd, args, nil)
	if err != nil {
		return err
	}

	atmosConfig, err := cfg.InitCliConfig(info, true)
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

	query, err := flags.GetString("query")
	if err != nil {
		return err
	}

	describeWorkflowsList, describeWorkflowsMap, describeWorkflowsAll, err := ExecuteDescribeWorkflows(atmosConfig)
	if err != nil {
		return err
	}

	var res any

	if outputType == "list" {
		res = describeWorkflowsList
	} else if outputType == "map" {
		res = describeWorkflowsMap
	} else {
		res = describeWorkflowsAll
	}

	if query != "" {
		res, err = u.EvaluateYqExpression(&atmosConfig, res, query)
		if err != nil {
			return err
		}
	}

	err = printOrWriteToFile(atmosConfig, format, "", res)
	if err != nil {
		return err
	}

	return nil
}
