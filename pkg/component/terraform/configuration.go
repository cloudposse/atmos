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
	"strings"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclparse"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/component"
	"github.com/cloudposse/atmos/pkg/component/configuration"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/dependencies"
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
	if err := resolveSelectedExecutable(info); err != nil {
		return nil, err
	}
	prepared, stagedIAM, err := restoreOrPrepareConfiguration(ctx, config, info)
	if err != nil || prepared == nil {
		return prepared, err
	}
	if err = finalizePreparedConfiguration(ctx, config, info, prepared, stagedIAM); err != nil {
		return nil, err
	}
	applyPreparedConfiguration(config, info, prepared)
	return prepared, nil
}

func finalizePreparedConfiguration(ctx context.Context, config *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo, prepared *schema.PreparedComponentConfiguration, stagedIAM *schema.ComponentIAM) error {
	if err := applyStackOverrides(config, info, prepared); err != nil {
		return err
	}
	return finishIAM(ctx, info, prepared, stagedIAM)
}

// Plugins do not inherit PATH. Resolve the selected host executable before
// Terragrunt evaluates version constraints or dependency outputs.
func resolveSelectedExecutable(info *schema.ConfigAndStacksInfo) error {
	binary, err := exec.LookPath(info.Command)
	if err != nil {
		return errUtils.Build(fmt.Errorf("%w: Terraform executable of component %q in stack %q: %w", errUtils.ErrPluginInvalidConfig, info.ComponentFromArg, info.Stack, err)).
			WithContext("component", info.ComponentFromArg).
			WithContext("stack", info.Stack).
			WithContext("executable", info.Command).
			WithHint("The unit's `terraform_binary` or the component's `command` must name an installed executable, a toolchain tool from `dependencies.tools`, or an absolute path").
			Err()
	}
	binary, err = filepath.Abs(binary)
	if err != nil {
		return fmt.Errorf("%w: %w", errUtils.ErrPluginInvalidConfig, err)
	}
	info.Command = binary
	return nil
}

func applyPreparedConfiguration(_ *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo, prepared *schema.PreparedComponentConfiguration) {
	info.ComponentVarsSection = prepared.Inputs
	// The inputs were replaced, so any earlier classification of them no longer describes them.
	info.TerraformSecretVarKeysComputed = false
	// The stack's env section wins over provider environment in the metadata this invocation runs,
	// including the apply-time metadata of a restored saved plan.
	info.ComponentExecution = withoutStackEnvironment(invocationExecution(info, prepared), info.ComponentEnvSection)
	info.ConfigurationSensitiveInputKeys = maps.Clone(prepared.SensitiveInputKeys)
	info.ComponentBackendType, info.ComponentBackendSection = prepared.BackendType, prepared.Backend
	info.TerraformWorkspace = prepared.Workspace
	info.ComponentSection["vars"] = prepared.Inputs
	applyPreparedSource(info, prepared)
	info.ComponentSection["backend_type"] = prepared.BackendType
	info.ComponentSection["backend"] = prepared.Backend
	// Only a provider that owns the backend overrides generation. Otherwise the project setting decides.
	if prepared.BackendGenerated {
		info.SetBackendFileGeneration(false)
	}
	warnOnCacheLocalState(info, prepared)
}

// applyPreparedSource points the component at the module the provider prepared, with the
// invocation-only authorization for fetching it.
func applyPreparedSource(info *schema.ConfigAndStacksInfo, prepared *schema.PreparedComponentConfiguration) {
	info.SourceAuthorization = prepared.SourceAuthorization
	source, _ := info.ComponentSection["source"].(map[string]any)
	source = maps.Clone(source)
	if source == nil {
		source = make(map[string]any)
	}
	source["uri"] = prepared.Source
	// A local module changes without its location changing. The source provisioner stamps a local
	// directory and refreshes the retained workdir only when that stamp changes, so no TTL is set.
	info.ComponentSection["source"] = source
}

