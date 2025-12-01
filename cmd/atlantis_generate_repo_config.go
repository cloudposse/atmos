package cmd

import (
	"github.com/spf13/cobra"

	e "github.com/cloudposse/atmos/internal/exec"
)

// atlantisGenerateRepoConfigCmd generates repository configuration for Atlantis.
var atlantisGenerateRepoConfigCmd = &cobra.Command{
	Use:                "repo-config",
	Short:              "Generate repository configuration for Atlantis",
	Long:               "Generate the repository configuration file required for Atlantis to manage Terraform repositories.",
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: false},
	RunE: func(cmd *cobra.Command, args []string) error {
		handleHelpRequest(cmd, args)
		if len(args) > 0 {
			showUsageAndExit(cmd, args)
		}
		// Check Atmos configuration
		checkAtmosConfig()
		err := e.ExecuteAtlantisGenerateRepoConfigCmd(cmd, args)
		return err
	},
}

func init() {
	atlantisGenerateRepoConfigCmd.DisableFlagParsing = false

	atlantisGenerateRepoConfigCmd.PersistentFlags().String("output-path", "", "Output path to write `atlantis.yaml` file")
	atlantisGenerateRepoConfigCmd.PersistentFlags().String("config-template", "", "Atlantis config template name")
	atlantisGenerateRepoConfigCmd.PersistentFlags().String("project-template", "", "Atlantis project template name")

	atlantisGenerateRepoConfigCmd.PersistentFlags().String("stacks", "", "Generate Atlantis projects for the specified stacks only (comma-separated values).")

	atlantisGenerateRepoConfigCmd.PersistentFlags().String("components", "",
		"Generate Atlantis projects for the specified components only (comma-separated values).",
	)

	atlantisGenerateRepoConfigCmd.PersistentFlags().Bool("affected-only", false,
		"Generate Atlantis projects only for the Atmos components changed between two Git commits.",
	)

	atlantisGenerateRepoConfigCmd.PersistentFlags().String("repo-path", "", "Filesystem path to the already cloned target repository with which to compare the current branch")
	atlantisGenerateRepoConfigCmd.PersistentFlags().String("ref", "", "Git reference with which to compare the current branch. Refer to [10.3 Git Internals Git References](https://git-scm.com/book/en/v2/Git-Internals-Git-References) for more details")
	atlantisGenerateRepoConfigCmd.PersistentFlags().String("sha", "", "Git commit SHA with which to compare the current branch")
	atlantisGenerateRepoConfigCmd.PersistentFlags().Bool("verbose", false, "Print more detailed output when cloning and checking out the Git repository")
	atlantisGenerateRepoConfigCmd.PersistentFlags().String("ssh-key", "", "Path to PEM-encoded private key to clone private repos using SSH")
	atlantisGenerateRepoConfigCmd.PersistentFlags().String("ssh-key-password", "", "Encryption password for the PEM-encoded private key if the key contains a password-encrypted PEM block")
	atlantisGenerateRepoConfigCmd.PersistentFlags().Bool("clone-target-ref", false, "Clone the target reference for comparison with the current branch. Only used when `--affected-only=true`. Defaults to false, which checks out the target reference instead.")

	atlantisGenerateCmd.AddCommand(atlantisGenerateRepoConfigCmd)
}
