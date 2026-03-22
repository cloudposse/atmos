package exec

import (
	"fmt"
	"os"

	errUtils "github.com/cloudposse/atmos/errors"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/dependencies"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/xdg"

	// Import backend provisioner to register S3 provisioner.
	_ "github.com/cloudposse/atmos/pkg/provisioner/backend"
)

const (
	// BeforeTerraformInitEvent is the hook event name for provisioners that run before terraform init.
	// This matches the hook event registered by backend provisioners in pkg/provisioner/backend/backend.go.
	// See pkg/hooks/event.go (hooks.BeforeTerraformInit) for the canonical definition.
	beforeTerraformInitEvent = "before.terraform.init"

	subcommandApply     = "apply"
	subcommandDeploy    = "deploy"
	subcommandInit      = "init"
	subcommandWorkspace = "workspace"

	autoApproveFlag           = "-auto-approve"
	outFlag                   = "-out"
	varFileFlag               = "-var-file"
	skipTerraformLockFileFlag = "--skip-lock-file"
	forceFlag                 = "--force"
	everythingFlag            = "--everything"
	detailedExitCodeFlag      = "-detailed-exitcode"
	logFieldComponent         = "component"
	dirPermissions            = 0o755
)

// resolveAndInstallToolchainDeps resolves and installs toolchain dependencies for a terraform component.
// Returns the ToolchainEnvironment for resolving executable paths downstream.
func resolveAndInstallToolchainDeps(atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo) (*dependencies.ToolchainEnvironment, error) {
	defer perf.Track(atmosConfig, "exec.resolveAndInstallToolchainDeps")()

	tenv, err := dependencies.ForComponent(atmosConfig, "terraform", info.StackSection, info.ComponentSection)
	if err != nil {
		return nil, err
	}

	return tenv, nil
}