func restoreOrPrepareConfiguration(ctx context.Context, config *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo) (*schema.PreparedComponentConfiguration, *schema.ComponentIAM, error) {
	if usesConfigurationPlan(info) {
		if err := useRetainedWorkdir(config, info); err != nil {
			return nil, nil, err
		}
		prepared, err := restoreConfigurationPlan(ctx, config, info)
		return prepared, nil, err
	}
	staged, err := probeAndApplyIAM(ctx, config.ComponentConfigurations, info)
	if err != nil {
		return nil, nil, err
	}
	prepared, err := config.ComponentConfigurations.Prepare(ctx, info)
	return prepared, staged, err
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

// invocationExecution returns the hooks and extra arguments the invocation runs. A saved plan was
// prepared for plan, so applying it runs the apply-time metadata recorded with it.
func invocationExecution(info *schema.ConfigAndStacksInfo, prepared *schema.PreparedComponentConfiguration) *schema.ComponentExecution {
	if usesConfigurationPlan(info) {
		return prepared.Execution.ForSavedPlanApply()
	}
	return prepared.Execution
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
// and applying provider file policies. The retained-file check runs under the same workdir lock as
// the write batch, so another invocation cannot change a retained file between the check and the write.
func writePreparedConfiguration(directory string, prepared *schema.PreparedComponentConfiguration) error {
	if len(prepared.Files) == 0 && !prepared.BackendGenerated {
		return configuration.WriteFiles(directory, prepared.Files)
	}
	handoff := &configuration.Handoff{
		Validate: func() error {
			return validateRetainedConfiguration(directory, prepared.Files, prepared.BackendGenerated)
		},
	}
	if prepared.BackendGenerated {
		// The removal runs inside the batch's lock, after every destination validates, and is put back
		// when the batch fails, so a rejected batch keeps the existing host backend and its receipt.
		handoff.Paths = backend.HostBackendPaths()
		handoff.Run = func() error { return backend.RemoveHostBackend(directory) }
	}
	return configuration.WriteFilesWithHandoff(directory, prepared.Files, handoff)
}

// validateRetainedConfiguration runs before any backend cleanup or file writes.
// A skipped Terraform file may disagree with the provider's resolved state identity.
// Accept different bytes only when both versions are provably free of backend/cloud
// declarations; otherwise fail closed without replacing the user's retained file.
//
// When the host backend is about to be handed off, a skipped file is excluded only if its receipt
// and bytes verify that RemoveHostBackend will remove it; any other retained file is still checked.
func validateRetainedConfiguration(directory string, files []schema.ComponentConfigurationFile, backendHandoff bool) error {
	defer perf.Track(nil, "terraform.validateRetainedConfiguration")()

	for _, file := range files {
		if file.IfExists != "skip" || !terraformConfigurationPath(file.Path) {
			continue
		}
		if backendHandoff && backend.WillRemoveHostBackend(directory, file.Path) {
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
		if isRuntimeArtifact(file.Path) && !isAdjacentLockFileCopy(file) {
			return fmt.Errorf("%w: generated file %s overlaps Terraform state or workdir runtime files", errUtils.ErrPluginInvalidConfig, file.Path)
		}
	}
	return nil
}

// isAdjacentLockFileCopy reports the provider's literal copy of the lock file beside the unit,
// which carries the unit's provider pins. Generated lock files at any path stay rejected.
func isAdjacentLockFileCopy(file schema.ComponentConfigurationFile) bool {
	return file.UnitFileCopy && file.IfExists == "overwrite" && filepath.ToSlash(filepath.Clean(file.Path)) == ".terraform.lock.hcl"
}

func isRuntimeArtifact(path string) bool {
	return configuration.IsRuntimeArtifact(path)
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
	// observed is the plan path of the last observed subprocess run. When it ran but kept no
	// snapshot, the binding must not fall back to a later observation that after hooks could change.
	observed string
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
// The failed callback classifies the result with the phase's effective arguments and environment, the
// same classification the hooks use, so the "changes present" status of -detailed-exitcode
// (from the command line or TF_CLI_ARGS, including extra_arguments) still binds the plan.
func (s *planSnapshotter) around(phaseArgs []string, run func() error, failed func(error) bool) func() error {
	return func() (runResult error) {
		s.result, s.err, s.observed = nil, nil, ""
		path := s.pathFrom(phaseArgs)
		if path == "" {
			return run()
		}
		s.observed = path
		// The engine output path differs from the bound path when a policy publishes the artifact.
		writtenPath := path
		if s.execCtx.planPolicy != nil {
			writtenPath = plannedOutputPath(s.execCtx.componentPath, phaseArgs)
		}
		// A plan already at the output path was not written by this run. It is emptied in place, with a
		// private copy kept, so that only a plan this subprocess writes can be fingerprinted, and it is
		// restored if none is written, which also covers help-only runs that exit successfully.
		settleStale, stashErr := stashStalePlan(s.execCtx.componentPath, writtenPath)
		defer func() { runResult = s.settle(settleStale, runResult, failed) }()
		if stashErr != nil {
			s.err = stashErr
			return run()
		}
		return s.observe(path, writtenPath, run, failed)
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
		if s.observed == path {
			return nil, fmt.Errorf("%w: Terraform ran without a reviewed snapshot of its configuration", errUtils.ErrPlanVerificationFailed)
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

// recheckReviewedApply runs inside each subprocess attempt, after before hooks and recovery init.
func recheckReviewedApply(ctx context.Context, config *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo) error {
	if info.DryRun || !schema.ConfigurationOwned(info.ComponentSettingsSection) || !usesConfigurationPlan(info) || !restoresConfigurationPlan(info) {
		return nil
	}
	_, err := restoreConfigurationPlan(ctx, config, info)
	return err
}
