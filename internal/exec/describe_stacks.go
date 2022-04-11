package exec

import (
	"errors"
	"fmt"
	c "github.com/cloudposse/atmos/pkg/config"
	u "github.com/cloudposse/atmos/pkg/utils"
	"github.com/spf13/cobra"
)

// ExecuteDescribeStacks executes `describe stacks` command
func ExecuteDescribeStacks(cmd *cobra.Command, args []string) error {
	flags := cmd.Flags()

	format, err := flags.GetString("format")
	if err != nil {
		return err
	}
	if format != "" && format != "yaml" && format != "json" {
		return errors.New(fmt.Sprintf("Invalid '--format' flag '%s'. Valid values are 'yaml' (default) and 'json'", format))
	}
	if format == "" {
		format = "yaml"
	}

	file, err := flags.GetString("file")
	if err != nil {
		return err
	}

	var configAndStacksInfo c.ConfigAndStacksInfo
	stacksMap, err := FindStacksMap(configAndStacksInfo, false)
	if err != nil {
		return err
	}

	if format == "yaml" {
		if file == "" {
			err = u.PrintAsYAML(stacksMap)
			if err != nil {
				return err
			}
		} else {
			err = u.WriteToFileAsYAML(file, stacksMap, 0644)
			if err != nil {
				return err
			}
		}
	} else if format == "json" {
		if file == "" {
			err = u.PrintAsJSON(stacksMap)
			if err != nil {
				return err
			}
		} else {
			err = u.WriteToFileAsJSON(file, stacksMap, 0644)
			if err != nil {
				return err
			}
		}
	}

	return nil
}
