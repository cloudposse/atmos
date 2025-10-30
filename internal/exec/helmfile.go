// https://github.com/roboll/helmfile#cli-reference

package exec

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	errUtils "github.com/cloudposse/atmos/errors"
	cfg "github.com/cloudposse/atmos/pkg/config"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// ExecuteHelmfileCmd parses the provided arguments and flags and executes helmfile commands.
func ExecuteHelmfileCmd(cmd *cobra.Command, args []string, additionalArgsAndFlags []string) error {
	info, err := ProcessCommandLineArgs("helmfile", cmd, args, additionalArgsAndFlags)
	if err != nil {
		return err
	}

	return ExecuteHelmfile(info)
}

// ExecuteHelmfile executes helmfile commands.
func ExecuteHelmfile(info schema.ConfigAndStacksInfo) error {
	defer perf.Track(nil, "exec.ExecuteHelmfile")()

	atmosConfig, err := cfg.InitCliConfig(info, true)
	if err != nil {
		return err
	}

	// Add the `command` from `components.helmfile.command` from `atmos.yaml`.
	if info.Command == "" {
		if atmosConfig.Components.Helmfile.Command != "" {
			info.Command = atmosConfig.Components.Helmfile.Command
		} else {
			info.Command = cfg.HelmfileComponentType
		}
	}

	if info.SubCommand == "version" {
		return ExecuteShellCommand(
			atmosConfig,
			info.Command,
			[]string{info.SubCommand},
			"",
			nil,
			false,
			info.RedirectStdErr,
		)
	}

	info, err = ProcessStacks(&atmosConfig, info, true, true, true, nil)
	if err != nil {
		return err
	}

	if !info.ComponentIsEnabled {
		log.Info("Component is not enabled and skipped", "component", info.ComponentFromArg)
		return nil
	}

	err = checkHelmfileConfig(&atmosConfig)
	if err != nil {
		return err
	}

	// Check if the component exists as a helmfile component.
	componentPath, err := u.GetComponentPath(&atmosConfig, "helmfile", info.ComponentFolderPrefix, info.FinalComponent)
	if err != nil {
		return fmt.Errorf("failed to resolve component path: %w", err)
	}

	componentPathExists, err := u.IsDirectory(componentPath)
	if err != nil || !componentPathExists {
		// Get the base path for the error message, respecting the user's actual config.
		basePath, _ := u.GetComponentBasePath(&atmosConfig, "helmfile")
		return fmt.Errorf("'%s' points to the Helmfile component '%s', but it does not exist in '%s'",
			info.ComponentFromArg,
			info.FinalComponent,
			basePath,
		)
	}

	// Check if the component is allowed to be provisioned (`metadata.type` attribute).
	if (info.SubCommand == "sync" || info.SubCommand == "apply" || info.SubCommand == "deploy") && info.ComponentIsAbstract {
		return fmt.Errorf("abstract component '%s' cannot be provisioned since it's explicitly prohibited from being deployed "+
			"by 'metadata.type: abstract' attribute", filepath.Join(info.ComponentFolderPrefix, info.Component))
	}

	// Check if the component is locked (`metadata.locked` is set to true).
	if info.ComponentIsLocked {
		// Allow read-only commands, block modification commands.
		switch info.SubCommand {
		case "sync", "apply", "deploy", "delete", "destroy":
			return fmt.Errorf("component `%s` is locked and cannot be modified (metadata.locked = true)",
				filepath.Join(info.ComponentFolderPrefix, info.Component))
		}
	}

	// Print component variables.
	log.Debug("Variables for component in stack", "component", info.ComponentFromArg, "stack", info.Stack, "variables", info.ComponentVarsSection)

	// Check if the component 'settings.validation' section is specified and validate the component.
	valid, err := ValidateComponent(
		&atmosConfig,
		info.ComponentFromArg,
		info.ComponentSection,
		"",
		"",
		nil,
		0,
	)
	if err != nil {
		return err
	}
	if !valid {
		return fmt.Errorf("%w: the component '%s' did not pass the validation policies",
			errUtils.ErrInvalidComponent,
			info.ComponentFromArg,
		)
	}

	// Write variables to a file.
	varFile := constructHelmfileComponentVarfileName(&info)
	varFilePath := constructHelmfileComponentVarfilePath(&atmosConfig, &info)

	log.Debug("Writing the variables to file:", "file", varFilePath)

	if !info.DryRun {
		err = u.WriteToFileAsYAML(varFilePath, info.ComponentVarsSection, 0o644)
		if err != nil {
			return err
		}
	}

	// Handle `helmfile deploy` custom command.
	if info.SubCommand == "deploy" {
		info.SubCommand = "sync"
	}

	context := cfg.GetContextFromVars(info.ComponentVarsSection)

	envVarsEKS := []string{}

	if atmosConfig.Components.Helmfile.UseEKS {
		// Prepare AWS profile.
		helmAwsProfile := cfg.ReplaceContextTokens(context, atmosConfig.Components.Helmfile.HelmAwsProfilePattern)
		log.Debug("Using AWS_PROFILE", "profile", helmAwsProfile)

		// Download kubeconfig by running `aws eks update-kubeconfig`.
		kubeconfigPath := fmt.Sprintf("%s/%s-kubecfg", atmosConfig.Components.Helmfile.KubeconfigPath, info.ContextPrefix)
		clusterName := cfg.ReplaceContextTokens(context, atmosConfig.Components.Helmfile.ClusterNamePattern)
		log.Debug("Downloading and saving kubeconfig", "cluster", clusterName, "path", kubeconfigPath)

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

	// Print command info.
	log.Debug("Command info:")
	log.Debug("Helmfile binary: " + info.Command)
	log.Debug("Helmfile command: " + info.SubCommand)

	// https://github.com/roboll/helmfile#cli-reference
	// atmos helmfile diff echo-server -s tenant1-ue2-dev --global-options "--no-color --namespace=test"
	// atmos helmfile diff echo-server -s tenant1-ue2-dev --global-options "--no-color --namespace test"
	// atmos helmfile diff echo-server -s tenant1-ue2-dev --global-options="--no-color --namespace=test"
	// atmos helmfile diff echo-server -s tenant1-ue2-dev --global-options="--no-color --namespace test"
	log.Debug("Global", "options", info.GlobalOptions)

	log.Debug("Arguments and flags", "additional", info.AdditionalArgsAndFlags)
	log.Debug("Component: " + info.ComponentFromArg)

	if len(info.BaseComponent) > 0 {
		log.Debug("Helmfile component: " + info.BaseComponent)
	}

	if info.Stack == info.StackFromArg {
		log.Debug("Stack: " + info.StackFromArg)
	} else {
		log.Debug("Stack: " + info.StackFromArg)
		log.Debug("Stack path: " + filepath.Join(atmosConfig.BasePath, atmosConfig.Stacks.BasePath, info.Stack))
	}

	workingDir := constructHelmfileComponentWorkingDir(&atmosConfig, &info)
	log.Debug("Using", "working dir", workingDir)

	// Prepare arguments and flags.
	allArgsAndFlags := []string{"--state-values-file", varFile}
	if info.GlobalOptions != nil && len(info.GlobalOptions) > 0 {
		allArgsAndFlags = append(allArgsAndFlags, info.GlobalOptions...)
	}
	allArgsAndFlags = append(allArgsAndFlags, info.SubCommand)
	allArgsAndFlags = append(allArgsAndFlags, info.AdditionalArgsAndFlags...)

	// Convert ComponentEnvSection to ComponentEnvList.
	// ComponentEnvSection is populated by auth hooks and stack config env sections.
	for k, v := range info.ComponentEnvSection {
		info.ComponentEnvList = append(info.ComponentEnvList, fmt.Sprintf("%s=%v", k, v))
	}

	// Prepare ENV vars.
	envVars := append(info.ComponentEnvList, []string{
		fmt.Sprintf("STACK=%s", info.Stack),
	}...)

	// Append the context ENV vars (first check if they are not set by the caller).
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
	envVars = append(envVars, fmt.Sprintf("ATMOS_CLI_CONFIG_PATH=%s", atmosConfig.CliConfigPath))
	basePath, err := filepath.Abs(atmosConfig.BasePath)
	if err != nil {
		return err
	}
	envVars = append(envVars, fmt.Sprintf("ATMOS_BASE_PATH=%s", basePath))
	log.Debug("Using ENV vars:")
	for _, v := range envVars {
		log.Debug(v)
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

	// Cleanup.
	err = os.Remove(varFilePath)
	if err != nil {
		log.Warn(err.Error())
	}

	return nil
}
