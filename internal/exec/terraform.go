package exec

import (
	"fmt"
	"os"
	osexec "os/exec"
	"path/filepath"
	"strings"

	"github.com/cockroachdb/errors"

	errUtils "github.com/cloudposse/atmos/errors"
	auth "github.com/cloudposse/atmos/pkg/auth"
	cfg "github.com/cloudposse/atmos/pkg/config"
	git "github.com/cloudposse/atmos/pkg/git"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/pro"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

const logFieldComponent = "component"

const (
	autoApproveFlag           = "-auto-approve"
	outFlag                   = "-out"
	varFileFlag               = "-var-file"
	skipTerraformLockFileFlag = "--skip-lock-file"
	forceFlag                 = "--force"
	detailedExitCodeFlag      = "--detailed-exitcode"
)

// ErrHTTPBackendWorkspaces is returned when attempting to use workspace commands with an HTTP backend.
var (
	ErrHTTPBackendWorkspaces     = errors.New("workspaces are not supported for the HTTP backend")
	ErrMissingStack              = errors.New("stack must be specified")
	ErrInvalidTerraformComponent = errors.New("invalid Terraform component")
	ErrAbstractComponent         = errors.New("component is abstract")
	ErrLockedComponent           = errors.New("component is locked")
	ErrComponentNotValid         = errors.New("component is invalid")
	ErrNoTty                     = errors.New("no TTY attached")
)

// ExecuteTerraform executes terraform commands.
func ExecuteTerraform(info schema.ConfigAndStacksInfo) error {
	defer perf.Track(nil, "exec.ExecuteTerraform")()

	info.CliArgs = []string{"terraform", info.SubCommand, info.SubCommand2}

	atmosConfig, err := cfg.InitCliConfig(info, true)
	if err != nil {
		return err
	}

	if info.NeedHelp {
		return nil
	}

	// Add the `command` from `components.terraform.command` from `atmos.yaml`.
	if info.Command == "" {
		if atmosConfig.Components.Terraform.Command != "" {
			info.Command = atmosConfig.Components.Terraform.Command
		} else {
			info.Command = cfg.TerraformComponentType
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
			info.RedirectStdErr)
	}

	// Skip stack processing when cleaning with the `--force` flag to allow cleaning without requiring stack configuration.
	shouldProcessStacks, shouldCheckStack := shouldProcessStacks(&info)

	if shouldProcessStacks {
		info, err = ProcessStacks(&atmosConfig, info, shouldCheckStack, info.ProcessTemplates, info.ProcessFunctions, info.Skip)
		if err != nil {
			return err
		}

		if len(info.Stack) < 1 && shouldCheckStack {
			return errUtils.WithExitCode(ErrMissingStack, 2)
		}
	}

	if !info.ComponentIsEnabled && info.SubCommand != "clean" {
		log.Info("Component is not enabled and skipped", logFieldComponent, info.ComponentFromArg)
		return nil
	}

	err = checkTerraformConfig(atmosConfig)
	if err != nil {
		return err
	}

	// Check if the component (or base component) exists as a Terraform component.
	componentPath, err := u.GetComponentPath(&atmosConfig, "terraform", info.ComponentFolderPrefix, info.FinalComponent)
	if err != nil {
		return fmt.Errorf("failed to resolve component path: %w", err)
	}

	componentPathExists, err := u.IsDirectory(componentPath)
	if err != nil || !componentPathExists {
		// Get the base path for error message, respecting user's actual config.
		basePath, err := u.GetComponentBasePath(&atmosConfig, "terraform")
		if err != nil {
			return fmt.Errorf("getting component base path for terraform: %w", err)
		}
		err = errUtils.Build(fmt.Errorf("%w: '%s' points to the Terraform component '%s', but it does not exist in '%s'",
			ErrInvalidTerraformComponent,
			info.ComponentFromArg,
			info.FinalComponent,
			basePath,
		)).
			WithContext("component", info.ComponentFromArg).
			WithContext("terraform_component", info.FinalComponent).
			WithContext("stack", info.Stack).
			WithContext("base_path", basePath).
			WithHint("Use `atmos list components` to see available components").
			WithHintf("Verify the component path: %s", basePath).
			WithExitCode(2).
			Err()
		return err
	}

	// Check if the component is allowed to be provisioned (the `metadata.type` attribute is not set to `abstract`).
	if (info.SubCommand == "plan" || info.SubCommand == "apply" || info.SubCommand == "deploy" || info.SubCommand == "workspace") && info.ComponentIsAbstract {
		err := errUtils.Build(fmt.Errorf("%w: the component '%s' cannot be provisioned because it's marked as abstract (metadata.type: abstract)",
			ErrAbstractComponent,
			filepath.Join(info.ComponentFolderPrefix,
				info.Component,
			))).
			WithContext("component", info.Component).
			WithContext("stack", info.Stack).
			WithContext("component_path", filepath.Join(info.ComponentFolderPrefix, info.Component)).
			WithHint("Abstract components are meant to be inherited, not provisioned directly").
			WithHintf("Create a concrete component that inherits from `%s`", info.Component).
			WithExitCode(2).
			Err()
		return err
	}

	// Check if the component is locked (`metadata.locked` is set to true).
	if info.ComponentIsLocked {
		// Allow read-only commands, block modification commands.
		switch info.SubCommand {
		case "apply", "deploy", "destroy", "import", "state", "taint", "untaint":
			err := errUtils.Build(fmt.Errorf("%w: component '%s' cannot be modified (metadata.locked: true)",
				ErrLockedComponent,
				filepath.Join(info.ComponentFolderPrefix, info.Component),
			)).
				WithContext("component", info.Component).
				WithContext("stack", info.Stack).
				WithContext("subcommand", info.SubCommand).
				WithContext("component_path", filepath.Join(info.ComponentFolderPrefix, info.Component)).
				WithHint("Remove `metadata.locked: true` from the component configuration to enable modifications").
				WithHint("Locked components are read-only to prevent accidental changes").
				WithExitCode(2).
				Err()
			return err
		}
	}

	// Check if trying to use `workspace` commands with HTTP backend.
	if info.SubCommand == "workspace" && info.ComponentBackendType == "http" {
		return ErrHTTPBackendWorkspaces
	}

	if info.SubCommand == "clean" {
		err = handleCleanSubCommand(info, componentPath, &atmosConfig)
		if err != nil {
			log.Debug("Error executing 'terraform clean'", logFieldComponent, componentPath, "error", err)
			return err
		}
		return nil
	}

	varFile := constructTerraformComponentVarfileName(&info)
	planFile := constructTerraformComponentPlanfileName(&info)

	// Print component variables and write to file.
	// Don't process variables when executing `terraform workspace` commands.
	if info.SubCommand != "workspace" {
		log.Debug("Variables for the component in the stack", logFieldComponent, info.ComponentFromArg, "stack", info.Stack)
		if atmosConfig.Logs.Level == u.LogLevelTrace || atmosConfig.Logs.Level == u.LogLevelDebug {
			err = u.PrintAsYAMLToFileDescriptor(&atmosConfig, info.ComponentVarsSection)
			if err != nil {
				return err
			}
		}

		// Write variables to a file (only if we are not using the previously generated terraform plan).
		if !info.UseTerraformPlan {
			var varFilePath, varFileNameFromArg string

			// Handle `terraform varfile` and `terraform write varfile` legacy commands.
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
				varFilePath = constructTerraformComponentVarfilePath(&atmosConfig, &info)
			}

			log.Debug("Writing the variables", "file", varFilePath)

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
			log.Debug("CLI variables (will override the variables defined in the stack manifests):")
			if atmosConfig.Logs.Level == u.LogLevelTrace || atmosConfig.Logs.Level == u.LogLevelDebug {
				err = u.PrintAsYAMLToFileDescriptor(&atmosConfig, cliVars)
				if err != nil {
					return err
				}
			}
		}
	}

	// Handle `terraform varfile` and `terraform write varfile` legacy commands.
	if info.SubCommand == "varfile" || (info.SubCommand == "write" && info.SubCommand2 == "varfile") {
		return nil
	}

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
			ErrComponentNotValid,
			info.ComponentFromArg,
		)
	}

	err = auth.TerraformPreHook(&atmosConfig, &info)
	if err != nil {
		log.Error("Error executing 'atmos auth terraform pre-hook'", logFieldComponent, info.ComponentFromArg, "error", err)
	}

	// Component working directory.
	workingDir := constructTerraformComponentWorkingDir(&atmosConfig, &info)

	err = generateBackendConfig(&atmosConfig, &info, workingDir)
	if err != nil {
		return err
	}

	err = generateProviderOverrides(&atmosConfig, &info, workingDir)
	if err != nil {
		return err
	}

	// Check for specific Terraform environment variables that might conflict with Atmos.
	warnOnExactVars := []string{
		"TF_CLI_ARGS",
		"TF_WORKSPACE",
	}

	warnOnPrefixVars := []string{
		"TF_VAR_",
		"TF_CLI_ARGS_",
	}

	var problematicVars []string

	for _, envVar := range os.Environ() {
		if parts := strings.SplitN(envVar, "=", 2); len(parts) == 2 {
			// Check for exact matches.
			if u.SliceContainsString(warnOnExactVars, parts[0]) {
				problematicVars = append(problematicVars, parts[0])
			}
			// Check for prefix matches.
			for _, prefix := range warnOnPrefixVars {
				if strings.HasPrefix(parts[0], prefix) {
					problematicVars = append(problematicVars, parts[0])
					break
				}
			}
		}
	}

	if len(problematicVars) > 0 {
		log.Warn("Detected environment variables that may interfere with Atmos's control of Terraform",
			"variables", problematicVars)
	}

	info.ComponentEnvList = append(info.ComponentEnvList, fmt.Sprintf("ATMOS_CLI_CONFIG_PATH=%s", atmosConfig.CliConfigPath))
	basePath, err := filepath.Abs(atmosConfig.BasePath)
	if err != nil {
		return err
	}
	info.ComponentEnvList = append(info.ComponentEnvList, fmt.Sprintf("ATMOS_BASE_PATH=%s", basePath))
	// Set `TF_IN_AUTOMATION` ENV var to `true` to suppress verbose instructions after terraform commands.
	// https://developer.hashicorp.com/terraform/cli/config/environment-variables#tf_in_automation
	info.ComponentEnvList = append(info.ComponentEnvList, "TF_IN_AUTOMATION=true")

	// Set 'TF_APPEND_USER_AGENT' ENV var based on precedence.
	// Precedence: Environment Variable > atmos.yaml > Default.
	appendUserAgent := atmosConfig.Components.Terraform.AppendUserAgent
	if envUA, exists := os.LookupEnv("TF_APPEND_USER_AGENT"); exists && envUA != "" {
		appendUserAgent = envUA
	}
	if appendUserAgent != "" {
		info.ComponentEnvList = append(info.ComponentEnvList, fmt.Sprintf("TF_APPEND_USER_AGENT=%s", appendUserAgent))
	}

	// Print ENV vars if they are found in the component's stack config.
	if len(info.ComponentEnvList) > 0 {
		log.Debug("Using ENV vars:")
		for _, v := range info.ComponentEnvList {
			log.Debug(v)
		}
	}

	// Run `terraform init` before running other commands.
	runTerraformInit := true
	if info.SubCommand == "init" ||
		info.SubCommand == "clean" ||
		(info.SubCommand == "deploy" && !atmosConfig.Components.Terraform.DeployRunInit) {
		runTerraformInit = false
	}

	if info.SkipInit {
		log.Debug("Skipping over 'terraform init' due to '--skip-init' flag being passed")
		runTerraformInit = false
	}

	if runTerraformInit {
		initCommandWithArguments := []string{"init"}
		if info.SubCommand == "workspace" || atmosConfig.Components.Terraform.InitRunReconfigure {
			initCommandWithArguments = []string{"init", "-reconfigure"}
		}
		// Add `--var-file` if configured in `atmos.yaml`.
		// NOTE: This feature is primarily intended for OpenTofu, which supports passing
		// a varfile to `init` for dynamic backend configuration. HashiCorp Terraform does
		// not support --var-file for the init command and will fail if this is enabled.
		// If you are using Terraform (not OpenTofu), do not enable PassVars, or ensure
		// your terraform command actually points to an OpenTofu binary.
		if atmosConfig.Components.Terraform.Init.PassVars {
			initCommandWithArguments = append(initCommandWithArguments, []string{varFileFlag, varFile}...)
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

	// Handle `terraform deploy` custom command.
	if info.SubCommand == "deploy" {
		info.SubCommand = "apply"
		if !info.UseTerraformPlan && !u.SliceContainsString(info.AdditionalArgsAndFlags, autoApproveFlag) {
			info.AdditionalArgsAndFlags = append(info.AdditionalArgsAndFlags, autoApproveFlag)
		}
	}

	// Handle atmosConfig.Components.Terraform.ApplyAutoApprove flag.
	if info.SubCommand == "apply" && atmosConfig.Components.Terraform.ApplyAutoApprove && !info.UseTerraformPlan {
		if !u.SliceContainsString(info.AdditionalArgsAndFlags, autoApproveFlag) {
			info.AdditionalArgsAndFlags = append(info.AdditionalArgsAndFlags, autoApproveFlag)
		}
	}

	// Print the command info/context.
	var command string
	if info.SubCommand2 == "" {
		command = info.SubCommand
	} else {
		command = fmt.Sprintf("%s %s", info.SubCommand, info.SubCommand2)
	}

	var inheritance string
	if len(info.ComponentInheritanceChain) > 0 {
		inheritance = info.ComponentFromArg + " -> " + strings.Join(info.ComponentInheritanceChain, " -> ")
	}

	log.Debug("Terraform context",
		"executable", info.Command,
		"command", command,
		logFieldComponent, info.ComponentFromArg,
		"stack", info.StackFromArg,
		"arguments and flags", info.AdditionalArgsAndFlags,
		"terraform component", info.BaseComponentPath,
		"inheritance", inheritance,
		"working directory", workingDir,
	)

	// Prepare the terraform command.
	allArgsAndFlags := strings.Fields(info.SubCommand)
	uploadStatusFlag := false

	switch info.SubCommand {
	case "plan":
		// Add varfile.
		allArgsAndFlags = append(allArgsAndFlags, []string{varFileFlag, varFile}...)
		// Add planfile.
		if !u.SliceContainsString(info.AdditionalArgsAndFlags, outFlag) &&
			!u.SliceContainsStringHasPrefix(info.AdditionalArgsAndFlags, outFlag+"=") &&
			!atmosConfig.Components.Terraform.Plan.SkipPlanfile {
			allArgsAndFlags = append(allArgsAndFlags, []string{outFlag, planFile}...)
		}
		// Check if the upload flag is present and parse its value (supports --flag, --flag=true, --flag=false forms).
		uploadStatusFlag = parseUploadStatusFlag(info.AdditionalArgsAndFlags, cfg.UploadStatusFlag)

		// Always remove the flag from AdditionalArgsAndFlags since it's only used internally by atmos.
		info.AdditionalArgsAndFlags = u.SliceRemoveFlag(info.AdditionalArgsAndFlags, cfg.UploadStatusFlag)

		if uploadStatusFlag {
			if !u.SliceContainsString(info.AdditionalArgsAndFlags, detailedExitCodeFlag) {
				allArgsAndFlags = append(allArgsAndFlags, []string{detailedExitCodeFlag}...)
			}
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
		// Add `--var-file` if configured in `atmos.yaml`.
		// NOTE: This feature is primarily intended for OpenTofu, which supports passing
		// a varfile to `init` for dynamic backend configuration. HashiCorp Terraform does
		// not support --var-file for the init command and will fail if this is enabled.
		// If you are using Terraform (not OpenTofu), do not enable PassVars, or ensure
		// your terraform command actually points to an OpenTofu binary.
		if atmosConfig.Components.Terraform.Init.PassVars {
			allArgsAndFlags = append(allArgsAndFlags, []string{varFileFlag, varFile}...)
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
			// If the planfile name was passed on the command line, use it.
			allArgsAndFlags = append(allArgsAndFlags, []string{info.PlanFile}...)
		} else {
			// Otherwise, use the planfile name what is autogenerated by Atmos.
			allArgsAndFlags = append(allArgsAndFlags, []string{planFile}...)
		}
	}

	// Handle the plan-diff command.
	if info.SubCommand == "plan-diff" {
		return TerraformPlanDiff(&atmosConfig, &info)
	}

	// Run `terraform workspace` before executing other terraform commands.
	// only if the `TF_WORKSPACE` environment variable is not set by the caller.
	if info.SubCommand != "init" && !(info.SubCommand == "workspace" && info.SubCommand2 != "") {
		// Don't use workspace commands in http backend.
		if info.ComponentBackendType != "http" {

			tfWorkspaceEnvVar := os.Getenv("TF_WORKSPACE")

			if tfWorkspaceEnvVar == "" {
				workspaceSelectRedirectStdErr := "/dev/stdout"

				// If `--redirect-stderr` flag is not passed, always redirect `stderr` to `stdout` for `terraform workspace select` command.
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
						// err is not a non-zero exit code or err is not exit code 1, which we are expecting.
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
	}

	// Check `region` for `terraform import`.
	if info.SubCommand == "import" {
		if region, regionExist := info.ComponentVarsSection["region"].(string); regionExist {
			info.ComponentEnvList = append(info.ComponentEnvList, fmt.Sprintf("AWS_REGION=%s", region))
		}
	}

	// Execute `terraform shell` command.
	if info.SubCommand == "shell" {
		err = execTerraformShellCommand(
			&atmosConfig,
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

	// Execute the provided command (except for `terraform workspace` which was executed above).
	if !(info.SubCommand == "workspace" && info.SubCommand2 == "") {
		err = ExecuteShellCommand(
			atmosConfig,
			info.Command,
			allArgsAndFlags,
			componentPath,
			info.ComponentEnvList,
			info.DryRun,
			info.RedirectStdErr,
		)
		// Compute exitCode for upload, whether or not err is set.
		var exitCode int
		if err != nil {
			var osErr *osexec.ExitError
			if errors.As(err, &osErr) {
				exitCode = osErr.ExitCode()
			} else {
				exitCode = 1
			}
		} else {
			exitCode = 0
		}

		// Upload plan status if requested.
		if uploadStatusFlag && shouldUploadStatus(&info) {
			client, cerr := pro.NewAtmosProAPIClientFromEnv(&atmosConfig)
			if cerr != nil {
				return cerr
			}
			gitRepo := &git.DefaultGitRepo{}
			if uerr := uploadStatus(&info, exitCode, client, gitRepo); uerr != nil {
				return uerr
			}
			// Treat 0 and 2 as success for plan uploads.
			if exitCode == 0 || exitCode == 2 {
				return nil
			}
		}
		// For other commands or failure, return the original error.
		if err != nil {
			return err
		}
	}

	// Clean up.
	if info.SubCommand != "plan" && info.SubCommand != "show" && info.PlanFile == "" {
		planFilePath := constructTerraformComponentPlanfilePath(&atmosConfig, &info)
		_ = os.Remove(planFilePath)
	}

	if info.SubCommand == "apply" {
		varFilePath := constructTerraformComponentVarfilePath(&atmosConfig, &info)
		_ = os.Remove(varFilePath)
	}

	return nil
}
