package cmd

import (
	"github.com/spf13/cobra"

	e "github.com/cloudposse/atmos/internal/exec"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
)

// describeComponentCmd describes configuration for components
var describeConfigCmd = &cobra.Command{
	Use:                "config",
	Short:              "Display the final merged CLI configuration",
	Long:               "This command displays the final, deep-merged CLI configuration after combining all relevant configuration files.",
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: false},
	Args:               cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		flags := cmd.Flags()

		format, err := flags.GetString("format")
		if err != nil {
			telemetry.CaptureCmd(cmd, err)
			return err
		}

		query, err := flags.GetString("query")
		if err != nil {
			telemetry.CaptureCmd(cmd, err)
			return err
		}

		atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, false)
		if err != nil {
			telemetry.CaptureCmd(cmd, err)
			return err
		}

		if cmd.Flags().Changed("pager") {
			// TODO: update this post pr:https://github.com/cloudposse/atmos/pull/1174 is merged
			atmosConfig.Settings.Terminal.Pager, err = cmd.Flags().GetString("pager")
			if err != nil {
				telemetry.CaptureCmd(cmd, err)
				return err
			}
		}

		err = e.NewDescribeConfig(&atmosConfig).ExecuteDescribeConfigCmd(query, format, "")
		return err
	},
}

func init() {
	describeConfigCmd.DisableFlagParsing = false
	describeConfigCmd.PersistentFlags().StringP("format", "f", "json", "The output format")

	describeCmd.AddCommand(describeConfigCmd)
}
