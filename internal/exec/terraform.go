package exec

import (
	"fmt"
	"os"

	errUtils "github.com/cloudposse/atmos/errors"
	auth "github.com/cloudposse/atmos/pkg/auth"
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

	autoApproveFlag           = "-auto-approve"
	outFlag                   = "-out"
	varFileFlag               = "-var-file"
	skipTerraformLockFileFlag = "--skip-lock-file"
	forceFlag                 = "--force"
	everythingFlag            = "--everything"
	detailedExitCodeFlag      = "-detailed-exitcode"
	logFieldComponent         = "component"
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
		return handleVersionSubcommand(atmosConfig, info)
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

	if err = checkTerraformConfig(atmosConfig); err != nil {
		return err
	}

	// Resolve the component filesystem path, auto-generate files, and run JIT provisioning.
	componentPath, err := resolveAndProvisionComponentPath(&atmosConfig, &info)
	if err != nil {
		return err
	}

	// Validate component access restrictions (abstract, locked, backend type).
	if err = checkComponentRestrictions(&info); err != nil {
		return err
	}

	// Resolve and install the toolchain (e.g. the exact terraform/tofu binary version).
	var tenv *dependencies.ToolchainEnvironment
	if shouldProcess {
		tenv, err = resolveAndInstallToolchainDeps(&atmosConfig, &info)
		if err != nil {
			return err
		}
		info.Command = tenv.Resolve(info.Command)
	}

	varFile := constructTerraformComponentVarfileName(&info)
	planFile := constructTerraformComponentPlanfileName(&info)

	// Print and persist component variable files.
	if err = printAndWriteVarFiles(&atmosConfig, &info, varFile); err != nil {
		return err
	}

	// Run OPA / JSON-schema validation policies.
	if err = validateTerraformComponent(&atmosConfig, &info); err != nil {
		return err
	}

	// Execute the auth pre-hook (e.g. assume-role, generate credentials).
	if err = auth.TerraformPreHook(&atmosConfig, &info); err != nil {
		log.Error("Error executing 'atmos auth terraform pre-hook'", logFieldComponent, info.ComponentFromArg, "error", err)
		return err
	}

	// Generate backend.tf.json, provider overrides, and other generated files.
	workingDir := constructTerraformComponentWorkingDir(&atmosConfig, &info)
	if err = generateConfigFiles(&atmosConfig, &info, workingDir); err != nil {
		return err
	}

	// Warn about environment variables that may conflict with Atmos.
	warnOnConflictingEnvVars()

	// Assemble the complete environment variable list for the subprocess.
	if err = assembleComponentEnvVars(&atmosConfig, &info, tenv); err != nil {
		return err
	}

	// Run `terraform init` as a pre-step before the main command when applicable.
	if shouldRunTerraformInit(&atmosConfig, &info) {
		componentPath, err = executeTerraformInitPhase(&atmosConfig, &info, componentPath, varFile)
		if err != nil {
			return err
		}
	}

	// Convert `deploy` → `apply` and add -auto-approve as needed.
	handleDeploySubcommand(&atmosConfig, &info)

	logTerraformContext(&info, workingDir)

	// Build the full argument list for the main terraform command.
	allArgsAndFlags, uploadStatusFlag, err := buildTerraformCommandArgs(&atmosConfig, &info, varFile, planFile, &componentPath)
	if err != nil {
		return err
	}

	// Select or create the Terraform workspace.
	if err = runWorkspaceSetup(&atmosConfig, &info, componentPath); err != nil {
		return err
	}

	// Guard against running `apply` interactively without a TTY.
	if err = checkTTYRequirement(&info); err != nil {
		return err
	}

	// Inject AWS_REGION for `terraform import`.
	addRegionEnvVarForImport(&info)

	// Execute the main terraform command and handle exit codes / status uploads.
	if err = executeMainTerraformCommand(&atmosConfig, &info, allArgsAndFlags, componentPath, uploadStatusFlag, opts...); err != nil {
		return err
	}

	// Remove ephemeral plan and varfiles generated by Atmos.
	cleanupTerraformFiles(&atmosConfig, &info)

	return nil
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
