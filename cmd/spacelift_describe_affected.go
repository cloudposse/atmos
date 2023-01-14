package cmd

import (
	"github.com/spf13/cobra"

	e "github.com/cloudposse/atmos/internal/exec"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// spaceliftDescribeAffectedCmd produces a list of the affected Spacelift stacks given two Git commits
var spaceliftDescribeAffectedCmd = &cobra.Command{
	Use:                "affected",
	Short:              "Execute 'spacelift describe affected' command",
	Long:               `This command produces a list of the affected Spacelift stacks given two Git commits: atmos spacelift describe affected [options]`,
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: false},
	Run: func(cmd *cobra.Command, args []string) {
		err := e.ExecuteSpaceliftDescribeAffectedCmd(cmd, args)
		if err != nil {
			u.PrintErrorToStdErrorAndExit(err)
		}
	},
}

func init() {
	spaceliftDescribeAffectedCmd.DisableFlagParsing = false

	spaceliftDescribeAffectedCmd.PersistentFlags().String("ref", "", "Git reference with which to compare the current branch: atmos spacelift describe affected --ref refs/heads/main. Refer to https://git-scm.com/book/en/v2/Git-Internals-Git-References for more details")
	spaceliftDescribeAffectedCmd.PersistentFlags().String("sha", "", "Git commit SHA with which to compare the current branch: atmos spacelift describe affected --sha 3a5eafeab90426bd82bf5899896b28cc0bab3073")
	spaceliftDescribeAffectedCmd.PersistentFlags().String("file", "", "Write the result to the file: atmos spacelift describe affected --ref refs/tags/v1.16.0 --file affected.json")
	spaceliftDescribeAffectedCmd.PersistentFlags().String("format", "json", "The output format: atmos spacelift describe affected --format=json|yaml ('json' is default)")
	spaceliftDescribeAffectedCmd.PersistentFlags().Bool("verbose", false, "Print more detailed output when cloning and checking out the Git repository: atmos spacelift describe affected --verbose=true")
	spaceliftDescribeAffectedCmd.PersistentFlags().String("ssh-key", "", "Path to PEM-encoded private key to clone private repos using SSH: atmos spacelift describe affected --ssh-key <path_to_ssh_key>")
	spaceliftDescribeAffectedCmd.PersistentFlags().String("ssh-key-password", "", "Encryption password for the PEM-encoded private key if the key contains a password-encrypted PEM block: atmos spacelift describe affected --ssh-key <path_to_ssh_key> --ssh-key-password <password>")

	spaceliftDescribeCmd.AddCommand(spaceliftDescribeAffectedCmd)
}
