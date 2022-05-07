// https://github.com/roboll/helmfile#cli-reference

package exec

import (
	"fmt"
	c "github.com/cloudposse/atmos/pkg/config"
	u "github.com/cloudposse/atmos/pkg/utils"
	"github.com/fatih/color"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"os"
	"path"
)

// ExecuteHelmfile executes helmfile commands
func ExecuteHelmfile(cmd *cobra.Command, args []string) error {
	info, err := processArgsConfigAndStacks("helmfile", cmd, args)
	if err != nil {
		return err
	}

	if info.NeedHelp == true {
		return nil
	}

	if len(info.Stack) < 1 {
		return errors.New("stack must be specified")
	}

	err = checkHelmfileConfig()
	if err != nil {
		return err
	}

	// Check if the component exists as a helmfile component
	componentPath := path.Join(c.ProcessedConfig.HelmfileDirAbsolutePath, info.ComponentFolderPrefix, info.FinalComponent)
	componentPathExists, err := u.IsDirectory(componentPath)
	if err != nil || !componentPathExists {
		return errors.New(fmt.Sprintf("Component '%s' is defined as Helmfile component in '%s', but it does not exist in '%s'",
			info.FinalComponent,
			info.ComponentFromArg,
			path.Join(c.Config.Components.Helmfile.BasePath, info.ComponentFolderPrefix),
		))
	}

	// Check if the component is allowed to be provisioned (`metadata.type` attribute)
	if (info.SubCommand == "sync" || info.SubCommand == "apply" || info.SubCommand == "deploy") && info.ComponentIsAbstract {
		return errors.New(fmt.Sprintf("Abstract component '%s' cannot be provisioned since it's explicitly prohibited from being deployed "+
			"by 'metadata.type: abstract' attribute", path.Join(info.ComponentFolderPrefix, info.Component)))
	}

	// Print component variables
	u.PrintInfo(fmt.Sprintf("\nVariables for the component '%s' in the stack '%s':\n", info.ComponentFromArg, info.Stack))
	err = u.PrintAsYAML(info.ComponentVarsSection)
	if err != nil {
		return err
	}

	// Write variables to a file
	varFile := constructHelmfileComponentVarfileName(info)
	varFilePath := constructHelmfileComponentVarfilePath(info)

	u.PrintInfo("Writing the variables to file:")
	fmt.Println(varFilePath)

	if !info.DryRun {
		err = u.WriteToFileAsYAML(varFilePath, info.ComponentVarsSection, 0644)
		if err != nil {
			return err
		}
	}

	// Handle `helmfile deploy` custom command
	if info.SubCommand == "deploy" {
		info.SubCommand = "sync"
	}

	context := c.GetContextFromVars(info.ComponentVarsSection)

	// Prepare AWS profile
	helmAwsProfile := c.ReplaceContextTokens(context, c.Config.Components.Helmfile.HelmAwsProfilePattern)
	u.PrintInfo(fmt.Sprintf("\nUsing AWS_PROFILE=%s\n\n", helmAwsProfile))

	// Download kubeconfig by running `aws eks update-kubeconfig`
	kubeconfigPath := fmt.Sprintf("%s/%s-kubecfg", c.Config.Components.Helmfile.KubeconfigPath, info.ContextPrefix)
	clusterName := c.ReplaceContextTokens(context, c.Config.Components.Helmfile.ClusterNamePattern)
	u.PrintInfo(fmt.Sprintf("Downloading kubeconfig from the cluster '%s' and saving it to %s\n\n", clusterName, kubeconfigPath))

	err = ExecuteShellCommand("aws",
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
		info.DryRun,
	)
	if err != nil {
		return err
	}

	// Print command info
	u.PrintInfo("\nCommand info:")
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
		fmt.Println("Helmfile component: " + info.BaseComponent)
	}

	if info.Stack == info.StackFromArg {
		fmt.Println("Stack: " + info.StackFromArg)
	} else {
		fmt.Println("Stack: " + info.StackFromArg)
		fmt.Println("Stack path: " + path.Join(c.Config.BasePath, c.Config.Stacks.BasePath, info.Stack))
	}

	workingDir := constructHelmfileComponentWorkingDir(info)
	fmt.Println(fmt.Sprintf("Working dir: %s\n\n", workingDir))

	// Prepare arguments and flags
	allArgsAndFlags := []string{"--state-values-file", varFile}
	if info.GlobalOptions != nil && len(info.GlobalOptions) > 0 {
		allArgsAndFlags = append(allArgsAndFlags, info.GlobalOptions...)
	}
	allArgsAndFlags = append(allArgsAndFlags, info.SubCommand)
	allArgsAndFlags = append(allArgsAndFlags, info.AdditionalArgsAndFlags...)

	// Prepare ENV vars
	envVars := append(info.ComponentEnvList, []string{
		fmt.Sprintf("AWS_PROFILE=%s", helmAwsProfile),
		fmt.Sprintf("KUBECONFIG=%s", kubeconfigPath),
		fmt.Sprintf("NAMESPACE=%s", context.Namespace),
		fmt.Sprintf("TENANT=%s", context.Tenant),
		fmt.Sprintf("ENVIRONMENT=%s", context.Environment),
		fmt.Sprintf("STAGE=%s", context.Stage),
		fmt.Sprintf("REGION=%s", context.Region),
		fmt.Sprintf("STACK=%s", info.Stack),
	}...)

	u.PrintInfo("Using ENV vars:\n")
	for _, v := range envVars {
		fmt.Println(v)
	}

	err = ExecuteShellCommand(info.Command, allArgsAndFlags, componentPath, envVars, info.DryRun)
	if err != nil {
		return err
	}

	// Cleanup
	err = os.Remove(varFilePath)
	if err != nil {
		color.Yellow("Error deleting the helmfile varfile: %s\n", err)
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
