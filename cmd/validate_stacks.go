package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/validation"
)

// ValidateStacksCmd validates stacks
var ValidateStacksCmd = &cobra.Command{
	Use:                "stacks",
	Short:              "Validate stack manifest configurations",
	Long:               "This command validates the configuration of stack manifests in Atmos to ensure proper setup and compliance.",
	Example:            "validate stacks",
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: false},
	Args:               cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runValidateStacks(cmd, args)
	},
}

// runValidateStacks executes stack validation without terminating the process.
// It can therefore be composed by aggregate validators.
func runValidateStacks(cmd *cobra.Command, args []string) error {
	affectedFiles, affected, err := validationAffectedFiles(cmd)
	if err != nil {
		return err
	}
	return runValidateStacksForFiles(cmd, args, affectedFiles, affected)
}

func runValidateStacksForFiles(cmd *cobra.Command, args []string, affectedFiles []string, affected bool) error {
	// A missing stacks directory is a valid no-op for this validator. The
	// executor below handles it explicitly, so do not reject the project while
	// loading the CLI configuration.
	if err := checkAtmosConfigE(WithStackValidation(false)); err != nil {
		return err
	}
	if affected && !affectedStacksApplicable(affectedFiles) {
		return validationNoAffectedFiles(cmd, "stack manifest")
	}
	format, err := validationFormat(cmd)
	if err != nil {
		return err
	}
	if format == validateFormatRich {
		err := exec.ValidateStacks(&atmosConfig)
		if err == nil {
			message := "✓ All stacks validated successfully"
			if len(atmosConfig.StackConfigFilesAbsolutePaths) == 0 {
				message = "✓ No stack manifests found to validate"
			}
			_, writeErr := fmt.Fprintln(cmd.OutOrStdout(), message)
			return writeErr
		}
		root := atmosConfig.StacksBaseAbsolutePath
		if root == "" {
			var rootErr error
			root, rootErr = os.Getwd()
			if rootErr != nil {
				return rootErr
			}
		}
		output := validation.Rich(validation.FromGCCText("stacks", err.Error()), validation.DefaultRichOptions(root))
		if output != "" {
			if _, writeErr := fmt.Fprintln(cmd.OutOrStdout(), output); writeErr != nil {
				return writeErr
			}
		}
		return errUtils.ExitCodeError{Code: 1, Silent: true}
	}

	return exec.ExecuteValidateStacksCmd(cmd, args)
}

func init() {
	ValidateStacksCmd.DisableFlagParsing = false

	ValidateStacksCmd.PersistentFlags().String("schemas-atmos-manifest", "", "Specifies the path to a JSON schema file used to validate the structure and content of the Atmos manifest file")
	addValidationFormatFlag(ValidateStacksCmd)
	addAffectedValidationFlags(ValidateStacksCmd)

	validateCmd.AddCommand(ValidateStacksCmd)
}
