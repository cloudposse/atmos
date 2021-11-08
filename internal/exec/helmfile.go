// https://github.com/roboll/helmfile#cli-reference

package exec

import (
	"fmt"
	c "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/utils"
	"github.com/fatih/color"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"os"
	"path"
)

// ExecuteHelmfile executes helmfile commands
func ExecuteHelmfile(cmd *cobra.Command, args []string) error {
	info, err := processConfigAndStacks("helmfile", cmd, args)
	if err != nil {
		return err
	}

	if len(info.Stack) < 1 {
		return errors.New("the specified stack does not exist")
	}

	err = checkHelmfileConfig()
	if err != nil {
		return err
	}

	componentPath := path.Join(c.ProcessedConfig.HelmfileDirAbsolutePath, info.ComponentFolderPrefix, info.Component)
	componentPathExists, err := utils.IsDirectory(componentPath)
	if err != nil || !componentPathExists {
		return errors.New(fmt.Sprintf("Component '%s' does not exixt in %s",
			info.Component,
			path.Join(c.ProcessedConfig.HelmfileDirAbsolutePath, info.ComponentFolderPrefix),
		))
	}

	// Write variables to a file
	var varFileName string
	if len(info.ComponentFolderPrefix) == 0 {
		varFileName = fmt.Sprintf("%s/%s/%s-%s.helmfile.vars.yaml",
			c.Config.Components.Helmfile.BasePath,
			info.Component,
			info.ContextPrefix,
			info.Component,
		)
	} else {
		varFileName = fmt.Sprintf("%s/%s/%s/%s-%s.helmfile.vars.yaml",
			c.Config.Components.Helmfile.BasePath,
			info.ComponentFolderPrefix,
			info.Component,
			info.ContextPrefix,
			info.Component,
		)
	}

	color.Cyan("Writing variables to file:")
	fmt.Println(varFileName)
	err = utils.WriteToFileAsYAML(varFileName, info.ComponentVarsSection, 0644)
	if err != nil {
		return err
	}

	// Handle `helmfile deploy` custom command
	if info.SubCommand == "deploy" {
		info.SubCommand = "sync"
	}

	context := c.GetContextFromVars(info.ComponentVarsSection)

	// Prepare AWS profile
	helmAwsProfile := c.ReplaceContextTokens(context, c.Config.Components.Helmfile.HelmAwsProfilePattern)
	color.Cyan(fmt.Sprintf("\nUsing AWS_PROFILE=%s\n\n", helmAwsProfile))

	// Download kubeconfig by running `aws eks update-kubeconfig`
	kubeconfigPath := fmt.Sprintf("%s/%s-kubecfg", c.Config.Components.Helmfile.KubeconfigPath, info.ContextPrefix)
	clusterName := c.ReplaceContextTokens(context, c.Config.Components.Helmfile.ClusterNamePattern)
	color.Cyan(fmt.Sprintf("Downloading kubeconfig from the cluster '%s' and saving it to %s\n\n", clusterName, kubeconfigPath))

	err = execCommand("aws",
		[]string{
			"--profile",
			helmAwsProfile,
			"eks",
			"update-kubeconfig",
			fmt.Sprintf("--name=%s", clusterName),
			fmt.Sprintf("--region=%s", context.Region),
			fmt.Sprintf("--kubeconfig=%s", kubeconfigPath),
		},
		componentPath,
		nil,
	)
	if err != nil {
		return err
	}

	// Print command info
	color.Cyan("\nCommand info:")
	fmt.Println("Helmfile binary: " + info.Command)
	fmt.Println("Helmfile command: " + info.SubCommand)

	// https://github.com/roboll/helmfile#cli-reference
	// atmos helmfile diff echo-server -s tenant1-ue2-dev --global-options "--no-color --namespace=test"
	// atmos helmfile diff echo-server -s tenant1-ue2-dev --global-options "--no-color --namespace test"
	// atmos helmfile diff echo-server -s tenant1-ue2-dev --global-options="--no-color --namespace=test"
	// atmos helmfile diff echo-server -s tenant1-ue2-dev --global-options="--no-color --namespace test"
	fmt.Println(fmt.Sprintf("Global options: %v", info.GlobalOptions))

	fmt.Println(fmt.Sprintf("Arguments and flags: %v", info.AdditionalArgsAndFlags))
	fmt.Println("Component: " + info.ComponentFromArg)
	if len(info.BaseComponent) > 0 {
		fmt.Println("Base component: " + info.BaseComponent)
	}
	fmt.Println("Stack: " + info.Stack)

	var workingDir string
	if len(info.ComponentFolderPrefix) == 0 {
		workingDir = fmt.Sprintf("%s/%s", c.Config.Components.Helmfile.BasePath, info.Component)
	} else {
		workingDir = fmt.Sprintf("%s/%s/%s", c.Config.Components.Helmfile.BasePath, info.ComponentFolderPrefix, info.Component)
	}
	fmt.Println(fmt.Sprintf("Working dir: %s\n\n", workingDir))

	varFile := fmt.Sprintf("%s-%s.helmfile.vars.yaml", info.ContextPrefix, info.Component)

	// Prepare arguments and flags
	allArgsAndFlags := []string{"--state-values-file", varFile}
	if info.GlobalOptions != nil && len(info.GlobalOptions) > 0 {
		allArgsAndFlags = append(allArgsAndFlags, info.GlobalOptions...)
	}
	allArgsAndFlags = append(allArgsAndFlags, info.SubCommand)
	allArgsAndFlags = append(allArgsAndFlags, info.AdditionalArgsAndFlags...)

	// Prepare ENV vars
	envVars := []string{
		fmt.Sprintf("AWS_PROFILE=%s", helmAwsProfile),
		fmt.Sprintf("KUBECONFIG=%s", kubeconfigPath),
		fmt.Sprintf("NAMESPACE=%s", context.Namespace),
		fmt.Sprintf("TENANT=%s", context.Tenant),
		fmt.Sprintf("ENVIRONMENT=%s", context.Environment),
		fmt.Sprintf("STAGE=%s", context.Stage),
		fmt.Sprintf("REGION=%s", context.Region),
		fmt.Sprintf("STACK=%s", info.Stack),
	}

	err = execCommand(info.Command, allArgsAndFlags, componentPath, envVars)
	if err != nil {
		return err
	}

	// Cleanup
	err = os.Remove(varFileName)
	if err != nil {
		color.Yellow("Error deleting helmfile varfile: %s\n", err)
	}

	return nil
}

