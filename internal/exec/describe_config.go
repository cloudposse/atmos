package exec

import (
	u "github.com/cloudposse/atmos/pkg/utils"
	"github.com/spf13/cobra"

	cfg "github.com/cloudposse/atmos/pkg/config"
)

// ExecuteDescribeConfigCmd executes `describe config` command
func ExecuteDescribeConfigCmd(cmd *cobra.Command, args []string) error {
	flags := cmd.Flags()

	format, err := flags.GetString("format")
	if err != nil {
		return err
	}

	query, err := flags.GetString("query")
	if err != nil {
		return err
	}

	info, err := ProcessCommandLineArgs("", cmd, args, nil)
	if err != nil {
		return err
	}

	atmosConfig, err := cfg.InitCliConfig(info, false)
	if err != nil {
		return err
	}

	var res any

	if query != "" {
		res, err = u.EvaluateYqExpression(&atmosConfig, atmosConfig, query)
		if err != nil {
			return err
		}
	} else {
		res = atmosConfig
	}

	err = printOrWriteToFile(atmosConfig, format, "", res)
	if err != nil {
		return err
	}

	return nil
}
