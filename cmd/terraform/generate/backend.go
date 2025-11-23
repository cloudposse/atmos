package generate

import (
	"github.com/spf13/cobra"

	e "github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/ui"
)

// backendCmd generates backend config for a terraform component.
var backendCmd = &cobra.Command{
	Use:                "backend",
	Short:              "Generate backend configuration for a Terraform component",
	Long:               `This command generates the backend configuration for a Terraform component using the specified stack`,
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: false},
	RunE: func(cmd *cobra.Command, args []string) error {
		err := e.ExecuteTerraformGenerateBackendCmd(cmd, args)
		return err
	},
}

func init() {
	backendCmd.DisableFlagParsing = false

	// Add stack flag (required).
	backendCmd.PersistentFlags().StringP("stack", "s", "", "Atmos stack (required)")
	if err := backendCmd.MarkPersistentFlagRequired("stack"); err != nil {
		ui.Error(err.Error())
	}

	GenerateCmd.AddCommand(backendCmd)
}
