package stack

import (
	"github.com/spf13/cobra"

	"github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/perf"
)

var stackValidateCmd = &cobra.Command{
	Use:   "validate",
	Short: "Validate stack manifest configurations",
	Long: `Validate the configuration of all stack manifests against the atmos-manifest
JSON Schema — the same one ` + "`atmos stack schema`" + ` prints. This is an alias for
` + "`atmos validate stacks`" + `.`,
	Example: "atmos stack validate",
	Args:    cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		defer perf.Track(atmosConfigPtr, "stack.validateRunE")()
		return exec.ExecuteValidateStacksCmd(cmd, args)
	},
}

func init() {
	stackValidateCmd.PersistentFlags().String("schemas-atmos-manifest", "", "Specifies the path to a JSON schema file used to validate the structure and content of the Atmos manifest file")
}
