package exec

// terraform_execute_helpers.go contains helper functions extracted from ExecuteTerraform
// to reduce cyclomatic complexity and improve testability.
// Each function handles one discrete responsibility of the terraform execution pipeline.

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	errUtils "github.com/cloudposse/atmos/errors"
	auth "github.com/cloudposse/atmos/pkg/auth"
	"github.com/cloudposse/atmos/pkg/component"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/dependencies"
	atmosio "github.com/cloudposse/atmos/pkg/io"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/provisioner"
	_ "github.com/cloudposse/atmos/pkg/provisioner/lock"   // register after.terraform.init providers-lock provisioner
	_ "github.com/cloudposse/atmos/pkg/provisioner/source" // register source provisioner
	provWorkdir "github.com/cloudposse/atmos/pkg/provisioner/workdir"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/store/authbridge"
	tfcache "github.com/cloudposse/atmos/pkg/terraform/cache"
	tfgenerate "github.com/cloudposse/atmos/pkg/terraform/generate"
	"github.com/cloudposse/atmos/pkg/terraform/rc"
	"github.com/cloudposse/atmos/pkg/terraform/tfvars"
	u "github.com/cloudposse/atmos/pkg/utils"
)

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
func handleVersionSubcommand(atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo) error {
	tenv, err := dependencies.ForComponent(atmosConfig, "terraform", nil, nil)
	if err != nil {
		return err
	}
	return ExecuteShellCommand(
		*atmosConfig,
		tenv.Resolve(info.Command),
		[]string{info.SubCommand},
		"",
		tenv.EnvVars(),
		false,
		info.RedirectStdErr,
	)
}

// setupTerraformAuth builds the merged auth config (global + component-specific via
// getMergedAuthConfig), creates and authenticates the AuthManager, stores the resolved
// identity back into info, and injects an auth resolver into the Atmos store registry.
//
// getMergedAuthConfig is the shared helper (utils_auth.go) that handles the
// component config fetch, debug logging on fallback, and the ErrInvalidComponent
// short-circuit. Using it here eliminates duplication and keeps both code paths in sync.
//
// The defaultAuthManagerCreator injectable var (utils_auth.go) is used so tests can
// substitute a fake creator without needing a separate var in this file.
// The defaultMergedAuthConfigGetter injectable var below allows tests to exercise the
// ErrInvalidAuthConfig wrap branch without requiring a real stack or component.

// defaultMergedAuthConfigGetter is the injectable function for getMergedAuthConfig.
// Overriding it in tests allows exercising error branches that are otherwise only
// reachable via MergeComponentAuthFromConfig failures (hard to trigger in unit tests).
//
// Note: overriding this var also bypasses the defaultComponentConfigFetcher layer
// (utils_auth.go), which is one level deeper. The two vars target different injection
// points: defaultComponentConfigFetcher injects at the component-fetch level (used for
// ErrInvalidComponent), whereas defaultMergedAuthConfigGetter injects at the whole getter
// level (used for ErrInvalidAuthConfig). Do not override both simultaneously — the deeper
// var is shadowed and its effect would be masked.
var defaultMergedAuthConfigGetter = getMergedAuthConfig

func setupTerraformAuth(atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo) (auth.AuthManager, error) {
	// Log the identity-selection decision point for easy debugging.
	log.Debug("Resolving auth config for component command",
		"stack", info.Stack, "component", info.ComponentFromArg, "subcommand", info.SubCommand)

	// Get merged auth config (global + component-specific if stack/component are set).
	// getMergedAuthConfig logs on debug when falling back to global config after an error.
	mergedAuthConfig, err := defaultMergedAuthConfigGetter(atmosConfig, info)
	if err != nil {
		// Propagate ErrInvalidComponent directly — prevents an auth prompt for a nonexistent component.
		if errors.Is(err, errUtils.ErrInvalidComponent) {
			return nil, err
		}
		// Wrap unexpected errors (e.g. MergeComponentAuthFromConfig failures) with the sentinel
		// to match the behaviour of createAndAuthenticateAuthManagerWithDeps.
		return nil, fmt.Errorf("%w: %w", errUtils.ErrInvalidAuthConfig, err)
	}

	// Create and authenticate the AuthManager using the same injectable creator as
	// createAndAuthenticateAuthManagerWithDeps to keep injection points unified.
	authManager, err := defaultAuthManagerCreator(
		info.Identity, mergedAuthConfig, cfg.IdentityFlagSelectValue, atmosConfig,
	)
	if err != nil {
		if errors.Is(err, errUtils.ErrUserAborted) {
			errUtils.Exit(errUtils.ExitCodeSIGINT)
		}
		// Wrap auth creation failures with the sentinel to match createAndAuthenticateAuthManagerWithDeps.
		return nil, fmt.Errorf("%w: %w", errUtils.ErrFailedToInitializeAuthManager, err)
	}

	// Store manager for nested YAML functions (e.g. !terraform.state).
	info.AuthManager = authManager

	injectTerraformStoreAuthResolver(atmosConfig, info, authManager)

	return authManager, nil
}

