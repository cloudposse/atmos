package exec

import (
	"github.com/spf13/cobra"

	evalUtils "github.com/cloudposse/atmos/internal/exec/utils"
	cfg "github.com/cloudposse/atmos/pkg/config"
)

// ExecuteDescribeConfigCmd executes `describe config` command
func ExecuteDescribeConfigCmd(cmd *cobra.Command, args []string) error {
	flags := cmd.Flags()

	format, err := flags.GetString("format")
	if err != nil {
		return err
	}

	jmespath, err := flags.GetString("jmespath")
	if err != nil {
		return err
	}

	jsonpath, err := flags.GetString("jsonpath")
	if err != nil {
		return err
	}

	info, err := processCommandLineArgs("", cmd, args, nil)
	if err != nil {
		return err
	}

	cliConfig, err := cfg.InitCliConfig(info, false)
	if err != nil {
		return err
	}

	var finalResult any
	if jmespath != "" {
		finalResult, err = evalUtils.EvaluateJmesPath(jmespath, cliConfig)
	} else if jsonpath != "" {
		finalResult, err = evalUtils.EvaluateJsonPath(jmespath, cliConfig)
	} else {
		finalResult = cliConfig
	}

	err = printOrWriteToFile(format, "", finalResult)
	if err != nil {
		return err
	}

	return nil
}
