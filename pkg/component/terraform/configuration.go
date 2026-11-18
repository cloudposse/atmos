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
	"runtime"
	"slices"
	"strings"

	"github.com/google/shlex"
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclparse"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/component"
	"github.com/cloudposse/atmos/pkg/component/configuration"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/dependencies"
	envpkg "github.com/cloudposse/atmos/pkg/env"
	"github.com/cloudposse/atmos/pkg/perf"
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
	applyPreparedConfiguration(config, info, prepared, execution)
	warnOnCacheLocalState(info, prepared)
	return prepared, nil
}

// applyPreparedConfiguration projects the prepared configuration onto the invocation.
func applyPreparedConfiguration(config *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo, prepared *schema.PreparedComponentConfiguration, execution *schema.ComponentExecution) {
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
		return retainedFileError(file.Path, err)
	}
	defer root.Close()
	stat, err := root.Lstat(file.Path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return retainedFileError(file.Path, err)
	}
	if !stat.Mode().IsRegular() {
		return retainedBackendError(file.Path)
	}
	retained, err := root.ReadFile(file.Path)
	if err != nil {
		return retainedFileError(file.Path, err)
	}
	if bytes.Equal(retained, file.Content) {
		return nil
	}
	if configurationHasBackend(file.Path, retained) || configurationHasBackend(file.Path, file.Content) {
		return retainedBackendError(file.Path)
	}
	return nil
}

// retainedFileError classifies a failure to inspect a retained generated file as invalid
// configuration, as configuration.WriteFiles does for its working directory, and keeps the cause.
func retainedFileError(path string, err error) error {
	return fmt.Errorf("%w: retained generated file %s: %w", errUtils.ErrPluginInvalidConfig, path, err)
}

func retainedBackendError(path string) error {
	return fmt.Errorf("%w: cannot verify backend identity for retained generated file %s (if_exists = \"skip\"); use matching generated content or an explicit overwrite policy", errUtils.ErrPluginInvalidConfig, path)
}

