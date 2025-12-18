package planfile

import (
	"github.com/spf13/cobra"
)

// PlanfileCmd represents the base command for planfile operations.
var PlanfileCmd = &cobra.Command{
	Use:   "planfile",
	Short: "Manage Terraform plan files",
	Long:  `Commands for managing Terraform plan files, including upload, download, list, delete, and show.`,
}

func init() {
	PlanfileCmd.AddCommand(uploadCmd)
	PlanfileCmd.AddCommand(downloadCmd)
	PlanfileCmd.AddCommand(listCmd)
	PlanfileCmd.AddCommand(deleteCmd)
	PlanfileCmd.AddCommand(showCmd)
}
