package cmd

import (
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	e "github.com/cloudposse/atmos/internal/exec"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/schema"
)

var describeConfigParser = flags.NewStandardOptionsBuilder().
	WithFormat([]string{"json", "yaml"}, "json").
	WithQuery().
	Build()

// describeComponentCmd describes configuration for components
var describeConfigCmd = &cobra.Command{
	Use:   "config",
	Short: "Display the final merged CLI configuration",
	Long:  "This command displays the final, deep-merged CLI configuration after combining all relevant configuration files.",
	RunE: func(cmd *cobra.Command, args []string) error {
		opts, err := describeConfigParser.Parse(cmd.Context(), args)
		if err != nil {
			return err
		}

		atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, false)
		if err != nil {
			return err
		}

		// Global --pager flag is now handled in cfg.InitCliConfig

		err = e.NewDescribeConfig(&atmosConfig).ExecuteDescribeConfigCmd(opts.Query, opts.Format, "")
		return err
	},
}

func init() {
	describeConfigParser.RegisterFlags(describeConfigCmd)
	_ = describeConfigParser.BindToViper(viper.GetViper())

	describeCmd.AddCommand(describeConfigCmd)
}
