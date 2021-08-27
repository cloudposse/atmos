// https://github.com/roboll/helmfile#cli-reference

package exec

import (
	c "atmos/internal/config"
	u "atmos/internal/utils"
	"fmt"
	"github.com/fatih/color"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"os"
	"path"
	"strings"
)

// ExecuteHelmfile executes helmfile commands
func ExecuteHelmfile(cmd *cobra.Command, args []string) error {
	stack, componentFromArg, component, baseComponent, command, subCommand, componentVarsSection, additionalArgsAndFlags,
		err := processConfigAndStacks("helmfile", cmd, args)

	err = checkHelmfileConfig()
	if err != nil {
		return err
	}

	componentPath := path.Join(c.ProcessedConfig.HelmfileDirAbsolutePath, component)
	componentPathExists, err := u.IsDirectory(componentPath)
	if err != nil || !componentPathExists {
		return errors.New(fmt.Sprintf("Component '%s' does not exixt in %s", component, c.ProcessedConfig.HelmfileDirAbsolutePath))
	}

	// Write variables to a file
	stackNameFormatted := strings.Replace(stack, "/", "-", -1)
	varFileName := fmt.Sprintf("%s/%s/%s-%s.helmfile.vars.yaml", c.Config.Components.Helmfile.BasePath, component, stackNameFormatted, componentFromArg)
	color.Cyan("Writing variables to file:")
	fmt.Println(varFileName)
	err = u.WriteToFileAsYAML(varFileName, componentVarsSection, 0644)
	if err != nil {
		return err
	}

	// Handle `helmfile deploy` custom command
	if subCommand == "deploy" {
		subCommand = "sync"
	}

	context := getContextFromVars(componentVarsSection)

	// Prepare AWS profile
	helmAwsProfile := replaceContextTokens(context, c.Config.Components.Helmfile.HelmAwsProfilePattern)
	color.Cyan(fmt.Sprintf("\nUsing AWS_PROFILE=%s\n\n", helmAwsProfile))

	// Download kubeconfig by running `aws eks update-kubeconfig`
	kubeconfigPath := fmt.Sprintf("%s/%s-kubecfg", c.Config.Components.Helmfile.KubeconfigPath, stackNameFormatted)
	clusterName := replaceContextTokens(context, c.Config.Components.Helmfile.ClusterNamePattern)
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
	color.Green("Helmfile binary: " + command)
	color.Green("Helmfile command: " + subCommand)
	color.Green("Arguments and flags: %v", additionalArgsAndFlags)
	color.Green("Component: " + componentFromArg)
	if len(baseComponent) > 0 {
		color.Green("Base component: " + baseComponent)
	}
	color.Green("Stack: " + stack)
	workingDir := fmt.Sprintf("%s/%s", c.Config.Components.Helmfile.BasePath, component)
	color.Green(fmt.Sprintf("Working dir: %s\n\n", workingDir))
	fmt.Println()

	varFile := fmt.Sprintf("%s-%s.helmfile.vars.yaml", stackNameFormatted, componentFromArg)

	// Prepare arguments and flags
	allArgsAndFlags := []string{"--state-values-file", varFile}
	allArgsAndFlags = append(allArgsAndFlags, subCommand)
	allArgsAndFlags = append(allArgsAndFlags, additionalArgsAndFlags...)

	// Prepare ENV vars
	envVars := []string{
		fmt.Sprintf("AWS_PROFILE=%s", helmAwsProfile),
		fmt.Sprintf("KUBECONFIG=%s", kubeconfigPath),
		fmt.Sprintf("NAMESPACE=%s", context.Namespace),
		fmt.Sprintf("TENANT=%s", context.Tenant),
		fmt.Sprintf("ENVIRONMENT=%s", context.Environment),
		fmt.Sprintf("STAGE=%s", context.Stage),
		fmt.Sprintf("REGION=%s", context.Region),
		fmt.Sprintf("STACK=%s", stackNameFormatted),
	}

	// Execute the command
	emoji, err := u.UnquoteCodePoint("\\U+1F680")
	if err != nil {
		return err
	}

	color.Cyan(fmt.Sprintf("\nExecuting command  %v", emoji))
	color.Green(fmt.Sprintf("Command: %s %s %s",
		command,
		subCommand,
		u.SliceOfStringsToSpaceSeparatedString(additionalArgsAndFlags),
	))

	err = execCommand(command, allArgsAndFlags, componentPath, envVars)
	if err != nil {
		return err
	}

	// Cleanup
	varFilePath := fmt.Sprintf("%s/%s/%s", c.ProcessedConfig.HelmfileDirAbsolutePath, component, varFile)
	err = os.Remove(varFilePath)
	if err != nil {
		color.Yellow("Error deleting helmfile var file: %s\n", err)
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
