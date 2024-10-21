package cmd

import (
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/spf13/cobra"

	e "github.com/cloudposse/atmos/internal/exec"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// atlantisGenerateRepoConfigCmd generates repository configuration for Atlantis
var atlantisGenerateRepoConfigCmd = &cobra.Command{
	Use:                "repo-config",
	Short:              "Execute 'atlantis generate repo-config`",
	Long:               "This command generates repository configuration for Atlantis",
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: false},
	Run: func(cmd *cobra.Command, args []string) {
		// Check Atmos configuration
		checkAtmosConfig()

		err := e.ExecuteAtlantisGenerateRepoConfigCmd(cmd, args)
		if err != nil {
			u.LogErrorAndExit(schema.CliConfiguration{}, err)
		}
	},
}

func init() {
	atlantisGenerateRepoConfigCmd.DisableFlagParsing = false

	atlantisGenerateRepoConfigCmd.PersistentFlags().String("output-path", "", "atmos atlantis generate repo-config --output-path ./atlantis.yaml --config-template config-1 --project-template project-1")
	atlantisGenerateRepoConfigCmd.PersistentFlags().String("config-template", "", "atmos atlantis generate repo-config --config-template config-1 --project-template project-1")
	atlantisGenerateRepoConfigCmd.PersistentFlags().String("project-template", "", "atmos atlantis generate repo-config --config-template config-1 --project-template project-1")

	atlantisGenerateRepoConfigCmd.PersistentFlags().String("stacks", "",
		"Generate Atlantis projects for the specified stacks only (comma-separated values).\n"+
			"atmos atlantis generate repo-config --config-template <config_template> --project-template <project_template> --stacks <stack1>,<stack2>\n"+
			"The filter can contain the names of the top-level stack manifests and the logical stack names (derived from the context vars)\n"+
			"atmos atlantis generate repo-config --config-template <config_template> --project-template <project_template> --stacks orgs/cp/tenant1/staging/us-east-2,orgs/cp/tenant2/dev/us-east-2\n"+
			"atmos atlantis generate repo-config --config-template <config_template> --project-template <project_template> --stacks tenant1-ue2-staging,tenant1-ue2-prod\n"+
			"atmos atlantis generate repo-config --config-template <config_template> --project-template <project_template> --stacks orgs/cp/tenant1/staging/us-east-2,tenant1-ue2-prod",
	)

	atlantisGenerateRepoConfigCmd.PersistentFlags().String("components", "",
		"Generate Atlantis projects for the specified components only (comma-separated values).\n"+
			"atmos atlantis generate repo-config --config-template <config_template> --project-template <project_template> --components <component1>,<component2>",
	)

	atlantisGenerateRepoConfigCmd.PersistentFlags().Bool("affected-only", false,
		"Generate Atlantis projects only for the Atmos components changed between two Git commits.\n"+
			"atmos atlantis generate repo-config --affected-only=true",
	)

	atlantisGenerateRepoConfigCmd.PersistentFlags().String("repo-path", "", "Filesystem path to the already cloned target repository with which to compare the current branch: atmos atlantis generate repo-config --affected-only=true --repo-path <path_to_already_cloned_repo>")
	atlantisGenerateRepoConfigCmd.PersistentFlags().String("ref", "", "Git reference with which to compare the current branch: atmos atlantis generate repo-config --affected-only=true --ref refs/heads/main. Refer to https://git-scm.com/book/en/v2/Git-Internals-Git-References for more details")
	atlantisGenerateRepoConfigCmd.PersistentFlags().String("sha", "", "Git commit SHA with which to compare the current branch: atmos atlantis generate repo-config --affected-only=true --sha 3a5eafeab90426bd82bf5899896b28cc0bab3073")
	atlantisGenerateRepoConfigCmd.PersistentFlags().Bool("verbose", false, "Print more detailed output when cloning and checking out the Git repository: atmos atlantis generate repo-config --affected-only=true --verbose=true")
	atlantisGenerateRepoConfigCmd.PersistentFlags().String("ssh-key", "", "Path to PEM-encoded private key to clone private repos using SSH: atmos atlantis generate repo-config --affected-only=true --ssh-key <path_to_ssh_key>")
	atlantisGenerateRepoConfigCmd.PersistentFlags().String("ssh-key-password", "", "Encryption password for the PEM-encoded private key if the key contains a password-encrypted PEM block: atmos atlantis generate repo-config --affected-only=true --ssh-key <path_to_ssh_key> --ssh-key-password <password>")

	atlantisGenerateCmd.PersistentFlags().Bool("clone-target-ref", false, "Clone the target reference with which to compare the current branch: "+
		"atmos atlantis generate repo-config --affected-only=true --clone-target-ref=true\n"+
		"The flag is only used when '--affected-only=true'\n"+
		"If set to 'false' (default), the target reference will be checked out instead\n"+
		"This requires that the target reference is already cloned by Git, and the information about it exists in the '.git' directory")

	atlantisGenerateCmd.AddCommand(atlantisGenerateRepoConfigCmd)
}
