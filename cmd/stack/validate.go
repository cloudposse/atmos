package stack

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/validation"
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
		format, _ := cmd.Flags().GetString("format")
		format = strings.ToLower(strings.TrimSpace(format))
		if format != "" && format != "text" && format != "rich" {
			return fmt.Errorf("unsupported validation format %q: expected text or rich", format)
		}
		if format == "rich" {
			err := exec.ValidateStacks(atmosConfigPtr)
			if err == nil {
				message := "✓ All stacks validated successfully"
				if len(atmosConfigPtr.StackConfigFilesAbsolutePaths) == 0 {
					message = "✓ No stack manifests found to validate"
				}
				_, writeErr := fmt.Fprintln(cmd.OutOrStdout(), message)
				return writeErr
			}
			root := atmosConfigPtr.StacksBaseAbsolutePath
			if root == "" {
				var rootErr error
				root, rootErr = os.Getwd()
				if rootErr != nil {
					return rootErr
				}
			}
			if _, writeErr := fmt.Fprintln(cmd.OutOrStdout(), validation.Rich(validation.FromGCCText("stacks", err.Error()), validation.DefaultRichOptions(root))); writeErr != nil {
				return writeErr
			}
			return errUtils.ExitCodeError{Code: 1, Silent: true}
		}
		return exec.ExecuteValidateStacksCmd(cmd, args)
	},
}

func init() {
	stackValidateCmd.PersistentFlags().String("schemas-atmos-manifest", "", "Specifies the path to a JSON schema file used to validate the structure and content of the Atmos manifest file")
	stackValidateCmd.PersistentFlags().String("format", "", "Output format: text, rich")
}