func injectTerraformStoreAuthResolver(atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo, authManager auth.AuthManager) {
	if authManager == nil {
		return
	}

	// Persist the auto-detected identity so downstream hooks, nested operations,
	// and stores without their own identity use the same authenticated principal.
	storeAutoDetectedIdentity(authManager, info)

	resolver := authbridge.NewResolver(authManager, info)
	atmosConfig.Stores.SetAuthContextResolverWithDefaultIdentity(resolver, storeDefaultIdentity(info.Identity))
}

func storeDefaultIdentity(identity string) string {
	switch identity {
	case "", cfg.IdentityFlagSelectValue, cfg.IdentityFlagDisabledValue:
		return ""
	default:
		return identity
	}
}

// SetupTerraformAuthForCLI exposes terraform auth setup to command-layer callers
// that need the same merged-auth and explicit-identity behavior as ExecuteTerraform.
func SetupTerraformAuthForCLI(atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo) (any, error) {
	defer perf.Track(atmosConfig, "exec.SetupTerraformAuthForCLI")()

	return setupTerraformAuth(atmosConfig, info)
}

// SetupComponentAuthForCLI exposes the shared component auth setup to non-Terraform
// command layers that still need authenticated YAML functions such as !terraform.state.
func SetupComponentAuthForCLI(atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo) (auth.AuthManager, error) {
	defer perf.Track(atmosConfig, "exec.SetupComponentAuthForCLI")()

	return setupTerraformAuth(atmosConfig, info)
}

// resolveAndProvisionComponentPath resolves the filesystem path for a terraform component,
// optionally auto-generates files, performs JIT source provisioning, and validates
// that the resulting directory actually exists.
func resolveAndProvisionComponentPath(atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo) (string, error) {
	componentPath, err := u.GetComponentPath(atmosConfig, "terraform", info.ComponentFolderPrefix, info.FinalComponent)
	if err != nil {
		return "", fmt.Errorf("failed to resolve component path: %w", err)
	}

	// Provision source before generating files: when provision.workdir.enabled
	// is true the resolved path is the workdir, and generated files must land
	// there rather than in the base component directory.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	componentPath, componentPathExists, err := component.ProvisionAndResolveComponentPath(
		ctx, atmosConfig, info, cfg.TerraformComponentType, componentPath,
	)
	if err != nil {
		return "", err
	}

	if err = autoGenerateComponentFiles(atmosConfig, info, componentPath); err != nil {
		return "", err
	}

	if !componentPathExists {
		basePath, _ := u.GetComponentBasePath(atmosConfig, cfg.TerraformComponentType)
		return "", fmt.Errorf(
			"%w: '%s' points to the Terraform component '%s', but it does not exist in '%s'",
			errUtils.ErrInvalidTerraformComponent,
			info.ComponentFromArg,
			info.FinalComponent,
			basePath,
		)
	}

	return componentPath, nil
}

// autoGenerateComponentFiles creates the component directory and generates source files
// when AutoGenerateFiles is enabled and a generate section is present.
func autoGenerateComponentFiles(atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo, componentPath string) error {
	if !atmosConfig.Components.Terraform.AutoGenerateFiles || info.DryRun {
		return nil
	}
	generateSection := tfgenerate.GetGenerateSectionFromComponent(info.ComponentSection)
	if generateSection == nil {
		return nil
	}
	if mkdirErr := os.MkdirAll(componentPath, dirPermissions); mkdirErr != nil {
		return errors.Join(errUtils.ErrCreateDirectory, fmt.Errorf("auto-generation: %w", mkdirErr))
	}
	return GenerateFilesForComponent(atmosConfig, info, componentPath)
}

