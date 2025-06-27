package cmd

import (
	log "github.com/charmbracelet/log"
	"github.com/spf13/cobra"

	atmoserr "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/internal/exec"
)

// ValidateStacksCmd validates stacks
var ValidateStacksCmd = &cobra.Command{
	Use:                "stacks",
	Short:              "Validate stack manifest configurations",
	Long:               "This command validates the configuration of stack manifests in Atmos to ensure proper setup and compliance.",
	Example:            "validate stacks",
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: false},
	Args:               cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		// Check Atmos configuration
		checkAtmosConfig()

		err := exec.ExecuteValidateStacksCmd(cmd, args)
		atmoserr.CheckErrorPrintMarkdownAndExit(err, "", "")

		log.Info("All stacks validated successfully")
	},
}

func init() {
	ValidateStacksCmd.DisableFlagParsing = false

	ValidateStacksCmd.PersistentFlags().String("schemas-atmos-manifest", "", "Specifies the path to a JSON schema file used to validate the structure and content of the Atmos manifest file")

	validateCmd.AddCommand(ValidateStacksCmd)
}
