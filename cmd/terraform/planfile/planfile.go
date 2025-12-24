package planfile

import (
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/flags/global"
)

// BaseOptions contains flags common to all planfile commands.
type BaseOptions struct {
	global.Flags
	Store string
}

// parseBaseOptions parses the common flags into BaseOptions.
func parseBaseOptions(cmd *cobra.Command, v *viper.Viper) BaseOptions {
	return BaseOptions{
		Flags: flags.ParseGlobalFlags(cmd, v),
		Store: v.GetString("store"),
	}
}

// PlanfileCmd represents the base command for planfile operations.
var PlanfileCmd = &cobra.Command{
	Use:   "planfile",
	Short: "Manage Terraform plan files",
	Long:  `Commands for managing Terraform plan files, including upload, download, list, delete, and show.`,
}