// checkComponentRestrictions returns an error when the requested subcommand is not
// permitted for the component due to its metadata (abstract, locked) or the configured
// backend type (HTTP backend does not support workspaces).
func checkComponentRestrictions(info *schema.ConfigAndStacksInfo) error {
	// Abstract components cannot be provisioned.
	if info.ComponentIsAbstract {
		switch info.SubCommand {
		case "plan", subcommandApply, subcommandDeploy, subcommandWorkspace:
			return fmt.Errorf(
				"%w: the component '%s' cannot be provisioned because it's marked as abstract (metadata.type: abstract)",
				errUtils.ErrAbstractComponentCantBeProvisioned,
				filepath.Join(info.ComponentFolderPrefix, info.Component),
			)
		}
	}

	// Locked components may not be mutated.
	if info.ComponentIsLocked {
		switch info.SubCommand {
		case subcommandApply, "deploy", "destroy", "import", "state", "taint", "untaint":
			return fmt.Errorf(
				"%w: component '%s' cannot be modified (metadata.locked: true)",
				errUtils.ErrLockedComponentCantBeProvisioned,
				filepath.Join(info.ComponentFolderPrefix, info.Component),
			)
		}
	}

	// HTTP backend does not support workspace commands.
	if info.SubCommand == subcommandWorkspace && info.ComponentBackendType == "http" {
		return errUtils.ErrHTTPBackendWorkspaces
	}

	return nil
}

// printAndWriteVarFiles logs component variables and, when not using a pre-existing
// plan file, writes them to the varfile on disk (path derived from atmosConfig+info).
// Workspace subcommands do not use varfiles and are skipped entirely.
func printAndWriteVarFiles(atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo) error {
	if info.SubCommand == subcommandWorkspace {
		return nil
	}

	if err := logAndWriteComponentVars(atmosConfig, info); err != nil {
		return err
	}

	return logCliVarsOverrides(atmosConfig, info)
}

// computeTerraformSecretVarKeys records the set of top-level variable keys whose value
// contains a resolved secret, so they can be kept out of the on-disk varfile and injected
// as TF_VAR_<name> environment variables instead. It must run after secret resolution
// (ProcessStacks) and before auth credentials are registered with the masker, so that the
// varfile-write and env-assembly steps partition the variables identically.
func computeTerraformSecretVarKeys(info *schema.ConfigAndStacksInfo) {
	_, secret := tfvars.Partition(info.ComponentVarsSection, atmosio.ContainsSecret)
	if len(secret) == 0 {
		info.TerraformSecretVarKeys = nil
		return
	}
	keys := make(map[string]bool, len(secret))
	for k := range secret {
		keys[k] = true
	}
	info.TerraformSecretVarKeys = keys
}

// diskSafeVars returns ComponentVarsSection with secret-bearing top-level keys removed,
// so resolved secrets are never written to the on-disk varfile. Returns the original map
// unchanged when no secret-bearing variables were detected.
func diskSafeVars(info *schema.ConfigAndStacksInfo) map[string]any {
	if len(info.TerraformSecretVarKeys) == 0 {
		return info.ComponentVarsSection
	}
	safe := make(map[string]any, len(info.ComponentVarsSection))
	for k, v := range info.ComponentVarsSection {
		if info.TerraformSecretVarKeys[k] {
			continue
		}
		safe[k] = v
	}
	return safe
}

// secretVarEnv returns the TF_VAR_<name> environment entries for the component's
// secret-bearing variables, for injection into the subprocess environment.
func secretVarEnv(info *schema.ConfigAndStacksInfo) ([]string, error) {
	if len(info.TerraformSecretVarKeys) == 0 {
		return nil, nil
	}
	secret := make(map[string]any, len(info.TerraformSecretVarKeys))
	for k := range info.TerraformSecretVarKeys {
		if v, ok := info.ComponentVarsSection[k]; ok {
			secret[k] = v
		}
	}
	return tfvars.SecretEnv(secret)
}

