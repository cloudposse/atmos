package terraform

import (
	"context"
	"encoding/hex"
	"fmt"
	"hash/fnv"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/component"
	"github.com/cloudposse/atmos/pkg/component/configuration"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/dependencies"
	"github.com/cloudposse/atmos/pkg/provisioner"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// warnConfigurationDryRun explains which plugin metadata a dry-run preview omits.
func warnConfigurationDryRun(info *schema.ConfigAndStacksInfo) {
	if info.DryRun && schema.ConfigurationOwned(info.ComponentSettingsSection) {
		ui.Warning("Dry-run preview may omit Terragrunt extra arguments, required var files, and environment variables because Terragrunt configuration is not evaluated")
	}
}

// resolvePreparedComponentPath keeps reviewed plans on their retained workdir.
func (e *Executor) resolvePreparedComponentPath(ctx context.Context, writers provisioner.OutputWriters, atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo, prepared *schema.PreparedComponentConfiguration) (string, error) {
	if prepared != nil && usesConfigurationPlan(info) {
		// Provisioning could replace the reviewed plan's modules.
		return ConstructTerraformComponentWorkingDir(atmosConfig, info), nil
	}
	return e.resolveAndProvisionComponentPath(ctx, writers, atmosConfig, info)
}

// resolveComponentToolchain preserves an environment already selected during preparation.
func resolveComponentToolchain(config *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo, shouldProcess bool, environment *dependencies.ToolchainEnvironment) (*dependencies.ToolchainEnvironment, error) {
	if !shouldProcess || environment != nil {
		return environment, nil
	}
	environment, err := resolveAndInstallToolchainDeps(config, info)
	if err != nil {
		return nil, err
	}
	info.Command = environment.Resolve(info.Command)
	return environment, nil
}

// writePreparedConfigurationFiles validates generated paths before writing a live preparation.
func writePreparedConfigurationFiles(info *schema.ConfigAndStacksInfo, prepared *schema.PreparedComponentConfiguration, componentPath string) error {
	if prepared == nil || info.DryRun || (usesConfigurationPlan(info) && restoresConfigurationPlan(info)) {
		return nil
	}
	if err := rejectRuntimeArtifacts(prepared.Files); err != nil {
		return err
	}
	if err := configuration.WriteFiles(componentPath, prepared.Files); err != nil {
		return err
	}
	// Prepared files can introduce sensitive declarations at an already inspected path.
	loadTerraformModuleInfo(info, componentPath)
	return nil
}

func prepareConfiguration(ctx context.Context, config *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo) (*schema.PreparedComponentConfiguration, error) {
	if !schema.ConfigurationOwned(info.ComponentSettingsSection) {
		return nil, nil
	}
	if config.ComponentConfigurations == nil {
		return nil, errUtils.ErrPluginUnsupportedExtension
	}
	if err := checkComponentRestrictions(info); err != nil {
		return nil, err
	}
	if info.DryRun {
		return nil, nil
	}
	if err := resolveHostExecutable(info); err != nil {
		return nil, err
	}
	prepared, execution, err := prepareOrRestore(ctx, config, info)
	if err != nil || prepared == nil {
		return prepared, err
	}
	info.ComponentVarsSection = prepared.Inputs
	info.ComponentExecution = execution
	info.ConfigurationSensitiveInputKeys = maps.Clone(prepared.SensitiveInputKeys)
	info.ComponentBackendType, info.ComponentBackendSection = prepared.BackendType, prepared.Backend
	info.TerraformWorkspace = prepared.Workspace
	info.ComponentSection["vars"] = prepared.Inputs
	source, _ := info.ComponentSection["source"].(map[string]any)
	source = maps.Clone(source)
	if source == nil {
		source = make(map[string]any)
	}
	source["uri"] = prepared.Source
	if isLocalModuleSource(prepared.Source) {
		// A local module changes without its location changing, so the retained workdir
		// would keep the code of the first run. Refreshing is a cheap local copy.
		source["ttl"] = "0s"
	}
	info.ComponentSection["source"] = source
	info.ComponentSection["backend_type"] = prepared.BackendType
	info.ComponentSection["backend"] = prepared.Backend
	config.Components.Terraform.AutoGenerateBackendFile = !prepared.BackendGenerated
	return prepared, nil
}

// resolveHostExecutable makes info.Command the absolute path of the selected executable.
func resolveHostExecutable(info *schema.ConfigAndStacksInfo) error {
	// Plugins do not inherit PATH. Resolve the selected host executable before
	// Terragrunt evaluates version constraints or dependency outputs.
	binary, err := exec.LookPath(info.Command)
	if err != nil {
		return fmt.Errorf("%w: Terraform executable: %w", errUtils.ErrPluginInvalidConfig, err)
	}
	binary, err = filepath.Abs(binary)
	if err != nil {
		return fmt.Errorf("%w: %w", errUtils.ErrPluginInvalidConfig, err)
	}
	info.Command = binary
	return nil
}

// prepareOrRestore prepares the component, or restores the reviewed preparation of a saved
// plan. It also returns the hooks and extra arguments the invocation runs: a saved plan was
// prepared for plan, so it runs the apply-time metadata recorded with it.
func prepareOrRestore(ctx context.Context, config *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo) (*schema.PreparedComponentConfiguration, *schema.ComponentExecution, error) {
	if !usesConfigurationPlan(info) {
		prepared, err := config.ComponentConfigurations.Prepare(ctx, info)
		if err != nil || prepared == nil {
			return prepared, nil, err
		}
		return prepared, prepared.Execution, nil
	}
	if err := useRetainedWorkdir(config, info); err != nil {
		return nil, nil, err
	}
	prepared, err := restoreConfigurationPlan(ctx, config, info)
	if err != nil || prepared == nil {
		return prepared, nil, err
	}
	return prepared, prepared.Execution.ForSavedPlanApply(), nil
}

// isLocalModuleSource reports whether the evaluated module source is a path on this machine.
func isLocalModuleSource(uri string) bool {
	return strings.HasPrefix(uri, ".") || filepath.IsAbs(uri)
}

// useRetainedWorkdir points the invocation at the directory the reviewed plan was created in.
// A new process has not provisioned anything, so the path is derived the way provisioning does.
func useRetainedWorkdir(config *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo) error {
	fallback, err := u.GetComponentPath(config, cfg.TerraformComponentType, info.ComponentFolderPrefix, info.FinalComponent)
	if err != nil {
		return fmt.Errorf("failed to resolve component path: %w", err)
	}
	_, err = component.ResolveRetainedWorkdir(config, info, cfg.TerraformComponentType, fallback)
	return err
}

func configurationArtifactName(info *schema.ConfigAndStacksInfo, suffix string) (string, bool) {
	if !schema.ConfigurationOwned(info.ComponentSettingsSection) {
		return "", false
	}
	component := info.ComponentFromArg
	if component == "" {
		// Callers that resolve a component without CLI arguments still name distinct files.
		component = info.Component
	}
	return "atmos-" + configurationIdentity(info.Stack, component) + suffix, true
}

// configurationIdentity names a stack and component pair for artifact files. The name must be the same in
// every process (a plan in one run is applied by another), so it is a plain content digest with no secret.
// It identifies a component for file naming only: it is not a credential hash and gives no security
// guarantee, which is why a non-cryptographic 128-bit FNV-1a digest is used instead of SHA-256.
func configurationIdentity(stack, component string) string {
	digest := fnv.New128a()
	// Writing to a hash never fails.
	_, _ = digest.Write([]byte(stack + "\x00" + component))
	return hex.EncodeToString(digest.Sum(nil))
}

// prepareConfigurationEnvironment applies the same authentication, RC, cache,
// and toolchain environment as Terraform before the evaluator can spawn children.
func (e *Executor) prepareConfigurationEnvironment(ctx context.Context, config *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo, shouldProcess bool) (*dependencies.ToolchainEnvironment, error) {
	if !schema.ConfigurationOwned(info.ComponentSettingsSection) || info.DryRun {
		return nil, nil
	}
	if err := checkComponentRestrictions(info); err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := e.prepareAuthEnvironmentFn(config, info); err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	var tenv *dependencies.ToolchainEnvironment
	if shouldProcess {
		var err error
		tenv, err = e.resolveConfigurationToolsFn(config, info)
		if err != nil {
			return nil, err
		}
		info.Command = tenv.Resolve(info.Command)
	}
	disableTerraformPluginCacheForExecution(config, info)
	if err := assembleComponentEnvVars(config, info, tenv); err != nil {
		return nil, err
	}
	return tenv, nil
}

// checkConfigurationWorkspace rejects a TF_WORKSPACE that would make Terraform use a different
// workspace than the one the configuration provider prepared, before any initialization runs.
// The component environment overrides the base environment, as it does for the Terraform process.
func checkConfigurationWorkspace(info *schema.ConfigAndStacksInfo) error {
	return checkConfigurationWorkspaceForPlatform(info, runtime.GOOS)
}

func checkConfigurationWorkspaceForPlatform(info *schema.ConfigAndStacksInfo, platform string) error {
	base := info.SanitizedEnv
	if base == nil {
		base = os.Environ()
	}
	workspace := ""
	for _, entries := range [][]string{base, info.ComponentEnvList} {
		for _, entry := range entries {
			if key, value, ok := strings.Cut(entry, "="); ok && (key == "TF_WORKSPACE" || platform == "windows" && strings.EqualFold(key, "TF_WORKSPACE")) {
				workspace = value
			}
		}
	}
	if workspace != "" && workspace != info.TerraformWorkspace {
		return fmt.Errorf("%w: TF_WORKSPACE %q differs from the prepared workspace %q", errUtils.ErrPluginInvalidConfig, workspace, info.TerraformWorkspace)
	}
	return nil
}

// rejectRuntimeArtifacts checks the whole artifact set before any file is written, so a
// generated file can never replace Terraform state, provider locks, or workdir metadata,
// even with an overwrite policy.
func rejectRuntimeArtifacts(files []schema.ComponentConfigurationFile) error {
	for _, file := range files {
		if isRuntimeArtifact(file.Path) {
			return fmt.Errorf("%w: generated file %s overlaps Terraform state or workdir runtime files", errUtils.ErrPluginInvalidConfig, file.Path)
		}
	}
	return nil
}

func isRuntimeArtifact(path string) bool {
	return configuration.IsRuntimeArtifact(path)
}

// recheckReviewedApply runs inside each subprocess attempt, after before hooks and recovery init.
func recheckReviewedApply(ctx context.Context, config *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo) error {
	if info.DryRun || !schema.ConfigurationOwned(info.ComponentSettingsSection) || !usesConfigurationPlan(info) || !restoresConfigurationPlan(info) {
		return nil
	}
	_, err := restoreConfigurationPlan(ctx, config, info)
	return err
}
