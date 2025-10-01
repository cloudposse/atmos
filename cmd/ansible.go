package cmd

import (
	"github.com/spf13/cobra"

	errUtils "github.com/cloudposse/atmos/errors"
	e "github.com/cloudposse/atmos/internal/exec"
)

// ansibleCmd represents the base command for all Ansible sub-commands.
var ansibleCmd = &cobra.Command{
    Use:                "ansible",
    Aliases:            []string{"ans"},
    Short:              "Run Ansible playbooks with Atmos components",
    Long:               `Execute ansible-playbook against component-scoped vars and settings defined in Atmos stacks`,
    FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: true},
    Args:               cobra.NoArgs,
}

func init() {
    ansibleCmd.DisableFlagParsing = true
    ansibleCmd.PersistentFlags().Bool("", false, doubleDashHint)
    AddStackCompletion(ansibleCmd)
    attachAnsibleCommands(ansibleCmd)
    RootCmd.AddCommand(ansibleCmd)
}

func ansibleRun(parentCmd *cobra.Command, actualCmd *cobra.Command, commandName string, args []string) error {
    handleHelpRequest(parentCmd, args)
    diffArgs := []string{commandName}
    diffArgs = append(diffArgs, args...)
    info := getConfigAndStacksInfo("ansible", parentCmd, diffArgs)
    info.CliArgs = []string{"ansible", commandName}

    if info.NeedHelp {
        err := actualCmd.Usage()
        errUtils.CheckErrorPrintAndExit(err, "", "")
        return nil
    }

    flags := parentCmd.Flags()
    _ = flags

    ansibleFlags := e.AnsibleFlags{}
    return e.ExecuteAnsible(&info, &ansibleFlags)
}