// logAndWriteComponentVars logs component variables and writes the varfile to disk
// when not using a pre-existing plan. Secret-bearing variables are excluded from the
// varfile (they are injected as TF_VAR_<name> env vars in assembleComponentEnvVars).
func logAndWriteComponentVars(atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo) error {
	log.Debug("Variables for the component in the stack", logFieldComponent, info.ComponentFromArg, "stack", info.Stack)
	if atmosConfig.Logs.Level == u.LogLevelTrace || atmosConfig.Logs.Level == u.LogLevelDebug {
		if err := u.PrintAsYAMLToFileDescriptor(atmosConfig, info.ComponentVarsSection); err != nil {
			return err
		}
	}

	if !info.UseTerraformPlan {
		varFilePath := constructTerraformComponentVarfilePath(atmosConfig, info)
		log.Debug("Writing the variables", "file", varFilePath)
		if !info.DryRun {
			if err := u.WriteToFileAsJSON(varFilePath, diskSafeVars(info), filePermissions); err != nil {
				return err
			}
		}
	}
	return nil
}

// logCliVarsOverrides logs CLI variable overrides when present at debug/trace level.
func logCliVarsOverrides(atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo) error {
	cliVars, ok := info.ComponentSection[cfg.TerraformCliVarsSectionName].(map[string]any)
	if !ok || len(cliVars) == 0 {
		return nil
	}
	log.Debug("CLI variables (will override the variables defined in the stack manifests):")
	if atmosConfig.Logs.Level == u.LogLevelTrace || atmosConfig.Logs.Level == u.LogLevelDebug {
		if err := u.PrintAsYAMLToFileDescriptor(atmosConfig, cliVars); err != nil {
			return err
		}
	}
	return nil
}

// validateTerraformComponent runs OPA/JSON-schema validation policies against the
// component's stack configuration section and returns an error if validation fails.
func validateTerraformComponent(atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo) error {
	valid, err := ValidateComponent(
		atmosConfig,
		info.ComponentFromArg,
		info.ComponentSection,
		"", "", nil, 0,
	)
	if err != nil {
		return err
	}
	if !valid {
		return fmt.Errorf("%w: the component '%s' did not pass the validation policies",
			errUtils.ErrInvalidComponent, info.ComponentFromArg)
	}
	return nil
}

