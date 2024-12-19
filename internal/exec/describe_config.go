package exec

import (
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

	info, err := ProcessCommandLineArgs("", cmd, args, nil)
	if err != nil {
		return err
	}

	atmosConfig, err := cfg.InitCliConfig(info, false)
	if err != nil {
		return err
	}

	err = printOrWriteToFile(format, "", atmosConfig)
	if err != nil {
		return err
	}

	return nil
}
