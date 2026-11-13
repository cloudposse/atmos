package terraform

import (
	"bytes"
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"hash/fnv"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclparse"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/component"
	"github.com/cloudposse/atmos/pkg/component/configuration"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/dependencies"
	envpkg "github.com/cloudposse/atmos/pkg/env"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/provisioner"
	"github.com/cloudposse/atmos/pkg/provisioner/workdir"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/terraform/backend"
	"github.com/cloudposse/atmos/pkg/ui"
	u "github.com/cloudposse/atmos/pkg/utils"
)

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
	if err := checkStackOverrides(info); err != nil {
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
	if err = applyStackOverrides(config, info, prepared); err != nil {
		return nil, err
	}
	info.ComponentVarsSection = prepared.Inputs
	// The stack's env section wins over provider environment in the metadata this invocation runs,
	// including the apply-time metadata of a restored saved plan.
	info.ComponentExecution = withoutStackEnvironment(execution, info.ComponentEnvSection)
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
	// A local module changes without its location changing. The source provisioner stamps a local
	// directory and refreshes the retained workdir only when that stamp changes, so no TTL is set.
	info.ComponentSection["source"] = source
	info.ComponentSection["backend_type"] = prepared.BackendType
	info.ComponentSection["backend"] = prepared.Backend
	config.Components.Terraform.AutoGenerateBackendFile = !prepared.BackendGenerated
	warnOnCacheLocalState(info, prepared)
	return prepared, nil
}

// terragruntCacheDirectory is where Terragrunt keeps its working copies of a unit.
const terragruntCacheDirectory = ".terragrunt-cache"

// cacheLocalStatePath returns the local state file path when it lives inside a Terragrunt cache.
func cacheLocalStatePath(prepared *schema.PreparedComponentConfiguration) (string, bool) {
	if prepared.BackendType != "local" {
		return "", false
	}
	path, _ := prepared.Backend["path"].(string)
	for _, segment := range strings.Split(filepath.ToSlash(path), "/") {
		if segment == terragruntCacheDirectory {
			return path, true
		}
	}
	return "", false
}

