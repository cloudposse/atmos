package cmd

import (
	"github.com/spf13/cobra"
)

// authMigrateCmd is the parent command for auth migration subcommands.
var authMigrateCmd = &cobra.Command{
	Use:   "migrate",
	Short: "Migrate authentication configuration",
	Long:  "Migrate authentication configuration from legacy patterns to atmos-managed auth. Run a subcommand to perform a specific migration.",
}

func init() {
	authCmd.AddCommand(authMigrateCmd)
}