// ExecuteTerraform executes terraform commands.
// Optional ShellCommandOption values are forwarded to the final ExecuteShellCommand call.
func ExecuteTerraform(info schema.ConfigAndStacksInfo, opts ...ShellCommandOption) error {
	defer perf.Track(nil, "exec.ExecuteTerraform")()

	log.Debug("ExecuteTerraform entry",
		"SubCommand", info.SubCommand,
		"ComponentFromArg", info.ComponentFromArg,
		"FinalComponent", info.FinalComponent,
		"Stack", info.Stack,
		"StackFromArg", info.StackFromArg,
	)

	info.CliArgs = []string{"terraform", info.SubCommand, info.SubCommand2}

	atmosConfig, err := cfg.InitCliConfig(info, true)
	if err != nil {
		return err
	}

	if info.NeedHelp {
		return nil
	}

	// Resolve the terraform executable (e.g. "terraform", "tofu", or a custom path).
	resolveTerraformCommand(&atmosConfig, &info)

	// Short-circuit for `terraform version` – no stack processing required.
	if info.SubCommand == "version" {
		return handleVersionSubcommand(&atmosConfig, &info)
	}

	// Set up authentication (merge global + component auth, create AuthManager, inject bridge).
	authManager, err := setupTerraformAuth(&atmosConfig, &info)
	if err != nil {
		return err
	}

	// Process and validate stack configuration.
	shouldProcess, shouldCheckStack := shouldProcessStacks(&info)
	if shouldProcess {
		info, err = ProcessStacks(&atmosConfig, info, shouldCheckStack, info.ProcessTemplates, info.ProcessFunctions, info.Skip, authManager)
		if err != nil {
			return err
		}
	}
	if shouldCheckStack && len(info.Stack) < 1 {
		return errUtils.ErrMissingStack
	}
	if !info.ComponentIsEnabled {
		log.Info("Component is not enabled and skipped", logFieldComponent, info.ComponentFromArg)
		return nil
	}

	// Resolve paths, install toolchain, write varfiles, validate, run hooks, and build env.
	execCtx, err := prepareComponentExecution(&atmosConfig, &info, shouldProcess)
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

	// Set TF_PLUGIN_CACHE_DIR for Terraform provider caching.
	pluginCacheEnvList := configurePluginCache(&atmosConfig)
	info.ComponentEnvList = append(info.ComponentEnvList, pluginCacheEnvList...)

	// Append toolchain PATH last so it takes precedence over any PATH entries
	// from ComponentEnvSection, auth hooks, or other env sources.
	if tenv != nil {
		info.ComponentEnvList = append(info.ComponentEnvList, tenv.EnvVars()...)
	}

	// Print ENV vars if they are found in the component's stack config.
	if len(info.ComponentEnvList) > 0 {
		log.Debug("Using ENV vars:")
		for _, v := range info.ComponentEnvList {
			log.Debug(v)
		}
	}

	// Run `terraform init` before running other commands.
	// Note: 'clean' is no longer checked here since it doesn't route through ExecuteTerraform.
	runTerraformInit := true
	if info.SubCommand == "init" ||
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
		// Add `--var-file` if configured in `atmos.yaml.
		// OpenTofu supports passing a varfile to `init` to dynamically configure backends.
		if atmosConfig.Components.Terraform.Init.PassVars {
			initCommandWithArguments = append(initCommandWithArguments, []string{varFileFlag, varFile}...)
		}

		// Before executing `terraform init`, delete the `.terraform/environment` file from the component directory.
		cleanTerraformWorkspace(atmosConfig, componentPath)

		// Execute provisioners registered for before.terraform.init hook event.
		// This runs backend provisioners to ensure backends exist before Terraform tries to configure them.
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()

		err = provisioner.ExecuteProvisioners(ctx, provisioner.HookEvent(beforeTerraformInitEvent), &atmosConfig, info.ComponentSection, info.AuthContext)
		if err != nil {
			return fmt.Errorf("provisioner execution failed: %w", err)
		}

		// Check if workdir provisioner set a workdir path - if so, use it instead of the component path.
		if workdirPath, ok := info.ComponentSection[provWorkdir.WorkdirPathKey].(string); ok && workdirPath != "" {
			componentPath = workdirPath
			log.Debug("Using workdir path", "workdirPath", workdirPath)
		}

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

		// Always remove the flag from AdditionalArgsAndFlags since it's only used internally by Atmos.
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
		// Before executing `terraform init`, delete the `.terraform/environment` file from the component directory.
		cleanTerraformWorkspace(atmosConfig, componentPath)

		// Execute provisioners registered for before.terraform.init hook event.
		// This runs backend provisioners to ensure backends exist before Terraform tries to configure them.
		initCtx, initCancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer initCancel()

		err = provisioner.ExecuteProvisioners(initCtx, provisioner.HookEvent(beforeTerraformInitEvent), &atmosConfig, info.ComponentSection, info.AuthContext)
		if err != nil {
			return fmt.Errorf("provisioner execution failed: %w", err)
		}

		// Check if workdir provisioner set a workdir path - if so, use it instead of the component path.
		if workdirPath, ok := info.ComponentSection[provWorkdir.WorkdirPathKey].(string); ok && workdirPath != "" {
			componentPath = workdirPath
			log.Debug("Using workdir path for terraform command", "workdirPath", workdirPath)
		}

		if atmosConfig.Components.Terraform.InitRunReconfigure {
			allArgsAndFlags = append(allArgsAndFlags, []string{"-reconfigure"}...)
		}
		// Add `--var-file` if configured in `atmos.yaml.
		// OpenTofu supports passing a varfile to `init` to dynamically configure backends.
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
			// Otherwise, use the planfile name what is autogenerated by Atmos
			allArgsAndFlags = append(allArgsAndFlags, []string{planFile}...)
		}
	}

	// Run `terraform workspace` before executing other terraform commands
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

				// For data-producing subcommands (output, show), redirect workspace select
				// stdout to stderr. Terraform writes "Switched to workspace..." to stdout,
				// which pollutes captured output in $() shell substitutions.
				var wsOpts []ShellCommandOption
				if info.SubCommand == "output" || info.SubCommand == "show" {
					wsOpts = append(wsOpts, WithStdoutOverride(os.Stderr))
				}

				err = ExecuteShellCommand(
					atmosConfig,
					info.Command,
					[]string{"workspace", "select", info.TerraformWorkspace},
					componentPath,
					info.ComponentEnvList,
					info.DryRun,
					workspaceSelectRedirectStdErr,
					wsOpts...,
				)
				if err != nil {
					// Check if it's an ExitCodeError with code 1 (workspace doesn't exist)
					var exitCodeErr errUtils.ExitCodeError
					if !errors.As(err, &exitCodeErr) || exitCodeErr.Code != 1 {
						// Different error or different exit code
						return err
					}
					// Workspace doesn't exist, try to create it
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
						// If `workspace new` also fails with code 1, the workspace may already
						// be the active workspace (the .terraform/environment file names it) but
						// its state directory was deleted.  In that case we are already in the
						// correct workspace and can proceed safely.
						var newExitCodeErr errUtils.ExitCodeError
						if errors.As(err, &newExitCodeErr) && newExitCodeErr.Code == 1 &&
							isTerraformCurrentWorkspace(componentPath, info.TerraformWorkspace) {
							log.Warn("Workspace is already active but its state directory is missing; proceeding — subsequent terraform commands may report missing state",
								"workspace", info.TerraformWorkspace)
						} else {
							return err
						}
					}
				}
			}
		}
	}

	// Check if the terraform command requires a user interaction,
	// but it's running in a scripted environment (where a `tty` is not attached or `stdin` is not attached).
	if os.Stdin == nil && !u.SliceContainsString(info.AdditionalArgsAndFlags, autoApproveFlag) {
		if info.SubCommand == "apply" {
			return fmt.Errorf("%w: 'terraform apply' requires a user interaction, but no TTY is attached. Use 'terraform apply -auto-approve' or 'terraform deploy' instead",
				errUtils.ErrNoTty,
			)
		}
	}

	// Check `region` for `terraform import`.
	if info.SubCommand == "import" {
		if region, regionExist := info.ComponentVarsSection["region"].(string); regionExist {
			info.ComponentEnvList = append(info.ComponentEnvList, fmt.Sprintf("AWS_REGION=%s", region))
		}
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
			opts...,
		)
		// Compute exitCode for upload, whether or not err is set.
		var exitCode int
		if err != nil {
			// Prefer our typed error to preserve exit codes from subcommands.
			var ec errUtils.ExitCodeError
			if errors.As(err, &ec) {
				exitCode = ec.Code
			} else {
				var osErr *osexec.ExitError
				if errors.As(err, &osErr) {
					exitCode = osErr.ExitCode()
				} else {
					exitCode = 1
				}
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
			// Treat 0 and 2 as success for plan uploads, but preserve exit code.
			if exitCode == 0 {
				return nil
			}
			if exitCode == 2 {
				// Exit code 2 is success for terraform plan but we must preserve it
				return errUtils.ExitCodeError{Code: 2}
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
		if err := os.Remove(planFilePath); err != nil && !os.IsNotExist(err) {
			log.Trace("Failed to remove plan file during cleanup", "error", err, "file", planFilePath)
		}
	}

	if info.SubCommand == "apply" {
		varFilePath := constructTerraformComponentVarfilePath(&atmosConfig, &info)
		if err := os.Remove(varFilePath); err != nil && !os.IsNotExist(err) {
			log.Trace("Failed to remove var file during cleanup", "error", err, "file", varFilePath)
		}
	}

	// Run the full command pipeline: init, arg build, workspace, execute, cleanup.
	return executeCommandPipeline(&atmosConfig, &info, execCtx, opts...)
}

// configurePluginCache returns environment variables for Terraform plugin caching.
// It checks if the user has already set TF_PLUGIN_CACHE_DIR (via OS env or global env),
// and if not, configures automatic caching based on atmosConfig.Components.Terraform.PluginCache.
func configurePluginCache(atmosConfig *schema.AtmosConfiguration) []string {
	// Check both OS env and global env (atmos.yaml env: section) for user override.
	// If user has TF_PLUGIN_CACHE_DIR set to a valid path, do nothing - they manage their own cache.
	// Invalid values (empty string or "/") are ignored with a warning, and we use our default.
	if userCacheDir := getValidUserPluginCacheDir(atmosConfig); userCacheDir != "" {
		log.Debug("TF_PLUGIN_CACHE_DIR already set, skipping automatic plugin cache configuration")
		return nil
	}

	if !atmosConfig.Components.Terraform.PluginCache {
		return nil
	}

	pluginCacheDir := atmosConfig.Components.Terraform.PluginCacheDir

	// Use XDG cache directory if no custom path configured.
	if pluginCacheDir == "" {
		cacheDir, err := xdg.GetXDGCacheDir("terraform/plugins", xdg.DefaultCacheDirPerm)
		if err != nil {
			log.Warn("Failed to create plugin cache directory", "error", err)
			return nil
		}
		pluginCacheDir = cacheDir
	}

	if pluginCacheDir == "" {
		return nil
	}

	return []string{
		fmt.Sprintf("TF_PLUGIN_CACHE_DIR=%s", pluginCacheDir),
		"TF_PLUGIN_CACHE_MAY_BREAK_DEPENDENCY_LOCK_FILE=true",
	}
}

// getValidUserPluginCacheDir checks if the user has set a valid TF_PLUGIN_CACHE_DIR.
// Returns the valid path if set, or empty string if not set or invalid.
// Invalid values (empty string or "/") are logged as warnings.
func getValidUserPluginCacheDir(atmosConfig *schema.AtmosConfiguration) string {
	// Check OS environment first.
	if osEnvDir, inOsEnv := os.LookupEnv("TF_PLUGIN_CACHE_DIR"); inOsEnv {
		if isValidPluginCacheDir(osEnvDir, "environment variable") {
			return osEnvDir
		}
		return ""
	}

	// Check global env section in atmos.yaml.
	if globalEnvDir, inGlobalEnv := atmosConfig.Env["TF_PLUGIN_CACHE_DIR"]; inGlobalEnv {
		if isValidPluginCacheDir(globalEnvDir, "atmos.yaml env section") {
			return globalEnvDir
		}
		return ""
	}

	return ""
}

// isValidPluginCacheDir checks if a plugin cache directory path is valid.
// Invalid paths (empty string or "/") are logged as warnings and return false.
func isValidPluginCacheDir(path, source string) bool {
	if path == "" {
		log.Warn("TF_PLUGIN_CACHE_DIR is empty, ignoring and using Atmos default", "source", source)
		return false
	}
	if path == "/" {
		log.Warn("TF_PLUGIN_CACHE_DIR is set to root '/', ignoring and using Atmos default", "source", source)
		return false
	}
	return true
}
