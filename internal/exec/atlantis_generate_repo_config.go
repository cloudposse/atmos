package exec

import (
	"github.com/spf13/cobra"
)

// ExecuteAtlantisGenerateRepoConfigCmd executes `atlantis generate repo-config` command
func ExecuteAtlantisGenerateRepoConfigCmd(cmd *cobra.Command, args []string) error {
	return ExecuteAtlantisGenerateRepoConfig()
}

// ExecuteAtlantisGenerateRepoConfig generates repository configuration for Atlantis
func ExecuteAtlantisGenerateRepoConfig() error {
	return nil
}
