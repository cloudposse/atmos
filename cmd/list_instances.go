package cmd

import (
	"github.com/spf13/cobra"

	e "github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/list"
	fl "github.com/cloudposse/atmos/pkg/list/flags"
	"github.com/cloudposse/atmos/pkg/schema"
)

// listInstancesCmd lists atmos instances.
var listInstancesCmd = &cobra.Command{
	Use:                "instances",
	Short:              "List all Atmos instances",
	Long:               "This command lists all Atmos instances or is used to upload instances to the pro API.",
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: false},
	Args:               cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Check Atmos configuration
		checkAtmosConfig()
		err := ExecuteListInstancesCmd(cmd, args)
		if err != nil {
			return err
		}
		return nil
	},
}

func init() {
	// Add common list flags
	fl.AddCommonListFlags(listInstancesCmd)

	// Add instance-specific flags
	listInstancesCmd.Flags().Bool("upload", false, "Upload instances to pro API")

	// Add the command to the list command
	listCmd.AddCommand(listInstancesCmd)
}

func ExecuteListInstancesCmd(cmd *cobra.Command, args []string) error {
	info := &schema.ConfigAndStacksInfo{}
	info.Command = "list"
	info.SubCommand = "instances"

	// Process and validate command line arguments
	_, err := e.ProcessCommandLineArgs("list", cmd, args, nil)
	if err != nil {
		return err
	}

	return list.ExecuteListInstancesCmd(info, cmd, args)
}
