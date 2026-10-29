package terraform

import (
	errors "errors"
	fmt "fmt"
	os "os"
	filepath "path/filepath"
	slices "slices"
	strings "strings"

	errUtils "github.com/cloudposse/atmos/errors"
	auth "github.com/cloudposse/atmos/pkg/auth"
	cfg "github.com/cloudposse/atmos/pkg/config"
	dependencies "github.com/cloudposse/atmos/pkg/dependencies"
	log "github.com/cloudposse/atmos/pkg/logger"
	perf "github.com/cloudposse/atmos/pkg/perf"
	schema "github.com/cloudposse/atmos/pkg/schema"
	store "github.com/cloudposse/atmos/pkg/store"
	authbridge "github.com/cloudposse/atmos/pkg/store/authbridge"
	tfcache "github.com/cloudposse/atmos/pkg/terraform/cache"
	rc "github.com/cloudposse/atmos/pkg/terraform/rc"
)

// warnOnConflictingEnvVars inspects the current process environment for variables
// that are known to interfere with Atmos's management of Terraform, and emits a
// warning when any are detected.
func warnOnConflictingEnvVars() {
	warnOnExactVars := []string{"TF_CLI_ARGS", "TF_WORKSPACE"}
	warnOnPrefixVars := []string{"TF_VAR_", "TF_CLI_ARGS_"}

	var problematicVars []string
	for _, envVar := range os.Environ() {
		parts := strings.SplitN(envVar, "=", 2)
		if len(parts) != 2 {
			continue
		}
		if slices.Contains(warnOnExactVars, parts[0]) {
			problematicVars = append(problematicVars, parts[0])
			continue
		}
		for _, prefix := range warnOnPrefixVars {
			if strings.HasPrefix(parts[0], prefix) {
				problematicVars = append(problematicVars, parts[0])
				break
			}
		}
	}

	if len(problematicVars) > 0 {
		log.Warn("Detected environment variables that may interfere with Atmos's control of Terraform",
			"variables", problematicVars)
	}
}

// assembleComponentEnvVars builds the complete list of environment variables for
// the terraform subprocess.  It combines the component env section, standard Atmos
// variables (ATMOS_CLI_CONFIG_PATH, ATMOS_BASE_PATH, TF_IN_AUTOMATION), the
// TF_APPEND_USER_AGENT value, the plugin-cache env, and any toolchain PATH overrides.
func assembleComponentEnvVars(atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo, tenv *dependencies.ToolchainEnvironment) error {
	// Convert ComponentEnvSection (set by auth hooks and stack config) to a list.
	for k, v := range info.ComponentEnvSection {
		info.ComponentEnvList = append(info.ComponentEnvList, fmt.Sprintf("%s=%v", k, v))
	}

	info.ComponentEnvList = append(info.ComponentEnvList,
		fmt.Sprintf("ATMOS_CLI_CONFIG_PATH=%s", atmosConfig.CliConfigPath))

	basePath, err := filepath.Abs(atmosConfig.BasePath)
	if err != nil {
		return err
	}
	info.ComponentEnvList = append(info.ComponentEnvList, fmt.Sprintf("ATMOS_BASE_PATH=%s", basePath))

	// Suppress verbose Terraform instructions in automated environments.
	// https://developer.hashicorp.com/terraform/cli/config/environment-variables#tf_in_automation
	info.ComponentEnvList = append(info.ComponentEnvList, "TF_IN_AUTOMATION=true")

	// Precedence: OS env > atmos.yaml > default (empty/omitted).
	appendUserAgent := atmosConfig.Components.Terraform.AppendUserAgent
	if envUA, exists := os.LookupEnv("TF_APPEND_USER_AGENT"); exists && envUA != "" {
		appendUserAgent = envUA
	}
	if appendUserAgent != "" {
		info.ComponentEnvList = append(info.ComponentEnvList, fmt.Sprintf("TF_APPEND_USER_AGENT=%s", appendUserAgent))
	}

	// Plugin cache directory.
	info.ComponentEnvList = append(info.ComponentEnvList, configurePluginCache(atmosConfig)...)

	// Terraform CLI configuration (.terraformrc) rendered by Atmos and exposed via
	// TF_CLI_CONFIG_FILE / TOFU_CLI_CONFIG_FILE. Also the injection point for the
	// registry cache's network_mirror/host directives. The cleanup closer is stashed
	// on info and run (deferred) after the whole terraform pipeline so the temp file
	// survives every subprocess (init, workspace, plan/apply).
	if err := configureTerraformRC(atmosConfig, info); err != nil {
		return err
	}

	// Toolchain PATH must come last so it takes precedence over all other PATH entries.
	if tenv != nil {
		info.ComponentEnvList = append(info.ComponentEnvList, tenv.EnvVars()...)
	}

	if len(info.ComponentEnvList) > 0 {
		log.Debug("Using ENV vars:")
		for _, v := range info.ComponentEnvList {
			log.Debug(v)
		}
	}

	return appendTerraformSecretEnv(info)
}

