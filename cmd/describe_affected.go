package cmd

import (
	"errors"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/cloudposse/atmos/internal/exec"
	cfg "github.com/cloudposse/atmos/pkg/config"
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
		props, err := parseDescribeAffectedCliArgs(cmd, args)
		if err != nil {
			u.PrintErrorMarkdownAndExit("", err, "")
		}
		err = exec.NewDescribeAffectedExec(&props.CLIConfig).Execute(props)
		if err != nil {
			u.PrintErrorMarkdownAndExit("", err, "")
		}
	},
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

	describeCmd.AddCommand(describeAffectedCmd)
}

func parseDescribeAffectedCliArgs(cmd *cobra.Command, args []string) (exec.DescribeAffectedCmdArgs, error) {
	info, err := exec.ProcessCommandLineArgs("", cmd, args, nil)
	if err != nil {
		return exec.DescribeAffectedCmdArgs{}, err
	}

	atmosConfig, err := cfg.InitCliConfig(info, true)
	if err != nil {
		return exec.DescribeAffectedCmdArgs{}, err
	}

	err = exec.ValidateStacks(atmosConfig)
	if err != nil {
		return exec.DescribeAffectedCmdArgs{}, err
	}

	// Process flags
	flags := cmd.Flags()

	if flags.Changed("pager") {
		atmosConfig.Settings.Terminal.Pager, err = flags.GetString("pager")
		checkFlagError(err)
	}

	ref, err := flags.GetString("ref")
	checkFlagError(err)

	sha, err := flags.GetString("sha")
	checkFlagError(err)

	repoPath, err := flags.GetString("repo-path")
	checkFlagError(err)

	format, err := flags.GetString("format")
	checkFlagError(err)

	if format != "" && format != "yaml" && format != "json" {
		return exec.DescribeAffectedCmdArgs{}, fmt.Errorf("invalid '--format' flag '%s'. Valid values are 'json' (default) and 'yaml'", format)
	}

	if format == "" {
		format = "json"
	}

	file, err := flags.GetString("file")
	checkFlagError(err)

	sshKeyPath, err := flags.GetString("ssh-key")
	checkFlagError(err)

	sshKeyPassword, err := flags.GetString("ssh-key-password")
	checkFlagError(err)

	includeSpaceliftAdminStacks, err := flags.GetBool("include-spacelift-admin-stacks")
	checkFlagError(err)

	includeDependents, err := flags.GetBool("include-dependents")
	checkFlagError(err)

	includeSettings, err := flags.GetBool("include-settings")
	checkFlagError(err)

	upload, err := flags.GetBool("upload")
	checkFlagError(err)

	cloneTargetRef, err := flags.GetBool("clone-target-ref")
	checkFlagError(err)

	stack, err := flags.GetString("stack")
	checkFlagError(err)

	if repoPath != "" && (ref != "" || sha != "" || sshKeyPath != "" || sshKeyPassword != "") {
		return exec.DescribeAffectedCmdArgs{}, errors.New("if the '--repo-path' flag is specified, the '--ref', '--sha', '--ssh-key' and '--ssh-key-password' flags can't be used")
	}

	// When uploading, always include dependents and settings for all affected components
	if upload {
		includeDependents = true
		includeSettings = true
	}

	query, err := flags.GetString("query")
	checkFlagError(err)

	processTemplates, err := flags.GetBool("process-templates")
	checkFlagError(err)

	processYamlFunctions, err := flags.GetBool("process-functions")
	checkFlagError(err)

	skip, err := flags.GetStringSlice("skip")
	checkFlagError(err)

	result := exec.DescribeAffectedCmdArgs{
		CLIConfig:                   atmosConfig,
		CloneTargetRef:              cloneTargetRef,
		Format:                      format,
		IncludeDependents:           includeDependents,
		IncludeSettings:             includeSettings,
		IncludeSpaceliftAdminStacks: includeSpaceliftAdminStacks,
		OutputFile:                  file,
		Ref:                         ref,
		RepoPath:                    repoPath,
		SHA:                         sha,
		SSHKeyPath:                  sshKeyPath,
		SSHKeyPassword:              sshKeyPassword,
		Upload:                      upload,
		Stack:                       stack,
		Query:                       query,
		ProcessTemplates:            processTemplates,
		ProcessYamlFunctions:        processYamlFunctions,
		Skip:                        skip,
	}

	return result, nil
}

func checkFlagError(err error) {
	if err != nil {
		panic(err)
	}
}
