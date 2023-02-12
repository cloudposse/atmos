// https://github.com/roboll/helmfile#cli-reference

package exec

import (
	"fmt"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"os"
	"path"

	cfg "github.com/cloudposse/atmos/pkg/config"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// ExecuteHelmfileCmd executes helmfile commands
func ExecuteHelmfileCmd(cmd *cobra.Command, args []string) error {
	info, err := processCommandLineArgs("helmfile", cmd, args)
	if err != nil {
		return err
	}

	cliConfig, err := cfg.InitCliConfig(info, true)
	if err != nil {
		return err
	}

	if info.NeedHelp {
		return nil
	}

	info, err = ProcessStacks(cliConfig, info, true)
	if err != nil {
		return err
	}

	if len(info.Stack) < 1 {
		return errors.New("stack must be specified")
	}

	err = checkHelmfileConfig(cliConfig)
	if err != nil {
		return err
	}

	// Check if the component exists as a helmfile component
	componentPath := path.Join(cliConfig.HelmfileDirAbsolutePath, info.ComponentFolderPrefix, info.FinalComponent)
	componentPathExists, err := u.IsDirectory(componentPath)
	if err != nil || !componentPathExists {
		return fmt.Errorf("'%s' points to the Helmfile component '%s', but it does not exist in '%s'",
			info.ComponentFromArg,
			info.FinalComponent,
			path.Join(cliConfig.Components.Helmfile.BasePath, info.ComponentFolderPrefix),
		)
	}

	// Check if the component is allowed to be provisioned (`metadata.type` attribute)
	if (info.SubCommand == "sync" || info.SubCommand == "apply" || info.SubCommand == "deploy") && info.ComponentIsAbstract {
		return fmt.Errorf("abstract component '%s' cannot be provisioned since it's explicitly prohibited from being deployed "+
			"by 'metadata.type: abstract' attribute", path.Join(info.ComponentFolderPrefix, info.Component))
	}

	// Print component variables
	u.PrintInfo(fmt.Sprintf("\nVariables for the component '%s' in the stack '%s':\n", info.ComponentFromArg, info.Stack))
	err = u.PrintAsYAML(info.ComponentVarsSection)
	if err != nil {
		return err
	}

	// Check if component 'settings.validation' section is specified and validate the component
	valid, err := ValidateComponent(cliConfig, info.ComponentFromArg, info.ComponentSection, "", "")
	if err != nil {
		return err
	}
	if !valid {
		return fmt.Errorf("\nComponent '%s' did not pass the validation policies.\n", info.ComponentFromArg)
	}

	// Write variables to a file
	varFile := constructHelmfileComponentVarfileName(info)
	varFilePath := constructHelmfileComponentVarfilePath(cliConfig, info)

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

	context := cfg.GetContextFromVars(info.ComponentVarsSection)

	envVarsEKS := []string{}

	if cliConfig.Components.Helmfile.UseEKS {
		// Prepare AWS profile
		helmAwsProfile := cfg.ReplaceContextTokens(context, cliConfig.Components.Helmfile.HelmAwsProfilePattern)
		u.PrintInfo(fmt.Sprintf("\nUsing AWS_PROFILE=%s\n\n", helmAwsProfile))

		// Download kubeconfig by running `aws eks update-kubeconfig`
		kubeconfigPath := fmt.Sprintf("%s/%s-kubecfg", cliConfig.Components.Helmfile.KubeconfigPath, info.ContextPrefix)
		clusterName := cfg.ReplaceContextTokens(context, cliConfig.Components.Helmfile.ClusterNamePattern)
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
			true,
			info.RedirectStdErr,
		)
		if err != nil {
			return err
		}

		envVarsEKS = append(envVarsEKS, []string{
			fmt.Sprintf("AWS_PROFILE=%s", helmAwsProfile),
			fmt.Sprintf("KUBECONFIG=%s", kubeconfigPath),
		}...)
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
	fmt.Printf("Global options: %v\n", info.GlobalOptions)

	fmt.Printf("Arguments and flags: %v\n", info.AdditionalArgsAndFlags)
	fmt.Println("Component: " + info.ComponentFromArg)

	if len(info.BaseComponent) > 0 {
		fmt.Println("Helmfile component: " + info.BaseComponent)
	}

	if info.Stack == info.StackFromArg {
		fmt.Println("Stack: " + info.StackFromArg)
	} else {
		fmt.Println("Stack: " + info.StackFromArg)
		fmt.Println("Stack path: " + path.Join(cliConfig.BasePath, cliConfig.Stacks.BasePath, info.Stack))
	}

	workingDir := constructHelmfileComponentWorkingDir(cliConfig, info)
	fmt.Printf("Working dir: %s\n\n", workingDir)

	// Prepare arguments and flags
	allArgsAndFlags := []string{"--state-values-file", varFile}
	if info.GlobalOptions != nil && len(info.GlobalOptions) > 0 {
		allArgsAndFlags = append(allArgsAndFlags, info.GlobalOptions...)
	}
	allArgsAndFlags = append(allArgsAndFlags, info.SubCommand)
	allArgsAndFlags = append(allArgsAndFlags, info.AdditionalArgsAndFlags...)

	// Prepare ENV vars
	envVars := append(info.ComponentEnvList, []string{
		fmt.Sprintf("STACK=%s", info.Stack),
	}...)

	// Append the context ENV vars (first check if they are not set by the caller)
	env := os.Getenv("NAMESPACE")
	if env == "" {
		envVars = append(envVars, fmt.Sprintf("NAMESPACE=%s", context.Namespace))
	}
	env = os.Getenv("TENANT")
	if env == "" {
		envVars = append(envVars, fmt.Sprintf("TENANT=%s", context.Tenant))
	}
	env = os.Getenv("ENVIRONMENT")
	if env == "" {
		envVars = append(envVars, fmt.Sprintf("ENVIRONMENT=%s", context.Environment))
	}
	env = os.Getenv("STAGE")
	if env == "" {
		envVars = append(envVars, fmt.Sprintf("STAGE=%s", context.Stage))
	}
	env = os.Getenv("REGION")
	if env == "" {
		envVars = append(envVars, fmt.Sprintf("REGION=%s", context.Region))
	}

	if cliConfig.Components.Helmfile.UseEKS {
		envVars = append(envVars, envVarsEKS...)
	}

	u.PrintInfo("Using ENV vars:")
	for _, v := range envVars {
		fmt.Println(v)
	}

	err = ExecuteShellCommand(
		info.Command,
		allArgsAndFlags,
		componentPath,
		envVars,
		info.DryRun,
		true,
		info.RedirectStdErr,
	)
	if err != nil {
		return err
	}

	// Cleanup
	_ = os.Remove(varFilePath)

	return nil
}

func checkHelmfileConfig(cliConfig cfg.CliConfiguration) error {
	if len(cliConfig.Components.Helmfile.BasePath) < 1 {
		return errors.New("Base path to helmfile components must be provided in 'components.helmfile.base_path' config or " +
			"'ATMOS_COMPONENTS_HELMFILE_BASE_PATH' ENV variable")
	}

	if cliConfig.Components.Helmfile.UseEKS {
		if len(cliConfig.Components.Helmfile.KubeconfigPath) < 1 {
			return errors.New("Kubeconfig path must be provided in 'components.helmfile.kubeconfig_path' config or " +
				"'ATMOS_COMPONENTS_HELMFILE_KUBECONFIG_PATH' ENV variable")
		}

		if len(cliConfig.Components.Helmfile.HelmAwsProfilePattern) < 1 {
			return errors.New("Helm AWS profile pattern must be provided in 'components.helmfile.helm_aws_profile_pattern' config or " +
				"'ATMOS_COMPONENTS_HELMFILE_HELM_AWS_PROFILE_PATTERN' ENV variable")
		}

		if len(cliConfig.Components.Helmfile.ClusterNamePattern) < 1 {
			return errors.New("Cluster name pattern must be provided in 'components.helmfile.cluster_name_pattern' config or " +
				"'ATMOS_COMPONENTS_HELMFILE_CLUSTER_NAME_PATTERN' ENV variable")
		}
	}

	return nil
}