// configureTerraformRC builds the effective Terraform CLI-config map from the
// user's components.terraform.rc plus any registry-cache contribution (network
// mirror + module host overrides), renders it to a temp file, and appends
// TF_CLI_CONFIG_FILE / TOFU_CLI_CONFIG_FILE. The cleanup closer is stashed on info
// and deferred at the ExecuteTerraform level so the file outlives every subprocess.
func configureTerraformRC(atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo) error {
	rcMap := map[string]any{}

	if rcCfg := atmosConfig.Components.Terraform.RC; rcCfg != nil && rcCfg.Enabled {
		for k, v := range rcCfg.Config {
			// Deep-copy nested values: mergeCacheContribution mutates the "host" map in
			// place, and a shallow copy would leak cache loopback overrides back into
			// atmosConfig.Components.Terraform.RC.Config for later components/commands in
			// the same process.
			rcMap[k] = cloneRCValue(v)
		}
	}

	if setup, ok := info.TerraformCache.(*tfcache.Setup); ok && setup != nil {
		mergeCacheContribution(rcMap, setup.Contribute())

		// Make the terraform/tofu subprocess trust the proxy's self-signed cert.
		trustEnv, err := setup.TrustEnv()
		if err != nil {
			return err
		}
		info.ComponentEnvList = append(info.ComponentEnvList, trustEnv...)
	}

	if len(rcMap) == 0 {
		return nil
	}

	rcEnv, rcCleanup, err := rc.Generate(rcMap, atmosConfig, info)
	if err != nil {
		return err
	}
	info.ComponentEnvList = append(info.ComponentEnvList, rcEnv...)
	info.RCCleanup = rcCleanup
	return nil
}

// mergeCacheContribution merges the registry cache's CLI-config directives into
// rcMap. The cache owns provider_installation (it must be the single network-mirror
// egress); host overrides are merged per-host so user-declared hosts are preserved.
func mergeCacheContribution(rcMap, contribution map[string]any) {
	const hostKey = "host"
	for key, value := range contribution {
		if key != hostKey {
			rcMap[key] = value
			continue
		}
		add, _ := value.(map[string]any)
		existing, _ := rcMap[hostKey].(map[string]any)
		if existing == nil {
			rcMap[hostKey] = add
			continue
		}
		for host, services := range add {
			existing[host] = services
		}
		rcMap[hostKey] = existing
	}
}

// cloneRCValue deep-copies a CLI-config value so callers can mutate the result without
// affecting the source. Maps and slices are cloned recursively; scalars are returned
// as-is. This protects atmosConfig.Components.Terraform.RC.Config from in-place mutation
// when the registry cache contribution is merged.
func cloneRCValue(v any) any {
	switch val := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(val))
		for k, item := range val {
			out[k] = cloneRCValue(item)
		}
		return out
	case []any:
		out := make([]any, len(val))
		for i, item := range val {
			out[i] = cloneRCValue(item)
		}
		return out
	default:
		return val
	}
}

// appendTerraformSecretEnv adds secrets after environment logging to keep them out of logs.
func appendTerraformSecretEnv(info *schema.ConfigAndStacksInfo) error {
	// Inject secret-bearing variables as TF_VAR_<name> so they reach Terraform via the
	// (transient) process environment instead of the on-disk varfile. Appended AFTER the
	// debug dump above so secret values are never written to logs.
	secretEnv, err := SecretVarEnv(info)
	if err != nil {
		return err
	}
	if len(secretEnv) > 0 {
		info.ComponentEnvList = append(info.ComponentEnvList, secretEnv...)
		log.Debug("Injecting secret variables as TF_VAR_* environment variables", "count", len(secretEnv))
	}

	testSecretEnv, err := terraformTestSecretVarEnv(info)
	if err != nil {
		return err
	}
	if len(testSecretEnv) > 0 {
		info.ComponentEnvList = append(info.ComponentEnvList, testSecretEnv...)
		log.Debug("Injecting secret Terraform test variables as TF_VAR_* environment variables", "count", len(testSecretEnv))
	}

	return nil
}

