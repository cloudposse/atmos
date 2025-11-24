package backend

import (
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/cloudposse/atmos/pkg/flags"
)

var createParser *flags.StandardParser

var createCmd = &cobra.Command{
	Use:     "<component>",
	Short:   "Provision backend infrastructure",
	Long:    `Create or update S3 backend with secure defaults (versioning, encryption, public access blocking). This operation is idempotent.`,
	Example: `  atmos terraform provision backend vpc --stack dev`,
	Args:    cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return ExecuteProvisionCommand(cmd, args, createParser, "backend.create.RunE")
	},
}

func init() {
	createCmd.DisableFlagParsing = false

	createParser = flags.NewStandardParser(
		flags.WithStackFlag(),
		flags.WithIdentityFlag(),
	)

	createParser.RegisterFlags(createCmd)

	if err := createParser.BindToViper(viper.GetViper()); err != nil {
		panic(err)
	}
}
