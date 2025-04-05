package cmd

import (
	"github.com/cloudposse/atmos/pkg/config"
	"github.com/spf13/cobra"
)

// vendorCmd executes 'atmos vendor' CLI commands
var vendorCmd = &cobra.Command{
	Use:                "vendor",
	Short:              "Manage external dependencies for components or stacks",
	Long:               `This command manages external dependencies for Atmos components or stacks by vendoring them. Vendoring involves copying and locking required dependencies locally, ensuring consistency, reliability, and alignment with the principles of immutable infrastructure.`,
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: false},
	Args:               cobra.NoArgs,
}

func init() {
	config.DefaultConfigHandler.AddConfig(vendorCmd, &config.ConfigOptions{
		FlagName:     "vendor-base-path",
		EnvVar:       "ATMOS_VENDOR_BASE_PATH",
		Description:  "Base path for vendored dependencies.",
		Key:          "vendor.base_path",
		DefaultValue: "",
	})
	RootCmd.AddCommand(vendorCmd)
}
