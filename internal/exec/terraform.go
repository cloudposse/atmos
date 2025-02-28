package exec

import (
	"fmt"
	"io"
	"os"
	osexec "os/exec"
	"path/filepath"
	"strings"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"golang.org/x/sync/errgroup"

	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

const (
	autoApproveFlag           = "-auto-approve"
	outFlag                   = "-out"
	varFileFlag               = "-var-file"
	skipTerraformLockFileFlag = "--skip-lock-file"
	forceFlag                 = "--force"
)

// ErrHTTPBackendWorkspaces is returned when attempting to use workspace commands with an HTTP backend.
var ErrHTTPBackendWorkspaces = errors.New("workspaces are not supported for the HTTP backend")

// ExecuteTerraformCmd parses the provided arguments and flags and executes terraform commands
func ExecuteTerraformCmd(cmd *cobra.Command, args []string, additionalArgsAndFlags []string) error {
	info, err := ProcessCommandLineArgs("terraform", cmd, args, additionalArgsAndFlags)
	if err != nil {
		return err
	}
	return ExecuteTerraform(info)
}

// ExecuteTerraform executes terraform commands
func ExecuteTerraform(info schema.ConfigAndStacksInfo) error {
	atmosConfig, err := cfg.InitCliConfig(info, true)
	if err != nil {
		return err
	}

	if info.NeedHelp {
		return nil
	}

	if info.SubCommand == "version" {
		return ExecuteShellCommand(atmosConfig,
			"terraform",
			[]string{info.SubCommand},
			"",
			nil,
			false,
			info.RedirectStdErr)
	}

	// Skip stack processing when cleaning with --force flag to allow cleaning without requiring stack configuration
	shouldProcessStacks, shouldCheckStack := shouldProcessStacks(&info)

	if shouldProcessStacks {
		processTemplatesAndYamlFunctions := needProcessTemplatesAndYamlFunctions(info.SubCommand)
		info, err = ProcessStacks(atmosConfig, info, shouldCheckStack, processTemplatesAndYamlFunctions, processTemplatesAndYamlFunctions, nil)
		if err != nil {
			return err
		}

		if len(info.Stack) < 1 && shouldCheckStack {
			return errors.New("stack must be specified for the terraform command")
		}
	}

	if !info.ComponentIsEnabled && info.SubCommand != "clean" {
		u.LogInfo(fmt.Sprintf("component '%s' is not enabled and skipped", info.ComponentFromArg))
		return nil
	}

	err = checkTerraformConfig(atmosConfig)
	if err != nil {
		return err
	}

	// Check if the component (or base component) exists as Terraform component
	componentPath := filepath.Join(atmosConfig.TerraformDirAbsolutePath, info.ComponentFolderPrefix, info.FinalComponent)
	componentPathExists, err := u.IsDirectory(componentPath)
	if err != nil || !componentPathExists {
		return fmt.Errorf("'%s' points to the Terraform component '%s', but it does not exist in '%s'",
			info.ComponentFromArg,
			info.FinalComponent,
			filepath.Join(atmosConfig.Components.Terraform.BasePath, info.ComponentFolderPrefix),
		)
	}

	// Check if the component is allowed to be provisioned (`metadata.type` attribute is not set to `abstract`)
	if (info.SubCommand == "plan" || info.SubCommand == "apply" || info.SubCommand == "deploy" || info.SubCommand == "workspace") && info.ComponentIsAbstract {
		return fmt.Errorf("abstract component '%s' cannot be provisioned since it's explicitly prohibited from being deployed "+
			"by 'metadata.type: abstract' attribute", filepath.Join(info.ComponentFolderPrefix, info.Component))
	}

	// Check if the component is locked (`metadata.locked` is set to true)
	if info.ComponentIsLocked {
		// Allow read-only commands, block modification commands
		switch info.SubCommand {
		case "apply", "deploy", "destroy", "import", "state", "taint", "untaint":
			return fmt.Errorf("component '%s' is locked and cannot be modified (metadata.locked = true)",
				filepath.Join(info.ComponentFolderPrefix, info.Component))
		}
	}

	// Check if trying to use workspace commands with HTTP backend
	if info.SubCommand == "workspace" && info.ComponentBackendType == "http" {
		return ErrHTTPBackendWorkspaces
	}

	if info.SubCommand == "clean" {
		err := handleCleanSubCommand(info, componentPath, atmosConfig)
		if err != nil {
			u.LogDebug(fmt.Errorf("error cleaning the terraform component: %v", err).Error())
			return err
		}
		return nil
	}

	varFile := constructTerraformComponentVarfileName(info)
	planFile := constructTerraformComponentPlanfileName(info)

	// Print component variables and write to file
	// Don't process variables when executing `terraform workspace` commands
	if info.SubCommand != "workspace" {
		u.LogDebug(fmt.Sprintf("\nVariables for the component '%s' in the stack '%s':", info.ComponentFromArg, info.Stack))
		if atmosConfig.Logs.Level == u.LogLevelTrace || atmosConfig.Logs.Level == u.LogLevelDebug {
			err = u.PrintAsYAMLToFileDescriptor(atmosConfig, info.ComponentVarsSection)
			if err != nil {
				return err
			}
		}

		// Write variables to a file (only if we are not using the previously generated terraform plan)
		if !info.UseTerraformPlan {
			var varFilePath, varFileNameFromArg string

			// Handle `terraform varfile` and `terraform write varfile` legacy commands
			if info.SubCommand == "varfile" || (info.SubCommand == "write" && info.SubCommand2 == "varfile") {
				if len(info.AdditionalArgsAndFlags) == 2 {
					fileFlag := info.AdditionalArgsAndFlags[0]
					if fileFlag == "-f" || fileFlag == "--file" {
						varFileNameFromArg = info.AdditionalArgsAndFlags[1]
					}
				}
			}

			if len(varFileNameFromArg) > 0 {
				varFilePath = varFileNameFromArg
			} else {
				varFilePath = constructTerraformComponentVarfilePath(atmosConfig, info)
			}

			u.LogDebug("Writing the variables to file:")
			u.LogDebug(varFilePath)

			if !info.DryRun {
				err = u.WriteToFileAsJSON(varFilePath, info.ComponentVarsSection, 0o644)
				if err != nil {
					return err
				}
			}
		}

		/*
		   Variables provided on the command line
		   https://developer.hashicorp.com/terraform/language/values/variables#variables-on-the-command-line
		   Terraform processes variables in the following order of precedence (from highest to lowest):
		     - Explicit -var flags: these have the highest priority and will override any other variable values, including those in --var-file
		     - Variables in --var-file: values in a variable file specified with --var-file override default values set in the Terraform configuration
		     - Environment variables: variables set as environment variables using the TF_VAR_ prefix
		     - Default values in the configuration file: these have the lowest priority
		*/
		if cliVars, ok := info.ComponentSection[cfg.TerraformCliVarsSectionName].(map[string]any); ok && len(cliVars) > 0 {
			u.LogDebug("\nCLI variables (will override the variables defined in the stack manifests):")
			if atmosConfig.Logs.Level == u.LogLevelTrace || atmosConfig.Logs.Level == u.LogLevelDebug {
				err = u.PrintAsYAMLToFileDescriptor(atmosConfig, cliVars)
				if err != nil {
					return err
				}
			}
		}
	}

	// Handle `terraform varfile` and `terraform write varfile` legacy commands
	if info.SubCommand == "varfile" || (info.SubCommand == "write" && info.SubCommand2 == "varfile") {
		return nil
	}

	// Check if component 'settings.validation' section is specified and validate the component
	valid, err := ValidateComponent(
		atmosConfig,
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
		return fmt.Errorf("\nComponent '%s' did not pass the validation policies.\n", info.ComponentFromArg)
	}

	// Component working directory
	workingDir := constructTerraformComponentWorkingDir(atmosConfig, info)

	err = generateBackendConfig(&atmosConfig, &info, workingDir)
	if err != nil {
		return err
	}

	err = generateProviderOverrides(&atmosConfig, &info, workingDir)
	if err != nil {
		return err
	}

	// Check for any Terraform environment variables that might conflict with Atmos
	for _, envVar := range os.Environ() {
		if strings.HasPrefix(envVar, "TF_") {
			varName := strings.SplitN(envVar, "=", 2)[0]
			u.LogWarning(fmt.Sprintf("detected '%s' set in the environment; this may interfere with Atmos's control of Terraform.", varName))
		}
	}
	info.ComponentEnvList = append(info.ComponentEnvList, fmt.Sprintf("ATMOS_CLI_CONFIG_PATH=%s", atmosConfig.CliConfigPath))
	basePath, err := filepath.Abs(atmosConfig.BasePath)
	if err != nil {
		return err
	}
	info.ComponentEnvList = append(info.ComponentEnvList, fmt.Sprintf("ATMOS_BASE_PATH=%s", basePath))
	// Set `TF_IN_AUTOMATION` ENV var to `true` to suppress verbose instructions after terraform commands
	// https://developer.hashicorp.com/terraform/cli/config/environment-variables#tf_in_automation
	info.ComponentEnvList = append(info.ComponentEnvList, "TF_IN_AUTOMATION=true")

	// Set 'TF_APPEND_USER_AGENT' ENV var based on precedence
	// Precedence: Environment Variable > atmos.yaml > Default
	appendUserAgent := atmosConfig.Components.Terraform.AppendUserAgent
	if envUA, exists := os.LookupEnv("TF_APPEND_USER_AGENT"); exists && envUA != "" {
		appendUserAgent = envUA
	}
	if appendUserAgent != "" {
		info.ComponentEnvList = append(info.ComponentEnvList, fmt.Sprintf("TF_APPEND_USER_AGENT=%s", appendUserAgent))
	}

	// Print ENV vars if they are found in the component's stack config
	if len(info.ComponentEnvList) > 0 {
		u.LogDebug("\nUsing ENV vars:")
		for _, v := range info.ComponentEnvList {
			u.LogDebug(v)
		}
	}

	// Run `terraform init` before running other commands
	runTerraformInit := true
	if info.SubCommand == "init" ||
		info.SubCommand == "clean" ||
		(info.SubCommand == "deploy" && !atmosConfig.Components.Terraform.DeployRunInit) {
		runTerraformInit = false
	}

	if info.SkipInit {
		u.LogDebug("Skipping over 'terraform init' due to '--skip-init' flag being passed")
		runTerraformInit = false
	}

	if runTerraformInit {
		initCommandWithArguments := []string{"init"}
		if info.SubCommand == "workspace" || atmosConfig.Components.Terraform.InitRunReconfigure {
			initCommandWithArguments = []string{"init", "-reconfigure"}
		}

		// Before executing `terraform init`, delete the `.terraform/environment` file from the component directory
		cleanTerraformWorkspace(atmosConfig, componentPath)

		err = ExecuteShellCommand(
			atmosConfig,
			info.Command,
			initCommandWithArguments,
			componentPath,
			info.ComponentEnvList,
			info.DryRun,
			info.RedirectStdErr,
		)
		if err != nil {
			return err
		}
	}

	// Handle `terraform deploy` custom command
	if info.SubCommand == "deploy" {
		info.SubCommand = "apply"
		if !info.UseTerraformPlan && !u.SliceContainsString(info.AdditionalArgsAndFlags, autoApproveFlag) {
			info.AdditionalArgsAndFlags = append(info.AdditionalArgsAndFlags, autoApproveFlag)
		}
	}

	// Handle atmosConfig.Components.Terraform.ApplyAutoApprove flag
	if info.SubCommand == "apply" && atmosConfig.Components.Terraform.ApplyAutoApprove && !info.UseTerraformPlan {
		if !u.SliceContainsString(info.AdditionalArgsAndFlags, autoApproveFlag) {
			info.AdditionalArgsAndFlags = append(info.AdditionalArgsAndFlags, autoApproveFlag)
		}
	}

	// Print command info
	u.LogDebug("\nCommand info:")
	u.LogDebug("Terraform binary: " + info.Command)

	if info.SubCommand2 == "" {
		u.LogDebug(fmt.Sprintf("Terraform command: %s", info.SubCommand))
	} else {
		u.LogDebug(fmt.Sprintf("Terraform command: %s %s", info.SubCommand, info.SubCommand2))
	}

	u.LogDebug(fmt.Sprintf("Arguments and flags: %v", info.AdditionalArgsAndFlags))
	u.LogDebug("Component: " + info.ComponentFromArg)

	if len(info.BaseComponentPath) > 0 {
		u.LogDebug("Terraform component: " + info.BaseComponentPath)
	}

	if len(info.ComponentInheritanceChain) > 0 {
		u.LogDebug("Inheritance: " + info.ComponentFromArg + " -> " + strings.Join(info.ComponentInheritanceChain, " -> "))
	}

	if info.Stack == info.StackFromArg {
		u.LogDebug("Stack: " + info.StackFromArg)
	} else {
		u.LogDebug("Stack: " + info.StackFromArg)
		u.LogDebug("Stack path: " + filepath.Join(atmosConfig.BasePath, atmosConfig.Stacks.BasePath, info.Stack))
	}

	u.LogDebug(fmt.Sprintf("Working dir: %s", workingDir))

	allArgsAndFlags := strings.Fields(info.SubCommand)

	switch info.SubCommand {
	case "plan":
		// Add varfile
		allArgsAndFlags = append(allArgsAndFlags, []string{varFileFlag, varFile}...)
		// Add planfile
		if !u.SliceContainsString(info.AdditionalArgsAndFlags, outFlag) &&
			!u.SliceContainsStringHasPrefix(info.AdditionalArgsAndFlags, outFlag+"=") {
			allArgsAndFlags = append(allArgsAndFlags, []string{outFlag, planFile}...)
		}
	case "destroy":
		allArgsAndFlags = append(allArgsAndFlags, []string{varFileFlag, varFile}...)
	case "import":
		allArgsAndFlags = append(allArgsAndFlags, []string{varFileFlag, varFile}...)
	case "refresh":
		allArgsAndFlags = append(allArgsAndFlags, []string{varFileFlag, varFile}...)
	case "apply":
		if !info.UseTerraformPlan {
			allArgsAndFlags = append(allArgsAndFlags, []string{varFileFlag, varFile}...)
		}
	case "init":
		// Before executing `terraform init`, delete the `.terraform/environment` file from the component directory
		cleanTerraformWorkspace(atmosConfig, componentPath)

		if atmosConfig.Components.Terraform.InitRunReconfigure {
			allArgsAndFlags = append(allArgsAndFlags, []string{"-reconfigure"}...)
		}
	case "workspace":
		if info.SubCommand2 == "list" || info.SubCommand2 == "show" {
			allArgsAndFlags = append(allArgsAndFlags, []string{info.SubCommand2}...)
		} else if info.SubCommand2 != "" {
			allArgsAndFlags = append(allArgsAndFlags, []string{info.SubCommand2, info.TerraformWorkspace}...)
		}
	}

	allArgsAndFlags = append(allArgsAndFlags, info.AdditionalArgsAndFlags...)

	// Add any args we're generating -- terraform is picky about ordering flags
	// and args, so these args need to go after any flags, including those
	// specified in AdditionalArgsAndFlags.
	if info.SubCommand == "apply" && info.UseTerraformPlan {
		if info.PlanFile != "" {
			// If the planfile name was passed on the command line, use it
			allArgsAndFlags = append(allArgsAndFlags, []string{info.PlanFile}...)
		} else {
			// Otherwise, use the planfile name what is autogenerated by Atmos
			allArgsAndFlags = append(allArgsAndFlags, []string{planFile}...)
		}
	}

	// Run `terraform workspace` before executing other terraform commands
	// only if the `TF_WORKSPACE` environment variable is not set by the caller
	if info.SubCommand != "init" && !(info.SubCommand == "workspace" && info.SubCommand2 != "") {
		tfWorkspaceEnvVar := os.Getenv("TF_WORKSPACE")

		if tfWorkspaceEnvVar == "" {
			workspaceSelectRedirectStdErr := "/dev/stdout"

			// If `--redirect-stderr` flag is not passed, always redirect `stderr` to `stdout` for `terraform workspace select` command
			if info.RedirectStdErr != "" {
				workspaceSelectRedirectStdErr = info.RedirectStdErr
			}

			err = ExecuteShellCommand(
				atmosConfig,
				info.Command,
				[]string{"workspace", "select", info.TerraformWorkspace},
				componentPath,
				info.ComponentEnvList,
				info.DryRun,
				workspaceSelectRedirectStdErr,
			)
			if err != nil {
				var osErr *osexec.ExitError
				ok := errors.As(err, &osErr)
				if !ok || osErr.ExitCode() != 1 {
					// err is not a non-zero exit code or err is not exit code 1, which we are expecting
					return err
				}
				err = ExecuteShellCommand(
					atmosConfig,
					info.Command,
					[]string{"workspace", "new", info.TerraformWorkspace},
					componentPath,
					info.ComponentEnvList,
					info.DryRun,
					info.RedirectStdErr,
				)
				if err != nil {
					return err
				}
			}
		}
	}

	// Check if the terraform command requires a user interaction,
	// but it's running in a scripted environment (where a `tty` is not attached or `stdin` is not attached)
	if os.Stdin == nil && !u.SliceContainsString(info.AdditionalArgsAndFlags, autoApproveFlag) {
		errorMessage := ""

		if info.SubCommand == "apply" {
			errorMessage = "'terraform apply' requires a user interaction, but it's running without `tty` or `stdin` attached." +
				"\nUse 'terraform apply -auto-approve' or 'terraform deploy' instead."
		}

		if errorMessage != "" {
			return errors.New(errorMessage)
		}
	}

	// Check `region` for `terraform import`
	if info.SubCommand == "import" {
		if region, regionExist := info.ComponentVarsSection["region"].(string); regionExist {
			info.ComponentEnvList = append(info.ComponentEnvList, fmt.Sprintf("AWS_REGION=%s", region))
		}
	}

	// Execute `terraform shell` command
	if info.SubCommand == "shell" {
		err = execTerraformShellCommand(
			atmosConfig,
			info.ComponentFromArg,
			info.Stack,
			info.ComponentEnvList,
			varFile,
			workingDir,
			info.TerraformWorkspace,
			componentPath,
		)
		if err != nil {
			return err
		}
		return nil
	}

	// Execute the provided command (except for `terraform workspace` which was executed above)
	if !(info.SubCommand == "workspace" && info.SubCommand2 == "") {
		if info.SubCommand == "plan" {
			allArgsAndFlags = append(allArgsAndFlags, "-json")
			pipeformStdin, planStdout := io.Pipe()
			// Get a plan file
			p := ""
			for i, a := range allArgsAndFlags {
				if a == "-out" || a == "--out" {
					p = allArgsAndFlags[i+1]
					break
				}
			}
			if p == "" {
				p = planFile
				allArgsAndFlags = append(allArgsAndFlags, "-out", p)
			}

			// terraform plan
			waitPlan, err := ExecuteShellCommandWithPipe(
				atmosConfig,
				info.Command,
				allArgsAndFlags,
				componentPath,
				info.ComponentEnvList,
				info.DryRun,
				info.RedirectStdErr,
				nil,
				planStdout,
			)
			if err != nil {
				return err
			}
			// pipeform
			waitPipeform, err := ExecuteShellCommandWithPipe(
				atmosConfig,
				"pipeform",
				nil,
				componentPath,
				info.ComponentEnvList,
				info.DryRun,
				info.RedirectStdErr,
				pipeformStdin,
				nil,
			)
			if err != nil {
				return err
			}
			g := &errgroup.Group{}
			g.Go(func() error {
				err := waitPlan()
				planStdout.Close()
				return err
			})
			g.Go(waitPipeform)
			if err := g.Wait(); err != nil {
				return err
			}
			// terraform show
			if err := ExecuteShellCommand(
				atmosConfig,
				info.Command,
				[]string{"show", p},
				componentPath,
				info.ComponentEnvList,
				info.DryRun,
				info.RedirectStdErr,
			); err != nil {
				return err
			}
		} else {
			err = ExecuteShellCommand(
				atmosConfig,
				info.Command,
				allArgsAndFlags,
				componentPath,
				info.ComponentEnvList,
				info.DryRun,
				info.RedirectStdErr,
			)
			if err != nil {
				return err
			}
		}
	}

	// Clean up
	if info.SubCommand != "plan" && info.SubCommand != "show" && info.PlanFile == "" {
		planFilePath := constructTerraformComponentPlanfilePath(atmosConfig, info)
		_ = os.Remove(planFilePath)
	}

	if info.SubCommand == "apply" {
		varFilePath := constructTerraformComponentVarfilePath(atmosConfig, info)
		_ = os.Remove(varFilePath)
	}

	return nil
}