// warnOnCacheLocalState tells the user that state kept inside .terragrunt-cache is easy to lose.
// The location is deliberate (it is Terragrunt's own backend identity, so existing state keeps
// working), but cleaning the cache, or checking out to another path, silently starts from empty state.
func warnOnCacheLocalState(info *schema.ConfigAndStacksInfo, prepared *schema.PreparedComponentConfiguration) {
	path, inCache := cacheLocalStatePath(prepared)
	if !inCache {
		return
	}
	ui.Warningf("Component `%s` in stack `%s` keeps its local state at `%s`, inside `%s`. "+
		"The location depends on the checkout path, and deleting the cache (for example by cleaning untracked files) loses the state.",
		info.ComponentFromArg, info.Stack, path, terragruntCacheDirectory)
	ui.Hint("Configure `remote_state` with a remote backend, or a local backend with a stable path such as " +
		"`path = \"${get_terragrunt_dir()}/terraform.tfstate\"`, and move the existing state file there.")
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

// writePreparedConfiguration verifies retained files before relinquishing the host backend
// and applying provider file policies.
func writePreparedConfiguration(directory string, prepared *schema.PreparedComponentConfiguration) error {
	if err := validateRetainedConfiguration(directory, prepared.Files); err != nil {
		return err
	}
	if prepared.BackendGenerated {
		if err := backend.RemoveHostBackend(directory); err != nil {
			return err
		}
	}
	return configuration.WriteFiles(directory, prepared.Files)
}

// validateRetainedConfiguration runs before any backend cleanup or file writes.
// A skipped Terraform file may disagree with the provider's resolved state identity.
// Accept different bytes only when both versions are provably free of backend/cloud
// declarations; otherwise fail closed without replacing the user's retained file.
func validateRetainedConfiguration(directory string, files []schema.ComponentConfigurationFile) error {
	defer perf.Track(nil, "terraform.validateRetainedConfiguration")()

	for _, file := range files {
		if file.IfExists != "skip" || !terraformConfigurationPath(file.Path) {
			continue
		}
		if err := validateRetainedFile(directory, file); err != nil {
			return err
		}
	}
	return nil
}

func validateRetainedFile(directory string, file schema.ComponentConfigurationFile) error {
	if !filepath.IsLocal(file.Path) {
		return errUtils.ErrPluginProtocol
	}
	root, err := os.OpenRoot(directory)
	if err != nil {
		return err
	}
	defer root.Close()
	stat, err := root.Lstat(file.Path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if !stat.Mode().IsRegular() {
		return retainedBackendError(file.Path)
	}
	retained, err := root.ReadFile(file.Path)
	if err != nil {
		return err
	}
	if bytes.Equal(retained, file.Content) {
		return nil
	}
	if configurationHasBackend(file.Path, retained) || configurationHasBackend(file.Path, file.Content) {
		return retainedBackendError(file.Path)
	}
	return nil
}

func retainedBackendError(path string) error {
	return fmt.Errorf("%w: cannot verify backend identity for retained generated file %s (if_exists = \"skip\"); use matching generated content or an explicit overwrite policy", errUtils.ErrPluginInvalidConfig, path)
}

func terraformConfigurationPath(path string) bool {
	lower := strings.ToLower(path)
	return strings.HasSuffix(lower, ".tf") || strings.HasSuffix(lower, ".tf.json")
}

// configurationHasBackend also returns true for unparseable configuration: absence
// of a backend cannot then be established. Diagnostics never expose retained secrets.
func configurationHasBackend(path string, content []byte) bool {
	parser := hclparse.NewParser()
	var file *hcl.File
	var diagnostics hcl.Diagnostics
	if strings.HasSuffix(strings.ToLower(path), ".json") {
		file, diagnostics = parser.ParseJSON(content, path)
	} else {
		file, diagnostics = parser.ParseHCL(content, path)
	}
	if diagnostics.HasErrors() {
		return true
	}
	body, _, diagnostics := file.Body.PartialContent(&hcl.BodySchema{Blocks: []hcl.BlockHeaderSchema{{Type: "terraform"}}})
	if diagnostics.HasErrors() {
		return true
	}
	for _, block := range body.Blocks {
		nested, _, diags := block.Body.PartialContent(&hcl.BodySchema{Blocks: []hcl.BlockHeaderSchema{{Type: "backend", LabelNames: []string{"type"}}, {Type: "cloud"}}})
		if diags.HasErrors() || len(nested.Blocks) != 0 {
			return true
		}
	}
	return false
}

// checkConfigurationWorkspace rejects a TF_WORKSPACE that would make Terraform use a different
// workspace than the one the configuration provider prepared, before any initialization runs.
// The component environment overrides the base environment, as it does for the Terraform process.
func checkConfigurationWorkspace(info *schema.ConfigAndStacksInfo) error {
	base := info.SanitizedEnv
	if base == nil {
		base = os.Environ()
	}
	workspace := ""
	for _, entries := range [][]string{base, info.ComponentEnvList} {
		for _, entry := range entries {
			if key, value, ok := strings.Cut(entry, "="); ok && key == "TF_WORKSPACE" {
				workspace = value
			}
		}
	}
	if workspace != "" && workspace != info.TerraformWorkspace {
		return fmt.Errorf("%w: TF_WORKSPACE %q differs from the prepared workspace %q", errUtils.ErrPluginInvalidConfig, workspace, info.TerraformWorkspace)
	}
	return nil
}

// Workdir runtime paths generated artifacts must never write. They mirror the paths the
// workdir sync preserves: Terraform data and workspace-state directories, Atmos metadata,
// provider lock files at any depth, and the default local-backend state files at the root.
var (
	runtimeArtifactDirs  = map[string]bool{".terraform": true, "terraform.tfstate.d": true, workdir.AtmosDir: true}
	runtimeArtifactFiles = map[string]bool{"terraform.tfstate": true, "terraform.tfstate.backup": true, ".terraform.tfstate.lock.info": true}
)

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
	path = filepath.ToSlash(filepath.Clean(filepath.FromSlash(path)))
	if runtimeArtifactFiles[path] || strings.HasSuffix(path, provisioner.CanonicalLockFilename) {
		return true
	}
	for _, segment := range strings.Split(path, "/") {
		if runtimeArtifactDirs[segment] {
			return true
		}
	}
	return false
}

// validatePlanOutputFlags rejects ambiguous plan destinations before hooks or Terraform run.
func validatePlanOutputFlags(command string, args []string) error {
	if command != "plan" {
		return nil
	}
	count := 0
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "-out" || arg == "--out" {
			count++
			i++
		} else if strings.HasPrefix(arg, "-out=") || strings.HasPrefix(arg, "--out=") {
			count++
		}
		if count > 1 {
			return fmt.Errorf("%w: multiple plan output flags", errUtils.ErrPlanVerificationFailed)
		}
	}
	return nil
}