// generateConfigFiles writes the backend configuration, generated files, and
// provider overrides for the component into the working directory.
//
// NOTE: GenerateFilesForComponent is also called by autoGenerateComponentFiles
// (inside resolveAndProvisionComponentPath) when AutoGenerateFiles=true. That call
// handles the generate: section from stack config, while this call handles the
// standard backend/provider override files. The two calls serve different purposes
// and both are needed. This is pre-existing behavior from the original terraform.go.
func generateConfigFiles(atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo, workingDir string) error {
	if err := generateBackendConfig(atmosConfig, info, workingDir); err != nil {
		return err
	}
	if err := GenerateFilesForComponent(atmosConfig, info, workingDir); err != nil {
		return err
	}
	if err := generateProviderOverrides(atmosConfig, info, workingDir); err != nil {
		return err
	}
	// Generate required_providers (terraform_override.tf.json) for version pinning (DEV-3124).
	return generateRequiredProviders(atmosConfig, info, workingDir)
}

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
		if u.SliceContainsString(warnOnExactVars, parts[0]) {
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

	// Inject secret-bearing variables as TF_VAR_<name> so they reach Terraform via the
	// (transient) process environment instead of the on-disk varfile. Appended AFTER the
	// debug dump above so secret values are never written to logs.
	secretEnv, err := secretVarEnv(info)
	if err != nil {
		return err
	}
	if len(secretEnv) > 0 {
		info.ComponentEnvList = append(info.ComponentEnvList, secretEnv...)
		log.Debug("Injecting secret variables as TF_VAR_* environment variables", "count", len(secretEnv))
	}

	return nil
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

// shouldRunTerraformInit returns true when a `terraform init` should be executed as a
// pre-step before the main command.  Init is skipped when: the subcommand is init
// itself (init runs as the main command), deploy with DeployRunInit=false is configured,
// or the caller passed the --skip-init flag.
func shouldRunTerraformInit(atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo) bool {
	if info.SubCommand == subcommandInit {
		return false
	}
	if info.SubCommand == subcommandDeploy && !atmosConfig.Components.Terraform.DeployRunInit {
		return false
	}
	if info.SkipInit {
		log.Debug("Skipping over 'terraform init' due to '--skip-init' flag being passed")
		return false
	}
	return true
}

// buildInitArgs constructs the argument list for `terraform init`.
//
// For non-workdir components, -reconfigure is added when:
//   - the component uses the workspace subcommand, or
//   - InitRunReconfigure is explicitly enabled in atmos.yaml.
//
// For workdir components, InitRunReconfigure is intentionally ignored when the workdir
// was not re-provisioned this invocation. The backend configuration for workdir
// components is always generated deterministically from the same stack config, so it
// never changes between runs of a preserved workdir. When -reconfigure is combined
// with existing workspace state directories (terraform.tfstate.d/), OpenTofu treats
// init as a fresh backend initialization and prompts "Do you want to migrate all
// workspaces?" — even when the backend is unchanged. The correct signal to add
// -reconfigure for workdir components is WorkdirReprovisionedKey, which is set only
// when the workdir was actually wiped and re-downloaded (TTL expired or TTL=0s).
func buildInitArgs(atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo, varFile string) []string {
	_, hasWorkdir := info.ComponentSection[provWorkdir.WorkdirPathKey].(string)
	_, wasReprovisioned := info.ComponentSection[provWorkdir.WorkdirReprovisionedKey]

	var useReconfigure bool
	if hasWorkdir {
		// Workdir component: only reconfigure when the workdir was actually wiped.
		useReconfigure = wasReprovisioned || info.SubCommand == subcommandWorkspace
	} else {
		// Non-workdir component: honour global InitRunReconfigure setting.
		useReconfigure = info.SubCommand == subcommandWorkspace || atmosConfig.Components.Terraform.InitRunReconfigure
	}

	if useReconfigure {
		if atmosConfig.Components.Terraform.Init.PassVars {
			return []string{subcommandInit, "-reconfigure", varFileFlag, varFile}
		}
		return []string{subcommandInit, "-reconfigure"}
	}
	if atmosConfig.Components.Terraform.Init.PassVars {
		return []string{subcommandInit, varFileFlag, varFile}
	}
	return []string{"init"}
}

// prepareInitExecution performs the pre-init housekeeping:
//  1. Deletes the .terraform/environment file so Terraform doesn't prompt for workspace selection
//     (skipped for workdir-enabled components — see note below).
//  2. Executes all provisioners registered for the before.terraform.init hook event.
//  3. Returns the effective component path (which may be overridden by a workdir provisioner).
//
// NOTE on cleanTerraformWorkspace and workdir components:
// cleanTerraformWorkspace was designed to prevent workspace-selection prompts when different
// backends are used for the same component across runs.  For workdir-enabled components the
// backend configuration is always consistent (generated fresh from the same stack config),
// so deleting .terraform/environment is not only unnecessary — it is actively harmful:
// when -reconfigure or init_run_reconfigure is also used, OpenTofu sees workspace state
// directories (terraform.tfstate.d/) but no .terraform/environment file and interprets the
// situation as a backend migration, producing the "Do you want to migrate all workspaces?"
// prompt on every apply.  Skipping the cleanup for workdir components avoids this.
func prepareInitExecution(atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo, componentPath string) (string, error) {
	_, isWorkdir := info.ComponentSection[provWorkdir.WorkdirPathKey].(string)
	if !isWorkdir {
		cleanTerraformWorkspace(*atmosConfig, componentPath)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	if err := provisioner.ExecuteProvisioners(
		ctx,
		provisioner.HookEvent(beforeTerraformInitEvent),
		atmosConfig,
		info.ComponentSection,
		info.AuthContext,
	); err != nil {
		return componentPath, fmt.Errorf("provisioner execution failed: %w", err)
	}

	if workdirPath, ok := info.ComponentSection[provWorkdir.WorkdirPathKey].(string); ok && workdirPath != "" {
		log.Debug("Using workdir path for terraform command", "workdirPath", workdirPath)
		return workdirPath, nil
	}

	return componentPath, nil
}

// executeTerraformInitPhase runs `terraform init` as a pre-step before the main command.
// It prepares the init execution environment, builds the init args, and delegates to
// ExecuteShellCommand.  Returns the (possibly updated) component path.
//
// MUTUAL EXCLUSION CONTRACT: this function is called ONLY when shouldRunTerraformInit()
// returns true (i.e. SubCommand ≠ "init").  For the "init" subcommand itself,
// buildInitSubcommandArgs in terraform_execute_helpers_args.go handles the provisioner
// invocation via prepareInitExecution.  These two code paths must never both execute
// in the same command invocation or provisioners will run twice.
func executeTerraformInitPhase(atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo, componentPath, varFile string, opts ...ShellCommandOption) (string, error) {
	newPath, err := prepareInitExecution(atmosConfig, info, componentPath)
	if err != nil {
		return componentPath, err
	}

	initArgs := buildInitArgs(atmosConfig, info, varFile)
	err = executeShellCommandWithRetry(
		atmosConfig,
		info,
		"init",
		func(o ...ShellCommandOption) error {
			return ExecuteShellCommand(
				*atmosConfig,
				info.Command,
				initArgs,
				newPath,
				info.ComponentEnvList,
				info.DryRun,
				info.RedirectStdErr,
				o...,
			)
		},
		opts...,
	)
	if err != nil {
		return newPath, err
	}

	dispatchAfterInit(atmosConfig, info, newPath)

	return newPath, nil
}

// dispatchAfterInit fires the after.terraform.init provisioners (e.g. the multi-platform
// providers-lock hook) once init has succeeded. It hands them a TerraformExecContext whose
// runner reuses the same binary, env (incl. TF_CLI_CONFIG_FILE pointing at the live proxy),
// and working directory as init, so a `providers lock` runs against the already-warm cache.
// Lock completion is best-effort: a failure is logged, not propagated, so it never fails the
// user's plan/apply.
func dispatchAfterInit(atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo, componentPath string) {
	execCtx := &provisioner.TerraformExecContext{
		WorkingDir: componentPath,
		Run: func(args []string) error {
			return ExecuteShellCommand(
				*atmosConfig,
				info.Command,
				args,
				componentPath,
				info.ComponentEnvList,
				info.DryRun,
				info.RedirectStdErr,
			)
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	if err := provisioner.ExecuteProvisioners(
		ctx,
		provisioner.HookEvent(afterTerraformInitEvent),
		atmosConfig,
		info.ComponentSection,
		info.AuthContext,
		execCtx,
	); err != nil {
		log.Warn("Failed to complete multi-platform provider lock", "error", err)
	}
}

// handleDeploySubcommand converts `deploy` into `apply` and ensures -auto-approve is
// added when appropriate.  When ApplyAutoApprove is set in atmos.yaml, it is also
// applied to plain `apply` subcommands.
func handleDeploySubcommand(atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo) {
	if info.SubCommand == subcommandDeploy {
		info.SubCommand = subcommandApply
		if !info.UseTerraformPlan && !u.SliceContainsString(info.AdditionalArgsAndFlags, autoApproveFlag) {
			info.AdditionalArgsAndFlags = append(info.AdditionalArgsAndFlags, autoApproveFlag)
		}
	}

	if info.SubCommand == subcommandApply && atmosConfig.Components.Terraform.ApplyAutoApprove && !info.UseTerraformPlan {
		if !u.SliceContainsString(info.AdditionalArgsAndFlags, autoApproveFlag) {
			info.AdditionalArgsAndFlags = append(info.AdditionalArgsAndFlags, autoApproveFlag)
		}
	}
}

// logTerraformContext emits a debug log line with the full execution context
// (executable, command, component, stack, flags, working directory, inheritance chain).
func logTerraformContext(info *schema.ConfigAndStacksInfo, workingDir string) {
	command := info.SubCommand
	if info.SubCommand2 != "" {
		command = fmt.Sprintf("%s %s", info.SubCommand, info.SubCommand2)
	}

	var inheritance string
	if len(info.ComponentInheritanceChain) > 0 {
		inheritance = info.ComponentFromArg + " -> " + strings.Join(info.ComponentInheritanceChain, " -> ")
	}

	log.Debug(
		"Terraform context",
		"executable", info.Command,
		"command", command,
		logFieldComponent, info.ComponentFromArg,
		"stack", info.StackFromArg,
		"arguments and flags", info.AdditionalArgsAndFlags,
		"terraform component", info.BaseComponentPath,
		"inheritance", inheritance,
		"working directory", workingDir,
	)
}
