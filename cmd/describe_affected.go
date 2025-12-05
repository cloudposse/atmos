package cmd

import (
	"os"

	"github.com/spf13/cobra"

	"github.com/cloudposse/atmos/internal/exec"
	log "github.com/cloudposse/atmos/pkg/logger"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// describeAffectedCmd produces a list of the affected Atmos components and stacks given two Git commits.
var describeAffectedCmd = &cobra.Command{
	Use:                "affected",
	Short:              "List Atmos components and stacks affected by two Git commits",
	Long:               "Identify and list Atmos components and stacks impacted by changes between two Git commits.",
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: false},
	Args:               cobra.NoArgs,
	RunE:               getRunnableDescribeAffectedCmd(checkAtmosConfig, exec.ParseDescribeAffectedCliArgs, exec.NewDescribeAffectedExec),
}

func init() {
	describeAffectedCmd.DisableFlagParsing = false

	describeAffectedCmd.PersistentFlags().String("repo-path", "", "Filesystem path to the already cloned target repository with which to compare the current branch")
	describeAffectedCmd.PersistentFlags().String("ref", "", "Git reference with which to compare the current branch. Refer to [10.3 Git Internals Git References](https://git-scm.com/book/en/v2/Git-Internals-Git-References) for more details")
	describeAffectedCmd.PersistentFlags().String("sha", "", "Git commit SHA with which to compare the current branch")
	describeAffectedCmd.PersistentFlags().String("file", "", "Write the result to the file")
	describeAffectedCmd.PersistentFlags().String("format", "json", "The output format. (`json` is default)")
	describeAffectedCmd.PersistentFlags().String("ssh-key", "", "Path to PEM-encoded private key to clone private repos using SSH")
	describeAffectedCmd.PersistentFlags().String("ssh-key-password", "", "Encryption password for the PEM-encoded private key if the key contains a password-encrypted PEM block")
	describeAffectedCmd.PersistentFlags().Bool("include-spacelift-admin-stacks", false, "Include the Spacelift admin stack of any stack that is affected by config changes")
	describeAffectedCmd.PersistentFlags().Bool("include-dependents", false, "Include the dependent components and stacks")
	describeAffectedCmd.PersistentFlags().Bool("include-settings", false, "Include the `settings` section for each affected component")
	describeAffectedCmd.PersistentFlags().Bool("upload", false, "Upload the affected components and stacks to a specified HTTP endpoint")
	AddStackCompletion(describeAffectedCmd)
	describeAffectedCmd.PersistentFlags().Bool("clone-target-ref", false, "Clone the target reference with which to compare the current branch\n"+
		"If set to `false` (default), the target reference will be checked out instead\n"+
		"This requires that the target reference is already cloned by Git, and the information about it exists in the `.git` directory")

	describeAffectedCmd.PersistentFlags().Bool("process-templates", true, "Enable/disable Go template processing in Atmos stack manifests when executing the command")
	describeAffectedCmd.PersistentFlags().Bool("process-functions", true, "Enable/disable YAML functions processing in Atmos stack manifests when executing the command")
	describeAffectedCmd.PersistentFlags().StringSlice("skip", nil, "Skip executing a YAML function when processing Atmos stack manifests")
	describeAffectedCmd.PersistentFlags().Bool("verbose", false, "Deprecated. Alias for `--logs-level=Debug`")
	describeAffectedCmd.PersistentFlags().Bool("exclude-locked", false, "Exclude the locked components (`metadata.locked: true`) from the output")

	describeCmd.AddCommand(describeAffectedCmd)
}

// getRunnableDescribeAffectedCmd returns a command to run `atmos describe affected`.
func getRunnableDescribeAffectedCmd(
	checkAtmosConfig func(opts ...AtmosValidateOption),
	parseDescribeAffectedCliArgs func(cmd *cobra.Command, args []string) (exec.DescribeAffectedCmdArgs, error),
	newDescribeAffectedExec exec.DescribeAffectedExecCreator,
) func(cmd *cobra.Command, args []string) error {
	return func(cmd *cobra.Command, args []string) error {
		// Check Atmos configuration
		checkAtmosConfig()

		props, err := parseDescribeAffectedCliArgs(cmd, args)
		if err != nil {
			return err
		}

		// Handle the deprecated `--verbose` flag.
		if cmd.Flags().Changed("verbose") {
			log.Warn("The --verbose flag is deprecated. Please use the --logs-level flag instead", "example", "atmos describe affected --logs-level=Debug")
			if props.Verbose {
				log.SetLevel(log.DebugLevel)
				props.CLIConfig.Logs.Level = u.LogLevelDebug
			}
		}

		// Get identity from flag and create AuthManager if provided.
		// Use the WithAtmosConfig variant to enable stack-level default identity loading.
		identityName := GetIdentityFromFlags(cmd, os.Args)
		authManager, err := CreateAuthManagerFromIdentityWithAtmosConfig(identityName, &props.CLIConfig.Auth, props.CLIConfig)
		if err != nil {
			return err
		}
		props.AuthManager = authManager

		// Global --pager flag is now handled in cfg.InitCliConfig

		err = newDescribeAffectedExec(props.CLIConfig).Execute(&props)
		return err
	}
}
