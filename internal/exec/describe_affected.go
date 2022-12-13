package exec

import (
	"fmt"
	"github.com/spf13/cobra"

	cfg "github.com/cloudposse/atmos/pkg/config"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// ExecuteDescribeAffectedCmd executes `describe affected` command
func ExecuteDescribeAffectedCmd(cmd *cobra.Command, args []string) error {
	info, err := processCommandLineArgs("", cmd, args)
	if err != nil {
		return err
	}

	cliConfig, err := cfg.InitCliConfig(info, true)
	if err != nil {
		u.PrintErrorToStdError(err)
		return err
	}

	flags := cmd.Flags()

	base, err := flags.GetString("base")
	if err != nil {
		return err
	}

	format, err := flags.GetString("format")
	if err != nil {
		return err
	}
	if format != "" && format != "yaml" && format != "json" {
		return fmt.Errorf("invalid '--format' flag '%s'. Valid values are 'yaml' (default) and 'json'", format)
	}
	if format == "" {
		format = "json"
	}

	file, err := flags.GetString("file")
	if err != nil {
		return err
	}

	finalStacksMap, err := ExecuteDescribeAffected(cliConfig, base)
	if err != nil {
		return err
	}

	if format == "yaml" {
		if file == "" {
			err = u.PrintAsYAML(finalStacksMap)
			if err != nil {
				return err
			}
		} else {
			err = u.WriteToFileAsYAML(file, finalStacksMap, 0644)
			if err != nil {
				return err
			}
		}
	} else if format == "json" {
		if file == "" {
			err = u.PrintAsJSON(finalStacksMap)
			if err != nil {
				return err
			}
		} else {
			err = u.WriteToFileAsJSON(file, finalStacksMap, 0644)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

// ExecuteDescribeAffected processes stack configs and returns a list of the affected Atmos components and stacks given two Git commits
func ExecuteDescribeAffected(
	cliConfig cfg.CliConfiguration,
	base string,
) (map[string]any, error) {

	finalStacksMap, err := ExecuteDescribeStacks(cliConfig, "", nil, nil, nil)
	if err != nil {
		return nil, err
	}

	return finalStacksMap, nil
}