// resolveTerraformCommand sets info.Command from atmosConfig if not already set.
// Falls back to the cfg.TerraformComponentType default when neither is configured.
func resolveTerraformCommand(atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo) {
	if info.Command != "" {
		return
	}
	if atmosConfig.Components.Terraform.Command != "" {
		info.Command = atmosConfig.Components.Terraform.Command
	} else {
		info.Command = cfg.TerraformComponentType
	}
}

// handleVersionSubcommand executes the `terraform version` command and returns the result.
// It resolves the toolchain binary and delegates directly to the shell, bypassing
// full stack processing.
func (e *Executor) handleVersionSubcommand(atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo) error {
	tenv, err := dependencies.ForComponent(atmosConfig, "terraform", nil, nil)
	if err != nil {
		return err
	}
	return e.services.ExecuteShellCommand(
		*atmosConfig,
		tenv.Resolve(info.Command),
		[]string{info.SubCommand},
		"",
		tenv.EnvVars(),
		false,
		info.RedirectStdErr,
	)
}

func (e *Executor) setupTerraformAuth(atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo) (auth.AuthManager, error) {
	// Log the identity-selection decision point for easy debugging.
	log.Debug("Resolving auth config for component command",
		"stack", info.Stack, "component", info.ComponentFromArg, "subcommand", info.SubCommand)

	// Get merged auth config (global + component-specific if stack/component are set).
	// getMergedAuthConfig logs on debug when falling back to global config after an error.
	mergedAuthConfig, err := e.services.DefaultMergedAuthConfigGetter(atmosConfig, info)
	if err != nil {
		// Propagate ErrInvalidComponent directly — prevents an auth prompt for a nonexistent component.
		if errors.Is(err, errUtils.ErrInvalidComponent) {
			return nil, err
		}
		// Wrap unexpected errors (e.g. MergeComponentAuthFromConfig failures) with the sentinel
		// to match the behaviour of createAndAuthenticateAuthManagerWithDeps. When the error names
		// a missing identity, offer the same profile-selection prompt `atmos auth login` has.
		return nil, e.services.ResolveIdentityConfigError(atmosConfig, info, err, errUtils.ErrInvalidAuthConfig)
	}

	// Create and authenticate the AuthManager using the same injectable creator as
	// createAndAuthenticateAuthManagerWithDeps to keep injection points unified. Carry
	// forward prompted component/stack so a later identity-not-found fallback inside
	// Authenticate can re-inject them into a profile-fallback re-exec.
	authManager, err := e.services.DefaultAuthManagerCreator(
		info.Identity, mergedAuthConfig, cfg.IdentityFlagSelectValue, atmosConfig, auth.ReExecContext{
			Component:         info.ComponentFromArg,
			ComponentPrompted: info.ComponentPrompted,
			Stack:             info.Stack,
			StackPrompted:     info.StackPrompted,
		},
	)
	if err != nil {
		if errors.Is(err, errUtils.ErrUserAborted) {
			errUtils.Exit(errUtils.ExitCodeSIGINT)
		}
		// Wrap auth creation failures with the sentinel to match createAndAuthenticateAuthManagerWithDeps.
		// When the error names a missing identity, offer the same profile-selection prompt
		// `atmos auth login` has.
		return nil, e.services.ResolveIdentityConfigError(atmosConfig, info, err, errUtils.ErrFailedToInitializeAuthManager)
	}

	// Store manager for nested YAML functions (e.g. !terraform.state).
	info.AuthManager = authManager

	// The manager is created with its own empty stackInfo; thread the target stack
	// through so emulator identities can resolve the running emulator's endpoint
	// when stores (`!store`, `!secret`, hooks) build their in-process auth context.
	// authManager may be nil when no identity/auth is configured.
	if authManager != nil {
		if si := authManager.GetStackInfo(); si != nil && si.Stack == "" {
			si.Stack = info.Stack
		}
	}

	e.injectTerraformStoreAuthResolver(atmosConfig, info, authManager)

	return authManager, nil
}