// validateEnvironmentPlanOutput rejects plan output flags in TF_CLI_ARGS or
// TF_CLI_ARGS_plan for a configuration-owned plan. Terraform reads these
// variables itself, so a flag there could choose a plan file that Atmos does not
// see and therefore does not bind. The check inspects the effective value of each
// variable in the subprocess environment: the sanitized process environment (or
// the inherited one when auth did not sanitize it), the global atmos.yaml
// environment, the component environment, and provider extra-argument
// environment, with later layers overriding earlier ones. Hooks run as child
// processes and cannot change it.
func validateEnvironmentPlanOutput(atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo, execCtx *componentExecContext) error {
	if execCtx.preparedConfiguration == nil || info.DryRun || info.SubCommand != subcommandPlan {
		return nil
	}
	effective := make(map[string]string)
	for _, entry := range planEnvironment(atmosConfig, info) {
		if name, value, ok := strings.Cut(entry, "="); ok {
			effective[name] = value
		}
	}
	for _, name := range []string{tfCliArgsEnvVar, tfCliArgsEnvVar + "_" + subcommandPlan} {
		if containsOutputFlag(parseArgs(effective[name])) {
			return fmt.Errorf("%w: %s sets a plan output flag; pass -out as a command argument instead", errUtils.ErrPlanVerificationFailed, name)
		}
	}
	return nil
}

// planEnvironment lists the environment layers a plan subprocess receives, in precedence order.
func planEnvironment(atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo) []string {
	base := info.SanitizedEnv
	if base == nil {
		base = os.Environ()
	}
	entries := slices.Clone(base)
	if atmosConfig != nil {
		entries = envpkg.MergeGlobalEnv(entries, atmosConfig.Env)
	}
	entries = append(entries, info.ComponentEnvList...)
	if info.ComponentExecution == nil {
		return entries
	}
	for i := range info.ComponentExecution.ExtraArguments {
		item := &info.ComponentExecution.ExtraArguments[i]
		if !slices.Contains(item.Commands, subcommandPlan) {
			continue
		}
		for name, value := range item.Environment {
			entries = append(entries, name+"="+value)
		}
	}
	return entries
}

// containsOutputFlag reports any plan output flag, with or without a value.
func containsOutputFlag(args []string) bool {
	return slices.ContainsFunc(args, func(arg string) bool {
		return arg == "-out" || arg == "--out" || strings.HasPrefix(arg, "-out=") || strings.HasPrefix(arg, "--out=")
	})
}

func plannedOutputPath(directory string, args []string) string {
	for i, arg := range args {
		path := ""
		switch {
		case (arg == "-out" || arg == "--out") && i+1 < len(args):
			path = args[i+1]
		case strings.HasPrefix(arg, "-out="):
			path = strings.TrimPrefix(arg, "-out=")
		case strings.HasPrefix(arg, "--out="):
			path = strings.TrimPrefix(arg, "--out=")
		}
		if path != "" {
			if filepath.IsAbs(path) {
				return path
			}
			return filepath.Join(directory, path)
		}
	}
	return ""
}
