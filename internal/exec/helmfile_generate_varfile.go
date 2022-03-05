package exec

import (
	"github.com/spf13/cobra"
)

// ExecuteHelmfileGenerateVarfile executes `helmfile generate varfile` command
func ExecuteHelmfileGenerateVarfile(cmd *cobra.Command, args []string) error {
	return generateVarfile(cmd, args, "helmfile")
}