func (e *Executor) injectTerraformStoreAuthResolver(atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo, authManager auth.AuthManager) {
	if authManager == nil {
		return
	}

	// Persist the auto-detected identity so downstream hooks, nested operations,
	// and stores without their own identity use the same authenticated principal.
	e.services.StoreAutoDetectedIdentity(authManager, info)

	resolver := authbridge.NewResolver(authManager, info)
	defaultIdentity := resolveStoreDefaultIdentity(info.Identity, authManager)
	atmosConfig.Stores.SetAuthContextResolverWithDefaultIdentity(resolver, defaultIdentity)

	// Also expose the resolver to cloud-KMS SOPS providers so `!secret` resolution during terraform
	// authenticates KMS decrypt as the component's effective identity (issue #2637).
	atmosConfig.SecretsAuth = &store.SecretsAuthContext{
		Resolver:        resolver,
		DefaultIdentity: defaultIdentity,
	}
}

func storeDefaultIdentity(identity string) string {
	switch identity {
	case "", cfg.IdentityFlagSelectValue, cfg.IdentityFlagDisabledValue:
		return ""
	default:
		return identity
	}
}

// resolveStoreDefaultIdentity returns the identity that stores without their own
// `identity:` should authenticate as. An explicit `--identity`/`ATMOS_IDENTITY`
// wins; otherwise we fall back to the stack's `default: true` identity so
// in-process store/secret reads use the same principal (and emulator endpoint) as
// the Terraform subprocess. When there is no default identity (or auth is
// disabled), this returns "" — stores then use the AWS/GCP/Azure default
// credential chain exactly as before (back-compat for ambient real-cloud creds).
func resolveStoreDefaultIdentity(identity string, authManager auth.AuthManager) string {
	if identity == cfg.IdentityFlagDisabledValue {
		return ""
	}
	if explicit := storeDefaultIdentity(identity); explicit != "" {
		return explicit
	}
	if authManager == nil {
		return ""
	}
	defaultIdentity, err := authManager.GetDefaultIdentity(false)
	if err != nil {
		return ""
	}
	return defaultIdentity
}

// HookStoreDefaultIdentity resolves the identity that identity-less stores invoked by terraform
// lifecycle hooks (e.g. the after-apply `store-outputs` hook) should inherit. It mirrors the main
// terraform path (injectTerraformStoreAuthResolver): it auto-detects the active identity from the
// auth manager's chain when info carries no explicit identity, then normalizes empty/select/disabled
// to "" (meaning "no inheritance — keep ambient/default SDK credentials"). It returns "" when no auth
// manager is present, preserving the prior behavior for runs without Atmos auth.
//
// The hook executes in a freshly loaded config (prepareHookContext), so it must re-derive this from
// the persisted auth manager rather than relying on the apply-phase config.
func (e *Executor) HookStoreDefaultIdentity(authManager auth.AuthManager, info *schema.ConfigAndStacksInfo) string {
	defer perf.Track(nil, "exec.HookStoreDefaultIdentity")()

	if authManager == nil || info == nil {
		return ""
	}
	e.services.StoreAutoDetectedIdentity(authManager, info)
	return storeDefaultIdentity(info.Identity)
}

// SetupTerraformAuthForCLI exposes terraform auth setup to command-layer callers
// that need the same merged-auth and explicit-identity behavior as ExecuteTerraform.
func (e *Executor) SetupTerraformAuthForCLI(atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo) (any, error) {
	defer perf.Track(atmosConfig, "exec.SetupTerraformAuthForCLI")()

	return e.setupTerraformAuth(atmosConfig, info)
}

// SetupComponentAuthForCLI exposes the shared component auth setup to non-Terraform
// command layers that still need authenticated YAML functions such as !terraform.state.
func (e *Executor) SetupComponentAuthForCLI(atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo) (auth.AuthManager, error) {
	defer perf.Track(atmosConfig, "exec.SetupComponentAuthForCLI")()

	return e.setupTerraformAuth(atmosConfig, info)
}
