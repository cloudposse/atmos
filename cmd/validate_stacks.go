package cmd

import (
	"context"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/flags"
	log "github.com/cloudposse/atmos/pkg/logger"
)

var validateStacksParser = flags.NewStandardOptionsBuilder().
	WithSchemasAtmosManifest("").
	Build()

// ValidateStacksCmd validates stacks
var ValidateStacksCmd = &cobra.Command{
	Use:     "stacks",
	Short:   "Validate stack manifest configurations",
	Long:    "This command validates the configuration of stack manifests in Atmos to ensure proper setup and compliance.",
	Example: "validate stacks",
	Args:    cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Check Atmos configuration
		checkAtmosConfig()

		opts, err := validateStacksParser.Parse(context.Background(), args)
		if err != nil {
			return err
		}

		err = exec.ExecuteValidateStacksCmd(opts)
		if err != nil {
			return err
		}

		log.Info("All stacks validated successfully")
		return nil
	},
}

func init() {

	validateStacksParser.RegisterFlags(ValidateStacksCmd)
	_ = validateStacksParser.BindToViper(viper.GetViper())

	validateCmd.AddCommand(ValidateStacksCmd)
}
