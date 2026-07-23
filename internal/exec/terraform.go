package exec

import (
	"context"
	"fmt"
	"os"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth/broker"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/dependencies"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	tfcache "github.com/cloudposse/atmos/pkg/terraform/cache"
	tfplugin "github.com/cloudposse/atmos/pkg/terraform/plugin"

	// Import backend provisioner to register S3 provisioner.
	_ "github.com/cloudposse/atmos/pkg/provisioner/backend"
)

const (
	terraformPluginCacheDirEnv              = tfplugin.CacheDirEnv
	terraformPluginCacheMayBreakLockFileEnv = tfplugin.CacheMayBreakLockFileEnv

	// BeforeTerraformInitEvent is the hook event name for provisioners that run before terraform init.
	// This matches the hook event registered by backend provisioners in pkg/provisioner/backend/backend.go.
	// See pkg/hooks/event.go (hooks.BeforeTerraformInit) for the canonical definition.
	beforeTerraformInitEvent = "before.terraform.init"

	// AfterTerraformInitEvent is the hook event name for provisioners that run after a
	// successful terraform init (e.g. the multi-platform providers-lock hook in
	// pkg/provisioner/lock). It is dispatched with a TerraformExecContext so the provisioner
	// can run a terraform subcommand against the live env, RC, and working directory.
	afterTerraformInitEvent = "after.terraform.init"

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

// startManagedTerraformCache starts the registry cache for this execution and returns
// its cleanup. It returns a no-op cleanup when caching is disabled or when the caller
// owns the cache lifecycle (info.TerraformCacheExternal, e.g. a graph-backed bulk run
// sharing one proxy across components). On a trust failure the proxy is closed before
// returning so it does not leak.
func startManagedTerraformCache(atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo) (*tfcache.Setup, func(), error) {
	defer perf.Track(atmosConfig, "exec.startManagedTerraformCache")()

	if info.TerraformCacheExternal {
		return nil, func() {}, nil
	}
	setup, cleanup, err := tfcache.StartForExecution(context.Background(), atmosConfig)
	if err != nil {
		return nil, func() {}, err
	}
	if setup == nil {
		return nil, cleanup, nil
	}
	info.TerraformCache = setup
	return setup, cleanup, nil
}

// ExecuteTerraform executes terraform commands.
// Optional ShellCommandOption values are forwarded to the final ExecuteShellCommand call.
func ExecuteTerraform(info schema.ConfigAndStacksInfo, opts ...ShellCommandOption) error {
	defer perf.Track(nil, "exec.ExecuteTerraform")()

	log.Debug(
		"ExecuteTerraform entry",
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

	// Ensure ambient credential brokers (e.g., Atmos Pro github/sts) have provisioned before the
	// subprocess environment is built, so terraform's own `git::` module fetches can read private
	// repos via the inherited GIT_CONFIG_* rewrites. Process-once and gated (CI + configured).
	broker.EnsureCredentials(context.Background(), &atmosConfig)

	// Start the Terraform registry cache (no-op when disabled or caller-owned). The
	// ephemeral proxy must outlive the whole pipeline, so its Close is deferred here.
	// Env assembly merges its CLI-config contribution into the generated RC.
	cacheSetup, closeCache, err := startManagedTerraformCache(&atmosConfig, &info)
	if err != nil {
		return err
	}
	if cacheSetup != nil {
		defer closeCache()
	}

	// Resolve paths, install toolchain, write varfiles, validate, run hooks, and build env.
	execCtx, err := prepareComponentExecution(&atmosConfig, &info, shouldProcess)
	if err != nil {
		return err
	}

	// Remove the temporary Terraform CLI config (TF_CLI_CONFIG_FILE) after the whole
	// pipeline (init, workspace, plan/apply) completes. Registered here, not inside the
	// pipeline, so the file survives every subprocess and is cleaned up on early errors.
	if info.RCCleanup != nil {
		defer func() {
			if cleanupErr := info.RCCleanup(); cleanupErr != nil {
				log.Debug("Failed to remove temporary Terraform CLI config", "error", cleanupErr)
			}
		}()
	}

	// Persist auth context so PostRunE hooks (e.g. store hooks that read
	// terraform outputs) can reuse the credentials established during this
	// execution. Without this, hooks create a fresh info with no auth.
	SetLastAuthContext(info.AuthContext, info.AuthManager)

	// Run the full command pipeline: init, arg build, workspace, execute, cleanup.
	// Forward caller-provided options (e.g. CI stdout/stderr capture) alongside the environment option.
	opts = append(opts, WithEnvironment(info.SanitizedEnv))
	err = executeCommandPipeline(&atmosConfig, &info, execCtx, opts...)
	if err == nil {
		// A successful Terraform command can create, change, or remove state. Drop
		// any preflight snapshot so a dependent graph node reads the current outputs.
		invalidateTerraformStateCache(info.Stack, info.ComponentFromArg)
	}
	return err
}

// configurePluginCache returns environment variables for Terraform plugin caching.
// It checks if the user has already set TF_PLUGIN_CACHE_DIR (via OS env or global env),
// and if not, configures automatic caching based on atmosConfig.Components.Terraform.PluginCache.
func configurePluginCache(atmosConfig *schema.AtmosConfiguration) []string {
	override, overrideSet := pluginCacheOverride(atmosConfig)
	cache := tfplugin.Resolve(atmosConfig, override, overrideSet)
	if !cache.Automatic {
		if cache.Directory != "" {
			log.Debug("TF_PLUGIN_CACHE_DIR already set, skipping automatic plugin cache configuration")
		}
		return nil
	}
	return []string{
		fmt.Sprintf("%s=%s", terraformPluginCacheDirEnv, cache.Directory),
		fmt.Sprintf("%s=true", terraformPluginCacheMayBreakLockFileEnv),
	}
}

// pluginCacheOverride resolves explicit user configuration with the historical
// command-path precedence: process environment first, then atmos.yaml global env.
func pluginCacheOverride(atmosConfig *schema.AtmosConfiguration) (string, bool) {
	if value, ok := os.LookupEnv(terraformPluginCacheDirEnv); ok {
		return value, true
	}
	if atmosConfig != nil {
		value, ok := atmosConfig.Env[terraformPluginCacheDirEnv]
		return value, ok
	}
	return "", false
}

// getValidUserPluginCacheDir checks if the user has set a valid TF_PLUGIN_CACHE_DIR.
// Returns the valid path if set, or empty string if not set or invalid.
// Invalid values (empty string or "/") are logged as warnings.
func getValidUserPluginCacheDir(atmosConfig *schema.AtmosConfiguration) string {
	// Check OS environment first.
	if osEnvDir, inOsEnv := os.LookupEnv(terraformPluginCacheDirEnv); inOsEnv {
		if isValidPluginCacheDir(osEnvDir, "environment variable") {
			return osEnvDir
		}
		return ""
	}

	// Check global env section in atmos.yaml.
	if globalEnvDir, inGlobalEnv := atmosConfig.Env[terraformPluginCacheDirEnv]; inGlobalEnv {
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
	return tfplugin.IsValidDirectory(path, source)
}

// disableTerraformPluginCacheForExecution removes Terraform/OpenTofu plugin-cache
// configuration from this execution. This is intentionally scoped to the current
// subprocess environment and config copy so concurrent graph runs can keep full
// scheduler parallelism without racing on a shared provider cache.
func disableTerraformPluginCacheForExecution(atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo) {
	if atmosConfig == nil || info == nil || !info.DisablePluginCache {
		return
	}

	atmosConfig.Components.Terraform.PluginCache = false
	atmosConfig.Components.Terraform.PluginCacheDir = ""

	delete(atmosConfig.Env, terraformPluginCacheDirEnv)
	delete(atmosConfig.Env, terraformPluginCacheMayBreakLockFileEnv)
	delete(info.ComponentEnvSection, terraformPluginCacheDirEnv)
	delete(info.ComponentEnvSection, terraformPluginCacheMayBreakLockFileEnv)

	baseEnv := info.SanitizedEnv
	if baseEnv == nil {
		baseEnv = os.Environ()
	}
	info.SanitizedEnv = removeEnvKeys(baseEnv, terraformPluginCacheDirEnv, terraformPluginCacheMayBreakLockFileEnv)
	info.ComponentEnvList = removeEnvKeys(info.ComponentEnvList, terraformPluginCacheDirEnv, terraformPluginCacheMayBreakLockFileEnv)
}

func removeEnvKeys(env []string, keys ...string) []string {
	if len(env) == 0 || len(keys) == 0 {
		return env
	}
	skip := make(map[string]struct{}, len(keys))
	for _, key := range keys {
		skip[key] = struct{}{}
	}

	filtered := env[:0]
	for _, entry := range env {
		key := envKey(entry)
		if _, ok := skip[key]; ok {
			continue
		}
		filtered = append(filtered, entry)
	}
	return filtered
}

func envKey(entry string) string {
	for i := 0; i < len(entry); i++ {
		if entry[i] == '=' {
			return entry[:i]
		}
	}
	return entry
}
