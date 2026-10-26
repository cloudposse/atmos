package cmd

import (
	cicmd "github.com/cloudposse/atmos/cmd/ci"
)

// validateCICmd is a validation-oriented alias for `atmos ci validate`.
// It has its own Cobra command instance because one command cannot be mounted
// under two parents.
var validateCICmd = cicmd.NewValidateCommand()

func init() {
	validateCICmd.Use = "ci [workflow-file ...]"
	validateCICmd.Long += "\n\nThis command is an alias for `atmos ci validate`."
	validateCmd.AddCommand(validateCICmd)
}
