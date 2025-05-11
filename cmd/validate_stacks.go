package cmd

import (
	"github.com/spf13/cobra"

	e "github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/ui/theme"
	u "github.com/cloudposse/atmos/pkg/utils"
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

		err := e.ExecuteValidateStacksCmd(cmd, args)
		if err != nil {
			u.PrintErrorMarkdownAndExit("", err, "")
		}

		u.PrintMessageInColor("all stacks validated successfully\n", theme.Colors.Success)
	},
}

func init() {
	ValidateStacksCmd.DisableFlagParsing = false

	config.DefaultConfigHandler.AddConfig(ValidateStacksCmd, &config.ConfigOptions{
		FlagName:     "schemas-atmos-manifest",
		EnvVar:       "ATMOS_SCHEMAS_ATMOS_MANIFEST",
		Description:  "Specifies the path to a JSON schema file used to validate the structure and content of the Atmos manifest file",
		Key:          "schemas.atmos.manifest",
		DefaultValue: "",
	})
	validateCmd.AddCommand(ValidateStacksCmd)
}
