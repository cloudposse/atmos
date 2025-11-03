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
	Args:               cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		flags := cmd.Flags()

		format, err := flags.GetString("format")
		if err != nil {
			return err
		}

		query, err := flags.GetString("query")
		if err != nil {
			return err
		}

		atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, false)
		if err != nil {
			return err
		}

		// Global --pager flag is now handled in cfg.InitCliConfig

		err = e.NewDescribeConfig(&atmosConfig).ExecuteDescribeConfigCmd(query, format, "")
		return err
	},
}

func init() {
	describeConfigCmd.DisableFlagParsing = false
	describeConfigCmd.PersistentFlags().StringP("format", "f", "json", "The output format")

	describeCmd.AddCommand(describeConfigCmd)
}
