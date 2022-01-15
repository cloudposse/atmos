package exec

import (
	"fmt"
	"github.com/spf13/cobra"
)

// ExecuteTerraformShell configures an environment for a component in a stack and starts a new shell allowing executing plain terraform commands
func ExecuteTerraformShell(cmd *cobra.Command, args []string) error {
	fmt.Println(fmt.Sprintf("'atmos terraform shell'"))

	fmt.Println()
	return nil
}
