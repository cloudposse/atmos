package cmd

import (
	e "github.com/cloudposse/atmos/internal/exec"
	u "github.com/cloudposse/atmos/pkg/utils"
	"github.com/spf13/cobra"
)

// Command: atmos helmfile diff
var helmfileDiffShort = "Show differences between the desired and actual state of Helm releases."
var helmfileDiffLong = `This command calculates and displays the differences between the desired state of Helm releases
defined in your configurations and the actual state deployed in the cluster.

Example usage:
  atmos helmfile diff echo-server -s tenant1-ue2-dev
  atmos helmfile diff echo-server -s tenant1-ue2-dev --redirect-stderr /dev/null`

// Command: atmos helmfile apply
var helmfileApplyShort = "Apply changes to align the actual state of Helm releases with the desired state."
var helmfileApplyLong = `This command reconciles the actual state of Helm releases in the cluster with the desired state
defined in your configurations by applying the necessary changes.

Example usage:
  atmos helmfile apply echo-server -s tenant1-ue2-dev
  atmos helmfile apply echo-server -s tenant1-ue2-dev --redirect-stderr /dev/stdout`

// Command: atmos helmfile sync
var helmfileSyncShort = "Synchronize the state of Helm releases with the desired state without making changes."
var helmfileSyncLong = `This command ensures that the actual state of Helm releases in the cluster matches the desired
state defined in your configurations without performing destructive actions.

Example usage:
  atmos helmfile sync echo-server --stack tenant1-ue2-dev
  atmos helmfile sync echo-server --stack tenant1-ue2-dev --redirect-stderr ./errors.txt`

// Command: atmos helmfile destroy
var helmfileDestroyShort = "Destroy the Helm releases for the specified stack."
var helmfileDestroyLong = `This command removes the specified Helm releases from the cluster, ensuring a clean state for
the given stack.

Example usage:
  atmos helmfile destroy echo-server --stack=tenant1-ue2-dev
  atmos helmfile destroy echo-server --stack=tenant1-ue2-dev --redirect-stderr /dev/stdout`

// helmfileCmd represents the base command for all helmfile sub-commands
var helmfileCmd = &cobra.Command{
	Use:                "helmfile",
	Aliases:            []string{"hf"},
	Short:              "Manage Helmfile-based Kubernetes deployments",
	Long:               `This command runs Helmfile commands to manage Kubernetes deployments using Helmfile.`,
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: true},
}

func init() {
	// https://github.com/spf13/cobra/issues/739
	helmfileCmd.DisableFlagParsing = true
	helmfileCmd.PersistentFlags().StringP("stack", "s", "", "atmos helmfile <helmfile_command> <component> -s <stack>")
	addUsageCommand(helmfileCmd, false)
	RootCmd.AddCommand(helmfileCmd)
}

func helmfileRun(cmd *cobra.Command, commandName string, args []string) {
	handleHelpRequest(cmd, args, false)
	diffArgs := []string{commandName}
	diffArgs = append(diffArgs, args...)
	info := getConfigAndStacksInfo("helmfile", cmd, diffArgs)
	err := e.ExecuteHelmfile(info)
	if err != nil {
		u.LogErrorAndExit(atmosConfig, err)
	}
}
