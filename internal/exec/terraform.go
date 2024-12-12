package exec

import (
	"fmt"
	"os"
	osexec "os/exec"
	"path/filepath"
	"strings"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"

	tuiUtils "github.com/cloudposse/atmos/internal/tui/utils"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

const (
	autoApproveFlag           = "-auto-approve"
	outFlag                   = "-out"
	varFileFlag               = "-var-file"
	skipTerraformLockFileFlag = "--skip-lock-file"
	everythingFlag            = "--everything"
	forceFlag                 = "--force"
)

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
	cliConfig, err := cfg.InitCliConfig(info, true)
	if err != nil {
		return err
	}

	if info.NeedHelp {
		return nil
	}

	// If the user just types `atmos terraform`, print Atmos logo and show terraform help
	if info.SubCommand == "" {
		fmt.Println()
		err = tuiUtils.PrintStyledText("ATMOS")
		if err != nil {
			return err
		}

		err = processHelp(cliConfig, "terraform", "")
		if err != nil {
			return err
		}

		fmt.Println()
		return nil
	}
	if info.SubCommand == "version" {
		return ExecuteShellCommand(cliConfig,
			"terraform",
			[]string{info.SubCommand},
			"",
			nil,
			false,
			info.RedirectStdErr)
	}

	shouldProcessStacks := true
	shouldCheckStack := true
	// Skip stack processing when cleaning with --everything or --force flags to allow
	// cleaning without requiring stack configuration
	if info.SubCommand == "clean" &&
		(u.SliceContainsString(info.AdditionalArgsAndFlags, everythingFlag) ||
			u.SliceContainsString(info.AdditionalArgsAndFlags, forceFlag)) {
		if info.ComponentFromArg == "" {
			shouldProcessStacks = false
		}

		shouldCheckStack = info.Stack != ""

	}

	if shouldProcessStacks {
		info, err = ProcessStacks(cliConfig, info, shouldCheckStack, true)
		if err != nil {
			return err
		}
		if len(info.Stack) < 1 && shouldCheckStack {
			return errors.New("stack must be specified when not using --everything or --force flags")
		}
	}

	if !info.ComponentIsEnabled {
		u.LogInfo(cliConfig, fmt.Sprintf("component '%s' is not enabled and skipped", info.ComponentFromArg))
		return nil
	}

	err = checkTerraformConfig(cliConfig)
	if err != nil {
		return err
	}
	// Check if the component (or base component) exists as Terraform component
	componentPath := filepath.Join(cliConfig.TerraformDirAbsolutePath, info.ComponentFolderPrefix, info.FinalComponent)
	componentPathExists, err := u.IsDirectory(componentPath)
	if err != nil || !componentPathExists {
		return fmt.Errorf("'%s' points to the Terraform component '%s', but it does not exist in '%s'",
			info.ComponentFromArg,
			info.FinalComponent,
			filepath.Join(cliConfig.Components.Terraform.BasePath, info.ComponentFolderPrefix),
		)
	}

	// Check if the component is allowed to be provisioned (`metadata.type` attribute is not set to `abstract`)
	if (info.SubCommand == "plan" || info.SubCommand == "apply" || info.SubCommand == "deploy" || info.SubCommand == "workspace") && info.ComponentIsAbstract {
		return fmt.Errorf("abstract component '%s' cannot be provisioned since it's explicitly prohibited from being deployed "+
			"by 'metadata.type: abstract' attribute", filepath.Join(info.ComponentFolderPrefix, info.Component))
	}

	if info.SubCommand == "clean" {
		err := handleCleanSubCommand(info, componentPath, cliConfig)
		if err != nil {
			u.LogTrace(cliConfig, fmt.Errorf("error cleaning the terraform component: %v", err).Error())
			return err
		}
		return nil
	}

	varFile := constructTerraformComponentVarfileName(info)
	planFile := constructTerraformComponentPlanfileName(info)

	// Print component variables and write to file
	// Don't process variables when executing `terraform workspace` commands
	if info.SubCommand != "workspace" {
		u.LogDebug(cliConfig, fmt.Sprintf("\nVariables for the component '%s' in the stack '%s':", info.ComponentFromArg, info.Stack))

		if cliConfig.Logs.Level == u.LogLevelTrace || cliConfig.Logs.Level == u.LogLevelDebug {
			err = u.PrintAsYAMLToFileDescriptor(cliConfig, info.ComponentVarsSection)
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
				varFilePath = constructTerraformComponentVarfilePath(cliConfig, info)
			}

			u.LogDebug(cliConfig, "Writing the variables to file:")
			u.LogDebug(cliConfig, varFilePath)

			if !info.DryRun {
				err = u.WriteToFileAsJSON(varFilePath, info.ComponentVarsSection, 0644)
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
	valid, err := ValidateComponent(cliConfig, info.ComponentFromArg, info.ComponentSection, "", "", nil, 0)
	if err != nil {
		return err
	}
	if !valid {
		return fmt.Errorf("\nComponent '%s' did not pass the validation policies.\n", info.ComponentFromArg)
	}

	// Component working directory
	workingDir := constructTerraformComponentWorkingDir(cliConfig, info)

	// Auto-generate backend file
	if cliConfig.Components.Terraform.AutoGenerateBackendFile {
		backendFileName := filepath.Join(workingDir, "backend.tf.json")

		u.LogDebug(cliConfig, "\nWriting the backend config to file:")
		u.LogDebug(cliConfig, backendFileName)

		if !info.DryRun {
			componentBackendConfig, err := generateComponentBackendConfig(info.ComponentBackendType, info.ComponentBackendSection, info.TerraformWorkspace)
			if err != nil {
				return err
			}

			err = u.WriteToFileAsJSON(backendFileName, componentBackendConfig, 0644)
			if err != nil {
				return err
			}
		}
	}

	// Generate `providers_override.tf.json` file if the `providers` section is configured
	if len(info.ComponentProvidersSection) > 0 {
		providerOverrideFileName := filepath.Join(workingDir, "providers_override.tf.json")

		u.LogDebug(cliConfig, "\nWriting the provider overrides to file:")
		u.LogDebug(cliConfig, providerOverrideFileName)

		if !info.DryRun {
			var providerOverrides = generateComponentProviderOverrides(info.ComponentProvidersSection)
			err = u.WriteToFileAsJSON(providerOverrideFileName, providerOverrides, 0644)
			if err != nil {
				return err
			}
		}
	}

	// Set `TF_IN_AUTOMATION` ENV var to `true` to suppress verbose instructions after terraform commands
	// https://developer.hashicorp.com/terraform/cli/config/environment-variables#tf_in_automation
	info.ComponentEnvList = append(info.ComponentEnvList, "TF_IN_AUTOMATION=true")

	// Set 'TF_APPEND_USER_AGENT' ENV var based on precedence
	// Precedence: Environment Variable > atmos.yaml > Default
	appendUserAgent := cliConfig.Components.Terraform.AppendUserAgent
	if envUA, exists := os.LookupEnv("TF_APPEND_USER_AGENT"); exists && envUA != "" {
		appendUserAgent = envUA
	}
	if appendUserAgent != "" {
		info.ComponentEnvList = append(info.ComponentEnvList, fmt.Sprintf("TF_APPEND_USER_AGENT=%s", appendUserAgent))
	}

	// Print ENV vars if they are found in the component's stack config
	if len(info.ComponentEnvList) > 0 {
		u.LogDebug(cliConfig, "\nUsing ENV vars:")
		for _, v := range info.ComponentEnvList {
			u.LogDebug(cliConfig, v)
		}
	}

	// Run `terraform init` before running other commands
	runTerraformInit := true
	if info.SubCommand == "init" ||
		info.SubCommand == "clean" ||
		(info.SubCommand == "deploy" && !cliConfig.Components.Terraform.DeployRunInit) {
		runTerraformInit = false
	}

	if info.SkipInit {
		u.LogDebug(cliConfig, "Skipping over 'terraform init' due to '--skip-init' flag being passed")
		runTerraformInit = false
	}

	if runTerraformInit {
		initCommandWithArguments := []string{"init"}
		if info.SubCommand == "workspace" || cliConfig.Components.Terraform.InitRunReconfigure {
			initCommandWithArguments = []string{"init", "-reconfigure"}
		}

		// Before executing `terraform init`, delete the `.terraform/environment` file from the component directory
		cleanTerraformWorkspace(cliConfig, componentPath)

		err = ExecuteShellCommand(
			cliConfig,
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

	// Handle cliConfig.Components.Terraform.ApplyAutoApprove flag
	if info.SubCommand == "apply" && cliConfig.Components.Terraform.ApplyAutoApprove && !info.UseTerraformPlan {
		if !u.SliceContainsString(info.AdditionalArgsAndFlags, autoApproveFlag) {
			info.AdditionalArgsAndFlags = append(info.AdditionalArgsAndFlags, autoApproveFlag)
		}
	}

	// Print command info
	u.LogDebug(cliConfig, "\nCommand info:")
	u.LogDebug(cliConfig, "Terraform binary: "+info.Command)

	if info.SubCommand2 == "" {
		u.LogDebug(cliConfig, fmt.Sprintf("Terraform command: %s", info.SubCommand))
	} else {
		u.LogDebug(cliConfig, fmt.Sprintf("Terraform command: %s %s", info.SubCommand, info.SubCommand2))
	}

	u.LogDebug(cliConfig, fmt.Sprintf("Arguments and flags: %v", info.AdditionalArgsAndFlags))
	u.LogDebug(cliConfig, "Component: "+info.ComponentFromArg)

	if len(info.BaseComponentPath) > 0 {
		u.LogDebug(cliConfig, "Terraform component: "+info.BaseComponentPath)
	}

	if len(info.ComponentInheritanceChain) > 0 {
		u.LogDebug(cliConfig, "Inheritance: "+info.ComponentFromArg+" -> "+strings.Join(info.ComponentInheritanceChain, " -> "))
	}

	if info.Stack == info.StackFromArg {
		u.LogDebug(cliConfig, "Stack: "+info.StackFromArg)
	} else {
		u.LogDebug(cliConfig, "Stack: "+info.StackFromArg)
		u.LogDebug(cliConfig, "Stack path: "+filepath.Join(cliConfig.BasePath, cliConfig.Stacks.BasePath, info.Stack))
	}

	u.LogDebug(cliConfig, fmt.Sprintf("Working dir: %s", workingDir))

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
		cleanTerraformWorkspace(cliConfig, componentPath)

		if cliConfig.Components.Terraform.InitRunReconfigure {
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
				cliConfig,
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
					cliConfig,
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
			cliConfig,
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
		err = ExecuteShellCommand(
			cliConfig,
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

	// Clean up
	if info.SubCommand != "plan" && info.PlanFile == "" {
		planFilePath := constructTerraformComponentPlanfilePath(cliConfig, info)
		_ = os.Remove(planFilePath)
	}

	if info.SubCommand == "apply" {
		varFilePath := constructTerraformComponentVarfilePath(cliConfig, info)
		_ = os.Remove(varFilePath)
	}

	return nil
}
