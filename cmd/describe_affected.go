package cmd

import (
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/spf13/cobra"

	e "github.com/cloudposse/atmos/internal/exec"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// describeAffectedCmd produces a list of the affected Atmos components and stacks given two Git commits
var describeAffectedCmd = &cobra.Command{
	Use:                "affected",
	Short:              "List Atmos components and stacks affected by two Git commits",
	Long:               "Identify and list Atmos components and stacks impacted by changes between two Git commits.",
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: false},
	Args:               cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		// Check Atmos configuration
		checkAtmosConfig()

		err := e.ExecuteDescribeAffectedCmd(cmd, args)
		if err != nil {
			u.LogErrorAndExit(schema.AtmosConfiguration{}, err)
		}
	},
}

func init() {
	describeAffectedCmd.DisableFlagParsing = false

	describeAffectedCmd.PersistentFlags().String("repo-path", "", "Filesystem path to the already cloned target repository with which to compare the current branch: atmos describe affected --repo-path <path_to_already_cloned_repo>")
	describeAffectedCmd.PersistentFlags().String("ref", "", "Git reference with which to compare the current branch: atmos describe affected --ref refs/heads/main. Refer to https://git-scm.com/book/en/v2/Git-Internals-Git-References for more details")
	describeAffectedCmd.PersistentFlags().String("sha", "", "Git commit SHA with which to compare the current branch: atmos describe affected --sha 3a5eafeab90426bd82bf5899896b28cc0bab3073")
	describeAffectedCmd.PersistentFlags().String("file", "", "Write the result to the file: atmos describe affected --ref refs/tags/v1.75.0 --file affected.json")
	describeAffectedCmd.PersistentFlags().String("format", "json", "The output format: atmos describe affected --format=json|yaml ('json' is default)")
	describeAffectedCmd.PersistentFlags().Bool("verbose", false, "Print more detailed output when cloning and checking out the Git repository: atmos describe affected --verbose=true")
	describeAffectedCmd.PersistentFlags().String("ssh-key", "", "Path to PEM-encoded private key to clone private repos using SSH: atmos describe affected --ssh-key <path_to_ssh_key>")
	describeAffectedCmd.PersistentFlags().String("ssh-key-password", "", "Encryption password for the PEM-encoded private key if the key contains a password-encrypted PEM block: atmos describe affected --ssh-key <path_to_ssh_key> --ssh-key-password <password>")
	describeAffectedCmd.PersistentFlags().Bool("include-spacelift-admin-stacks", false, "Include the Spacelift admin stack of any stack that is affected by config changes: atmos describe affected --include-spacelift-admin-stacks=true")
	describeAffectedCmd.PersistentFlags().Bool("include-dependents", false, "Include the dependent components and stacks: atmos describe affected --include-dependents=true")
	describeAffectedCmd.PersistentFlags().Bool("include-settings", false, "Include the 'settings' section for each affected component: atmos describe affected --include-settings=true")
	describeAffectedCmd.PersistentFlags().Bool("upload", false, "Upload the affected components and stacks to a specified HTTP endpoint: atmos describe affected --upload=true")
	describeAffectedCmd.PersistentFlags().StringP("stack", "s", "", "atmos describe affected -s <stack>")
	AddStackCompltion(describeAffectedCmd)
	describeAffectedCmd.PersistentFlags().Bool("clone-target-ref", false, "Clone the target reference with which to compare the current branch: atmos describe affected --clone-target-ref=true\n"+
		"If set to 'false' (default), the target reference will be checked out instead\n"+
		"This requires that the target reference is already cloned by Git, and the information about it exists in the '.git' directory")

	describeCmd.AddCommand(describeAffectedCmd)
}