func checkHelmfileConfig() error {
	if len(c.Config.Components.Helmfile.BasePath) < 1 {
		return errors.New("Base path to helmfile components must be provided in 'components.helmfile.base_path' config or " +
			"'ATMOS_COMPONENTS_HELMFILE_BASE_PATH' ENV variable")
	}

	if len(c.Config.Components.Helmfile.KubeconfigPath) < 1 {
		return errors.New("Kubeconfig path must be provided in 'components.helmfile.kubeconfig_path' config or " +
			"'ATMOS_COMPONENTS_HELMFILE_KUBECONFIG_PATH' ENV variable")
	}

	if len(c.Config.Components.Helmfile.HelmAwsProfilePattern) < 1 {
		return errors.New("Helm AWS profile pattern must be provided in 'components.helmfile.helm_aws_profile_pattern' config or " +
			"'ATMOS_COMPONENTS_HELMFILE_HELM_AWS_PROFILE_PATTERN' ENV variable")
	}

	if len(c.Config.Components.Helmfile.ClusterNamePattern) < 1 {
		return errors.New("Cluster name pattern must be provided in 'components.helmfile.cluster_name_pattern' config or " +
			"'ATMOS_COMPONENTS_HELMFILE_CLUSTER_NAME_PATTERN' ENV variable")
	}

	return nil
}
