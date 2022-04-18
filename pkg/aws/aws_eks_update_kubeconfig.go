package exec

import (
	"fmt"
	e "github.com/cloudposse/atmos/internal/exec"
	c "github.com/cloudposse/atmos/pkg/config"
	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

func ExecuteAwsEksUpdateKubeconfigCommand(cmd *cobra.Command, args []string) error {
	return nil
}

// ExecuteAwsEksUpdateKubeconfig executes 'aws eks update-kubeconfig' command
func ExecuteAwsEksUpdateKubeconfig(info c.ConfigAndStacksInfo, path string) error {
	// Prepare AWS profile
	context := c.GetContextFromVars(info.ComponentVarsSection)
	helmAwsProfile := c.ReplaceContextTokens(context, c.Config.Components.Helmfile.HelmAwsProfilePattern)
	color.Cyan(fmt.Sprintf("\nUsing AWS_PROFILE=%s\n\n", helmAwsProfile))

	// Download `kubeconfig` by running `aws eks update-kubeconfig`
	kubeconfigPath := fmt.Sprintf("%s/%s-kubecfg", c.Config.Components.Helmfile.KubeconfigPath, info.ContextPrefix)
	clusterName := c.ReplaceContextTokens(context, c.Config.Components.Helmfile.ClusterNamePattern)
	color.Cyan(fmt.Sprintf("Downloading kubeconfig from the cluster '%s' and saving it to '%s'\n\n", clusterName, kubeconfigPath))

	err := e.ExecuteShellCommand("aws",
		[]string{
			"eks",
			"update-kubeconfig",
			"--profile",
			helmAwsProfile,
			fmt.Sprintf("--name=%s", clusterName),
			fmt.Sprintf("--region=%s", context.Region),
			fmt.Sprintf("--kubeconfig=%s", kubeconfigPath),
		},
		path,
		nil,
		false,
	)
	if err != nil {
		return err
	}

	return nil
}
