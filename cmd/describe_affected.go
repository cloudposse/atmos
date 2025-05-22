package cmd

import (
	"errors"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/cloudposse/atmos/internal/exec"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

var ErrRepoPathConflict = errors.New("if the '--repo-path' flag is specified, the '--ref', '--sha', '--ssh-key' and '--ssh-key-password' flags can't be used")

type describeAffectedExecCreator func(atmosConfig *schema.AtmosConfiguration) exec.DescribeAffectedExec

// describeAffectedCmd produces a list of the affected Atmos components and stacks given two Git commits
var describeAffectedCmd = &cobra.Command{
	Use:                "affected",
	Short:              "List Atmos components and stacks affected by two Git commits",
	Long:               "Identify and list Atmos components and stacks impacted by changes between two Git commits.",
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: false},
	Args:               cobra.NoArgs,
	Run:                getRunnableDescribeAffectedCmd(checkAtmosConfig, parseDescribeAffectedCliArgs, exec.NewDescribeAffectedExec),
}

func getRunnableDescribeAffectedCmd(
	checkAtmosConfig func(opts ...AtmosValidateOption),
	parseDescribeAffectedCliArgs func(cmd *cobra.Command, args []string) (exec.DescribeAffectedCmdArgs, error),
	newDescribeAffectedExec describeAffectedExecCreator,
) func(cmd *cobra.Command, args []string) {
	return func(cmd *cobra.Command, args []string) {
		// Check Atmos configuration
		checkAtmosConfig()
		props, err := parseDescribeAffectedCliArgs(cmd, args)
		checkErrorAndExit(err)
		err = newDescribeAffectedExec(&props.CLIConfig).Execute(&props)
		checkErrorAndExit(err)
	}
}

func checkErrorAndExit(err error) {
	if err != nil {
		u.PrintErrorMarkdownAndExit("", err, "")
	}
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
	var atmosConfig schema.AtmosConfiguration
	if info, err := exec.ProcessCommandLineArgs("", cmd, args, nil); err != nil {
		return exec.DescribeAffectedCmdArgs{}, err
	} else if atmosConfig, err = cfg.InitCliConfig(info, true); err != nil {
		return exec.DescribeAffectedCmdArgs{}, err
	}

	if err := exec.ValidateStacks(atmosConfig); err != nil {
		return exec.DescribeAffectedCmdArgs{}, err
	}

	// Process flags
	flags := cmd.Flags()

	result := exec.DescribeAffectedCmdArgs{
		CLIConfig: atmosConfig,
	}
	setFlagValueInCliArgs(flags, &result)

	if result.Format != "yaml" && result.Format != "json" {
		return exec.DescribeAffectedCmdArgs{}, exec.ErrInvalidFormat
	}
	if result.RepoPath != "" && (result.Ref != "" || result.SHA != "" || result.SSHKeyPath != "" || result.SSHKeyPassword != "") {
		return exec.DescribeAffectedCmdArgs{}, ErrRepoPathConflict
	}

	return result, nil
}

func setFlagValueInCliArgs(flags *pflag.FlagSet, describe *exec.DescribeAffectedCmdArgs) {
	flagsKeyValue := map[string]any{
		"ref":                            &describe.Ref,
		"sha":                            &describe.SHA,
		"repo-path":                      &describe.RepoPath,
		"ssh-key":                        &describe.SSHKeyPath,
		"ssh-key-password":               &describe.SSHKeyPassword,
		"include-spacelift-admin-stacks": &describe.IncludeSpaceliftAdminStacks,
		"include-dependents":             &describe.IncludeDependents,
		"include-settings":               &describe.IncludeSettings,
		"upload":                         &describe.Upload,
		"clone-target-ref":               &describe.CloneTargetRef,
		"process-templates":              &describe.ProcessTemplates,
		"process-functions":              &describe.ProcessYamlFunctions,
		"skip":                           &describe.Skip,
		"pager":                          &describe.CLIConfig.Settings.Terminal.Pager,
		"stack":                          &describe.Stack,
		"format":                         &describe.Format,
		"file":                           &describe.OutputFile,
		"query":                          &describe.Query,
	}

	var err error
	for k := range flagsKeyValue {
		if !flags.Changed(k) {
			continue
		}
		switch v := flagsKeyValue[k].(type) {
		case *string:
			*v, err = flags.GetString(k)
		case *bool:
			*v, err = flags.GetBool(k)
		case *[]string:
			*v, err = flags.GetStringSlice(k)
		default:
			panic(fmt.Sprintf("unsupported type %T for flag %s", v, k))
		}
		checkFlagError(err)
	}
	// When uploading, always include dependents and settings for all affected components
	if describe.Upload {
		describe.IncludeDependents = true
		describe.IncludeSettings = true
	}
	if describe.Format == "" {
		describe.Format = "json"
	}
}

func checkFlagError(err error) {
	if err != nil {
		panic(err)
	}
}
