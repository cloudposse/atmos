// https://github.com/roboll/helmfile#cli-reference

package exec

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"

	tuiUtils "github.com/cloudposse/atmos/internal/tui/utils"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// ExecuteHelmfileCmd parses the provided arguments and flags and executes helmfile commands
func ExecuteHelmfileCmd(cmd *cobra.Command, args []string, additionalArgsAndFlags []string) error {
	info, err := ProcessCommandLineArgs("helmfile", cmd, args, additionalArgsAndFlags)
	if err != nil {
		return err
	}

	return ExecuteHelmfile(info)
}

// ExecuteHelmfile executes helmfile commands
func ExecuteHelmfile(info schema.ConfigAndStacksInfo) error {
	atmosConfig, err := cfg.InitCliConfig(info, true)
	if err != nil {
		return err
	}

	if info.NeedHelp {
		return nil
	}

	// If the user just types `atmos helmfile`, print Atmos logo and show helmfile help
	if info.SubCommand == "" {
		fmt.Println()
		err = tuiUtils.PrintStyledText("ATMOS")
		if err != nil {
			return err
		}

		err = processHelp(atmosConfig, "helmfile", "")
		if err != nil {
			return err
		}

		fmt.Println()
		return nil
	}

	if info.SubCommand == "version" {
		return ExecuteShellCommand(atmosConfig, "helmfile", []string{info.SubCommand}, "", nil, false, info.RedirectStdErr)
	}

	info, err = ProcessStacks(atmosConfig, info, true, true)
	if err != nil {
		return err
	}

	if len(info.Stack) < 1 {
		return errors.New("stack must be specified")
	}

	if !info.ComponentIsEnabled {
		u.LogInfo(atmosConfig, fmt.Sprintf("component '%s' is not enabled and skipped", info.ComponentFromArg))
		return nil
	}

	err = checkHelmfileConfig(atmosConfig)
	if err != nil {
		return err
	}

	// Check if the component exists as a helmfile component
	componentPath := filepath.Join(atmosConfig.HelmfileDirAbsolutePath, info.ComponentFolderPrefix, info.FinalComponent)
	componentPathExists, err := u.IsDirectory(componentPath)
	if err != nil || !componentPathExists {
		return fmt.Errorf("'%s' points to the Helmfile component '%s', but it does not exist in '%s'",
			info.ComponentFromArg,
			info.FinalComponent,
			filepath.Join(atmosConfig.Components.Helmfile.BasePath, info.ComponentFolderPrefix),
		)
	}

	// Check if the component is allowed to be provisioned (`metadata.type` attribute)
	if (info.SubCommand == "sync" || info.SubCommand == "apply" || info.SubCommand == "deploy") && info.ComponentIsAbstract {
		return fmt.Errorf("abstract component '%s' cannot be provisioned since it's explicitly prohibited from being deployed "+
			"by 'metadata.type: abstract' attribute", filepath.Join(info.ComponentFolderPrefix, info.Component))
	}

	// Check if the component is locked (`metadata.locked` is set to true)
	if info.ComponentIsLocked {
		// Allow read-only commands, block modification commands
		switch info.SubCommand {
		case "sync", "apply", "deploy", "delete", "destroy", "secrets":
			return fmt.Errorf("component '%s' is locked and cannot be modified (metadata.locked = true)",
				filepath.Join(info.ComponentFolderPrefix, info.Component))
		}
	}

	// Print component variables
	u.LogDebug(atmosConfig, fmt.Sprintf("\nVariables for the component '%s' in the stack '%s':", info.ComponentFromArg, info.Stack))

	if atmosConfig.Logs.Level == u.LogLevelTrace || atmosConfig.Logs.Level == u.LogLevelDebug {
		err = u.PrintAsYAMLToFileDescriptor(atmosConfig, info.ComponentVarsSection)
		if err != nil {
			return err
		}
	}

	// Check if component 'settings.validation' section is specified and validate the component
	valid, err := ValidateComponent(atmosConfig, info.ComponentFromArg, info.ComponentSection, "", "", nil, 0)
	if err != nil {
		return err
	}
	if !valid {
		return fmt.Errorf("\nComponent '%s' did not pass the validation policies.\n", info.ComponentFromArg)
	}

	// Write variables to a file
	varFile := constructHelmfileComponentVarfileName(info)
	varFilePath := constructHelmfileComponentVarfilePath(atmosConfig, info)

	u.LogDebug(atmosConfig, "Writing the variables to file:")
	u.LogDebug(atmosConfig, varFilePath)

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

	if atmosConfig.Components.Helmfile.UseEKS {
		// Prepare AWS profile
		helmAwsProfile := cfg.ReplaceContextTokens(context, atmosConfig.Components.Helmfile.HelmAwsProfilePattern)
		u.LogDebug(atmosConfig, fmt.Sprintf("\nUsing AWS_PROFILE=%s\n\n", helmAwsProfile))

		// Download kubeconfig by running `aws eks update-kubeconfig`
		kubeconfigPath := fmt.Sprintf("%s/%s-kubecfg", atmosConfig.Components.Helmfile.KubeconfigPath, info.ContextPrefix)
		clusterName := cfg.ReplaceContextTokens(context, atmosConfig.Components.Helmfile.ClusterNamePattern)
		u.LogDebug(atmosConfig, fmt.Sprintf("Downloading kubeconfig from the cluster '%s' and saving it to %s\n\n", clusterName, kubeconfigPath))

		err = ExecuteShellCommand(
			atmosConfig,
			"aws",
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
	u.LogDebug(atmosConfig, "\nCommand info:")
	u.LogDebug(atmosConfig, "Helmfile binary: "+info.Command)
	u.LogDebug(atmosConfig, "Helmfile command: "+info.SubCommand)

	// https://github.com/roboll/helmfile#cli-reference
	// atmos helmfile diff echo-server -s tenant1-ue2-dev --global-options "--no-color --namespace=test"
	// atmos helmfile diff echo-server -s tenant1-ue2-dev --global-options "--no-color --namespace test"
	// atmos helmfile diff echo-server -s tenant1-ue2-dev --global-options="--no-color --namespace=test"
	// atmos helmfile diff echo-server -s tenant1-ue2-dev --global-options="--no-color --namespace test"
	u.LogDebug(atmosConfig, fmt.Sprintf("Global options: %v", info.GlobalOptions))

	u.LogDebug(atmosConfig, fmt.Sprintf("Arguments and flags: %v", info.AdditionalArgsAndFlags))
	u.LogDebug(atmosConfig, "Component: "+info.ComponentFromArg)

	if len(info.BaseComponent) > 0 {
		u.LogDebug(atmosConfig, "Helmfile component: "+info.BaseComponent)
	}

	if info.Stack == info.StackFromArg {
		u.LogDebug(atmosConfig, "Stack: "+info.StackFromArg)
	} else {
		u.LogDebug(atmosConfig, "Stack: "+info.StackFromArg)
		u.LogDebug(atmosConfig, "Stack path: "+filepath.Join(atmosConfig.BasePath, atmosConfig.Stacks.BasePath, info.Stack))
	}

	workingDir := constructHelmfileComponentWorkingDir(atmosConfig, info)
	u.LogDebug(atmosConfig, fmt.Sprintf("Working dir: %s\n\n", workingDir))

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

	if atmosConfig.Components.Helmfile.KubeconfigPath != "" {
		envVars = append(envVars, fmt.Sprintf("KUBECONFIG=%s", atmosConfig.Components.Helmfile.KubeconfigPath))
	}

	if atmosConfig.Components.Helmfile.UseEKS {
		envVars = append(envVars, envVarsEKS...)
	}

	u.LogTrace(atmosConfig, "Using ENV vars:")
	for _, v := range envVars {
		u.LogTrace(atmosConfig, v)
	}

	err = ExecuteShellCommand(
		atmosConfig,
		info.Command,
		allArgsAndFlags,
		componentPath,
		envVars,
		info.DryRun,
		info.RedirectStdErr,
	)
	if err != nil {
		return err
	}

	// Cleanup
	err = os.Remove(varFilePath)
	if err != nil {
		u.LogWarning(atmosConfig, err.Error())
	}

	return nil
}

func checkHelmfileConfig(atmosConfig schema.AtmosConfiguration) error {
	if len(atmosConfig.Components.Helmfile.BasePath) < 1 {
		return errors.New("Base path to helmfile components must be provided in 'components.helmfile.base_path' config or " +
			"'ATMOS_COMPONENTS_HELMFILE_BASE_PATH' ENV variable")
	}

	if atmosConfig.Components.Helmfile.UseEKS {
		if len(atmosConfig.Components.Helmfile.KubeconfigPath) < 1 {
			return errors.New("Kubeconfig path must be provided in 'components.helmfile.kubeconfig_path' config or " +
				"'ATMOS_COMPONENTS_HELMFILE_KUBECONFIG_PATH' ENV variable")
		}

		if len(atmosConfig.Components.Helmfile.HelmAwsProfilePattern) < 1 {
			return errors.New("Helm AWS profile pattern must be provided in 'components.helmfile.helm_aws_profile_pattern' config or " +
				"'ATMOS_COMPONENTS_HELMFILE_HELM_AWS_PROFILE_PATTERN' ENV variable")
		}

		if len(atmosConfig.Components.Helmfile.ClusterNamePattern) < 1 {
			return errors.New("Cluster name pattern must be provided in 'components.helmfile.cluster_name_pattern' config or " +
				"'ATMOS_COMPONENTS_HELMFILE_CLUSTER_NAME_PATTERN' ENV variable")
		}
	}

	return nil
}
