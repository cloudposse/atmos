package backend

import (
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/cloudposse/atmos/pkg/flags"
)

var updateParser *flags.StandardParser

var updateCmd = &cobra.Command{
	Use:   "update <component>",
	Short: "Update backend configuration",
	Long: `Apply configuration changes to existing backend.

This operation is idempotent and will update backend settings like
versioning, encryption, and public access blocking to match secure defaults.`,
	Example: `  atmos terraform provision backend update vpc --stack dev`,
	Args:    cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return ExecuteProvisionCommand(cmd, args, updateParser, "backend.update.RunE")
	},
}

func init() {
	updateCmd.DisableFlagParsing = false

	updateParser = flags.NewStandardParser(
		flags.WithStackFlag(),
		flags.WithIdentityFlag(),
	)

	updateParser.RegisterFlags(updateCmd)

	if err := updateParser.BindToViper(viper.GetViper()); err != nil {
		panic(err)
	}
}