func terraformConfigurationPath(path string) bool {
	lower := strings.ToLower(path)
	return strings.HasSuffix(lower, ".tf") || strings.HasSuffix(lower, ".tf.json") ||
		strings.HasSuffix(lower, ".tofu") || strings.HasSuffix(lower, ".tofu.json")
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
func validateEnvironmentPlanOutput(atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo, execCtx *componentExecContext, opts ...component.ExecutionOption) error {
	if execCtx.preparedConfiguration == nil || info.DryRun || info.SubCommand != subcommandPlan {
		return nil
	}
	return rejectEnvironmentPlanOutput(planEnvironment(atmosConfig, info, opts), runtime.GOOS)
}

// rejectEnvironmentPlanOutput rejects a plan output flag in the effective TF_CLI_ARGS or
// TF_CLI_ARGS_plan of an environment listed in precedence order. Variable names are
// case-insensitive on Windows only, where the last matching entry is the effective value.
func rejectEnvironmentPlanOutput(entries []string, goos string) error {
	for _, name := range []string{tfCliArgsEnvVar, tfCliArgsEnvVar + "_" + subcommandPlan} {
		value, _ := lastEnvListValue(entries, name, goos)
		args, err := tokenizeEnvironmentArgs(value)
		if err != nil {
			return fmt.Errorf("%w: %s cannot be parsed: %w", errUtils.ErrPlanVerificationFailed, name, err)
		}
		if containsOutputFlag(args) {
			return fmt.Errorf("%w: %s sets a plan output flag; pass -out as a command argument instead", errUtils.ErrPlanVerificationFailed, name)
		}
	}
	return nil
}

// tokenizeEnvironmentArgs splits a TF_CLI_ARGS value the way the engine does:
// spaces, tabs, and newlines separate tokens outside quotes, and a malformed
// value (for example an unterminated quote) is an error because the engine
// refuses to run with it. The shared parseArgs only splits on spaces, so a tab
// before -out would hide the flag from the check.
func tokenizeEnvironmentArgs(value string) ([]string, error) {
	return shlex.Split(value)
}

// planEnvironment lists the environment layers a plan subprocess receives, in precedence order.
func planEnvironment(atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo, opts []component.ExecutionOption) []string {
	entries := slices.Clone(planBaseEnvironment(info, opts))
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

// planSnapshot is the configuration a saved plan is bound to: the provider identity and the
// digest of the retained working directory, excluding the plan and the state Terraform writes.
type planSnapshot struct {
	path, identity, workingDigest, sensitiveDigest string
	// publicFiles holds the observed non-sensitive generated bytes as canonical JSON.
	publicFiles string
	// engineDigest is the digest of the Terraform executable observed around the subprocess, so a hook
	// that replaces the binary afterwards cannot move the binding onto the replacement. It is empty when
	// the executable could not be read, and the binding then falls back to its own observation.
	engineDigest string
	// planDigest is the digest of the plan file Terraform wrote, taken before any after hook runs. It is
	// empty when no digest was observed, and it is not part of the before-and-after comparison.
	planDigest string
}

// planSnapshotKey carries the plan snapshotter to the main command's subprocess.
type planSnapshotKey struct{}

// planSnapshotter observes the configuration around the Terraform subprocess, after the before
// hooks and before the after hooks. A hook that rewrites the module after Terraform read it then
// cannot move the binding onto the rewritten bytes.
type planSnapshotter struct {
	ctx      context.Context
	config   *schema.AtmosConfiguration
	info     *schema.ConfigAndStacksInfo
	execCtx  *componentExecContext
	pathFrom func(phaseArgs []string) string
	result   *planSnapshot
	err      error
}

// takePlanSnapshot fingerprints the configuration with the inputs the binding record uses.
func takePlanSnapshot(ctx context.Context, config *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo, execCtx *componentExecContext, path string) (*planSnapshot, error) {
	planProvider, err := planRuntime(config)
	if err != nil {
		return nil, err
	}
	identity, err := planProvider.PlanIdentity(ctx, info, execCtx.preparedConfiguration.Sources)
	if err != nil {
		return nil, err
	}
	digest, err := configurationWorkdirDigest(execCtx.workingDir, path)
	if err != nil {
		return nil, err
	}
	sensitive, err := sensitiveDigest(execCtx.workingDir, execCtx.preparedConfiguration.Files)
	if err != nil {
		return nil, err
	}
	publicFiles, err := snapshotPublicFiles(execCtx.workingDir, execCtx.preparedConfiguration.Files)
	if err != nil {
		return nil, err
	}
	engineDigest, _ := fileDigest(info.Command)
	return &planSnapshot{path: path, identity: identity, workingDigest: digest, sensitiveDigest: sensitive, engineDigest: engineDigest, publicFiles: publicFiles}, nil
}

// around wraps one run of the Terraform subprocess. The configuration must be the same before and
// after it; the unchanged snapshot is kept for the binding, and a changed one rejects the plan.
func (s *planSnapshotter) around(phaseArgs []string, run func() error) func() error {
	return func() error {
		s.result, s.err = nil, nil
		path := s.pathFrom(phaseArgs)
		if path == "" {
			return run()
		}
		before, beforeErr := takePlanSnapshot(s.ctx, s.config, s.info, s.execCtx, path)
		runErr := run()
		if beforeErr != nil {
			// The command still completes and its outcome is reported; the binding is what fails.
			s.err = beforeErr
			return runErr
		}
		after, err := takePlanSnapshot(s.ctx, s.config, s.info, s.execCtx, path)
		switch {
		case err != nil:
			s.err = err
		case *after != *before:
			s.err = fmt.Errorf("%w: configuration changed while Terraform ran", errUtils.ErrPlanVerificationFailed)
		default:
			// Terraform writes the plan itself unless a policy gate publishes it later, so the plan is
			// fingerprinted now and an after hook cannot move the binding onto rewritten bytes.
			if s.execCtx.planPolicy == nil {
				if digest, digestErr := fileDigest(path); digestErr == nil {
					after.planDigest = digest
				}
			}
			s.result = after
		}
		return runErr
	}
}

// reviewedSnapshot returns the configuration Terraform read, or takes a fresh snapshot when no
// observed run recorded one for this plan.
func (c *componentExecContext) reviewedSnapshot(ctx context.Context, config *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo, path string) (*planSnapshot, error) {
	if s := c.planSnapshots; s != nil {
		if s.err != nil {
			return nil, s.err
		}
		if s.result != nil && s.result.path == path {
			return s.result, nil
		}
	}
	return takePlanSnapshot(ctx, config, info, c, path)
}

// observePlanSnapshots returns ctx carrying a snapshotter for a plan this invocation binds. Policy
// plans publish the artifact at the original output path, so that path is the one excluded from the
// digest. The returned context is unchanged when no plan is bound.
func (c *componentExecContext) observePlanSnapshots(ctx context.Context, config *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo, policyArgs []string) context.Context {
	c.planSnapshots = nil
	if !bindsPlan(info, c) {
		return ctx
	}
	c.planSnapshots = &planSnapshotter{ctx: ctx, config: config, info: info, execCtx: c, pathFrom: func(phaseArgs []string) string {
		if c.planPolicy != nil {
			phaseArgs = policyArgs
		}
		return plannedOutputPath(c.componentPath, phaseArgs)
	}}
	return context.WithValue(ctx, planSnapshotKey{}, c.planSnapshots)
}
