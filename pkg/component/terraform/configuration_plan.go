package terraform

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/component/configuration"
	"github.com/cloudposse/atmos/pkg/schema"
)

// freshPlanName is the file `deploy` writes the plan it generates for verification to.
const freshPlanName = "new.plan"

const configurationPlanSuffix = ".atmos.json"

// configurationPlanVersion is bumped whenever the stored binding changes shape.
const configurationPlanVersion = 3

// subcommandPlan names the Terraform plan command.
const subcommandPlan = "plan"

// planBindingSuffix names the binding of a default saved plan, which Atmos writes as "*.planfile".
const planBindingSuffix = ".planfile" + configurationPlanSuffix

// configurationPlan is the private record stored beside a saved plan. It binds the plan to the
// exact preparation it was made from. Evaluated inputs and backend settings are not stored:
// the plan file already embodies the inputs, and the backend is re-resolved at apply time and
// compared by BackendDigest, so credentials in backend settings never reach this file.
//
// RebuildDigest and LockDigest let a runner that holds only the restored artifacts prove that a
// workdir it re-provisioned from the checkout equals the reviewed one: RebuildDigest covers the
// files restoration regenerates (module source and generated files, not the plan, its record,
// variable files, state, or the lock file), and LockDigest the provider lock file ("" when absent).
type configurationPlan struct {
	Version                                                                               int
	Stack, Component, Identity, EngineDigest, PlanDigest, WorkingDirectory, WorkingDigest string
	// WorkingIdentity binds a relative working_directory override to its checkout-relative location.
	// Older records omit it and remain bound to their original absolute directory.
	WorkingIdentity                          string
	BackendDigest, RebuildDigest, LockDigest string
	// Modules records that the reviewed workdir held downloaded child modules, which RebuildDigest covers.
	// A rebuild downloads them again before verification. Older records omit it.
	Modules  bool `json:",omitempty"`
	Prepared *schema.PreparedComponentConfiguration
}

// ownsConfigurationPlan reports whether a provider manages this component's saved plans.
// Their plan files are reviewed artifacts bound to a record, not ephemeral scratch files.
func ownsConfigurationPlan(info *schema.ConfigAndStacksInfo) bool {
	_, owned := info.ComponentSettingsSection["configuration"]
	return owned
}

// usesConfigurationPlan reports whether the invocation applies a saved plan. A downloaded
// StoredPlanFile alone does not: it is a reference that `deploy` compares a fresh plan against, and
// the fresh plan is what gets applied.
func usesConfigurationPlan(info *schema.ConfigAndStacksInfo) bool {
	return info.UseTerraformPlan || info.PlanFile != ""
}

// dropCIPlanLocation forgets the plan path the CI download hook records while it fetches a stored
// plan for `deploy`. An apply may have an explicitly supplied PlanFile even when
// UseTerraformPlan is false, so only deploy's hook path is discarded.
func dropCIPlanLocation(info *schema.ConfigAndStacksInfo) {
	if info.SubCommand == "deploy" && info.PlanFile != "" && !info.UseTerraformPlan && info.StoredPlanFile != "" {
		info.PlanFile = ""
	}
}

func planRuntime(config *schema.AtmosConfiguration) (schema.ComponentConfigurationPlanRuntime, error) {
	runtime, ok := config.ComponentConfigurations.(schema.ComponentConfigurationPlanRuntime)
	if !ok {
		return nil, fmt.Errorf("%w: provider cannot bind saved plans", errUtils.ErrPluginUnsupportedExtension)
	}
	return runtime, nil
}

func configurationPlanPath(config *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo) string {
	if info.PlanFile != "" {
		if filepath.IsAbs(info.PlanFile) {
			return info.PlanFile
		}
		return filepath.Join(ConstructTerraformComponentWorkingDir(config, info), info.PlanFile)
	}
	return ConstructTerraformComponentPlanfilePath(config, info)
}

// restoreConfigurationPlan verifies the reviewed artifact before returning its original preparation.
// It requires the retained working directory; a runner that holds only the restored artifacts goes
// through restoreOrRebuildConfigurationPlan instead, which re-provisions the directory.
func restoreConfigurationPlan(ctx context.Context, config *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo) (*schema.PreparedComponentConfiguration, error) {
	prepared, rebuild, err := restoreOrRebuildConfigurationPlan(ctx, config, info)
	if err != nil {
		return nil, err
	}
	if rebuild != nil {
		return nil, fmt.Errorf("%w: reviewed working directory changed", errUtils.ErrPlanVerificationFailed)
	}
	return prepared, nil
}

// restoreOrRebuildConfigurationPlan verifies everything about a reviewed plan that does not depend on
// the working directory: the record's coordinates, the plan bytes, the engine, the provider, options
// and watched sources, and the backend. The directory itself is then either the retained one (its
// digest equals the reviewed digest), or it holds nothing but the restored artifacts and must be
// re-provisioned (a non-nil rebuild), or it is unverifiable and the plan is rejected.
func restoreOrRebuildConfigurationPlan(ctx context.Context, config *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo) (*schema.PreparedComponentConfiguration, *reviewedRebuild, error) {
	if !restoresConfigurationPlan(info) {
		return nil, nil, fmt.Errorf("%w: plugin saved plans require direct apply of the original artifact", errUtils.ErrPluginUnsupportedExtension)
	}
	if _, err := planRuntime(config); err != nil {
		return nil, nil, err
	}
	path := configurationPlanPath(config, info)
	plan, err := loadConfigurationPlan(path, info)
	if err != nil {
		return nil, nil, err
	}
	if plan.WorkingIdentity != "" {
		workdir := filepath.Clean(ConstructTerraformComponentWorkingDir(config, info))
		if relativeWorkdirIdentity(config, info, workdir) != plan.WorkingIdentity {
			return nil, nil, fmt.Errorf("%w: reviewed working directory changed", errUtils.ErrPlanVerificationFailed)
		}
		plan.WorkingDirectory = workdir
	}
	if err := verifyReviewedArtifacts(ctx, config, info, plan, path); err != nil {
		return nil, nil, err
	}
	rebuild, err := restoreReviewedWorkdir(info, plan, path)
	if err != nil {
		return nil, nil, err
	}
	// Terraform runs inside the workdir, so a path relative to the project would not resolve there.
	absolute, err := filepath.Abs(path)
	if err != nil {
		return nil, nil, err
	}
	info.ConfigurationPlanImplicit = info.ConfigurationPlanImplicit || info.PlanFile == ""
	info.UseTerraformPlan = true
	info.PlanFile = absolute
	return plan.Prepared, rebuild, nil
}

// restoreReviewedWorkdir settles the reviewed plan's working directory. A retained workdir gets the
// reviewed bytes of its sensitive files back and needs no rebuild; any other workdir must be rebuilt.
func restoreReviewedWorkdir(info *schema.ConfigAndStacksInfo, plan *configurationPlan, path string) (*reviewedRebuild, error) {
	retained, err := reviewedWorkdirRetained(info, plan, path)
	if err != nil {
		return nil, err
	}
	if !retained {
		return newReviewedRebuild(info, plan, path)
	}
	// The retained workdir is verified, so it holds the reviewed bytes of sensitive files. A rebuild
	// instead writes them again from a fresh preparation.
	if err := restoreSensitiveContent(plan.WorkingDirectory, plan.Prepared.Files); err != nil {
		return nil, err
	}
	return nil, nil
}

// reviewedWorkdirRetained reports whether the working directory still equals the reviewed one, either
// as the workdir the plan ran in or as a verified rebuild of it.
func reviewedWorkdirRetained(info *schema.ConfigAndStacksInfo, plan *configurationPlan, path string) (bool, error) {
	retained, err := workdirMatches(plan, path)
	if err != nil || retained {
		return retained, err
	}
	// A rebuild that was verified and then failed in init or apply leaves a workdir that no longer
	// holds only the restored artifacts. It still equals the reviewed rebuild, so a retry may use it.
	return rebuiltWorkdirMatches(info, plan, path)
}

// restoresConfigurationPlan reports whether the invocation applies a reviewed artifact. The downloaded
// stored plan that `deploy` verifies against is never applied itself, so it must not be the plan to apply.
func restoresConfigurationPlan(info *schema.ConfigAndStacksInfo) bool {
	return (info.StoredPlanFile == "" || info.StoredPlanFile != info.PlanFile) && (info.SubCommand == "apply" || info.SubCommand == "deploy")
}

// loadConfigurationPlan reads the companion binding and checks it targets this component.
func loadConfigurationPlan(path string, info *schema.ConfigAndStacksInfo) (*configurationPlan, error) {
	if err := requireSavedPlan(path, info); err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path + configurationPlanSuffix)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, missingPlanRecordError(path, info)
	}
	if err != nil {
		return nil, fmt.Errorf("%w: plugin plan binding cannot be read: %w", errUtils.ErrPlanVerificationFailed, err)
	}
	var plan configurationPlan
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.UseNumber()
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&plan); err != nil {
		return nil, errUtils.ErrPlanVerificationFailed
	}
	if plan.Version != configurationPlanVersion || plan.Prepared == nil || plan.Stack != info.Stack || plan.Component != info.ComponentFromArg {
		return nil, errUtils.ErrPlanVerificationFailed
	}
	// The stored record omits inputs; the plan file already carries every value Terraform applies.
	plan.Prepared.Inputs = map[string]any{}
	return &plan, nil
}

// requireSavedPlan explains an absent plan file in terms of what the user did, not of file access.
func requireSavedPlan(path string, info *schema.ConfigAndStacksInfo) error {
	if _, err := os.Stat(path); err == nil || !errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if info.PlanFile != "" {
		return errUtils.Build(errUtils.ErrSavedPlanNotFound).
			WithExplanationf("The plan file `%s` does not exist.", path).
			WithHintf("Create it with `atmos terraform plan %s -s %s -- -out=%s`.", info.ComponentFromArg, info.Stack, path).
			Err()
	}
	return errUtils.Build(errUtils.ErrSavedPlanNotFound).
		WithExplanationf("Component `%s` in stack `%s` has no saved plan. It was never planned, or its plan was already applied or replaced by a later apply.", info.ComponentFromArg, info.Stack).
		WithHintf("Run `atmos terraform plan %s -s %s` first, then apply with `--from-plan`.", info.ComponentFromArg, info.Stack).
		Err()
}

// missingPlanRecordError reports a plan file that Atmos did not create for this component.
func missingPlanRecordError(path string, info *schema.ConfigAndStacksInfo) error {
	return errUtils.Build(errUtils.ErrSavedPlanNotFound).
		WithExplanationf("The plan file `%s` has no Atmos plan record (`%s%s`). Plugin-managed components apply only plans created by `atmos terraform plan`, because the record proves what was reviewed.", path, filepath.Base(path), configurationPlanSuffix).
		WithHintf("Run `atmos terraform plan %s -s %s -- -out=%s` and apply that file.", info.ComponentFromArg, info.Stack, path).
		Err()
}

// configurationArtifactMatches reports whether the plan file, engine binary, and working directory path are unchanged.
func configurationArtifactMatches(config *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo, plan *configurationPlan, path string) (bool, error) {
	digest, err := fileDigest(path)
	if err != nil {
		return false, err
	}
	engine, err := fileDigest(info.Command)
	if err != nil {
		return false, err
	}
	workdir := filepath.Clean(ConstructTerraformComponentWorkingDir(config, info))
	return digest == plan.PlanDigest && engine == plan.EngineDigest && workdir == plan.WorkingDirectory, nil
}

// verifyReviewedArtifacts checks everything about a reviewed plan that does not depend on the working
// directory: provider identity and watched sources, plan bytes, engine, and backend.
func verifyReviewedArtifacts(ctx context.Context, config *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo, plan *configurationPlan, path string) error {
	runtime, err := planRuntime(config)
	if err != nil {
		return err
	}
	identity, err := runtime.PlanIdentity(ctx, info, plan.Prepared.Sources)
	if err != nil {
		return err
	}
	artifactMatches, err := configurationArtifactMatches(config, info, plan, path)
	if err != nil {
		return err
	}
	if identity != plan.Identity || !artifactMatches {
		return fmt.Errorf("%w: plugin, source, engine, workdir, or reviewed artifact changed; create a new plan", errUtils.ErrPlanVerificationFailed)
	}
	return verifyConfigurationPlanState(ctx, config, info, plan)
}

// workdirMatches reports whether the retained working directory still holds exactly the reviewed
// bytes. A directory that no longer exists simply does not match.
func workdirMatches(plan *configurationPlan, path string) (bool, error) {
	workingDigest, err := configurationWorkdirDigest(plan.WorkingDirectory, path)
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return workingDigest == plan.WorkingDigest, nil
}

// recordConfigurationPlan stores a private companion beside the completed plan.
func recordConfigurationPlan(ctx context.Context, config *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo, execCtx *componentExecContext, args []string) error {
	path := recordedPlanPath(info, execCtx, args)
	if path == "" {
		return nil
	}
	runtime, err := planRuntime(config)
	if err != nil {
		return err
	}
	identity, err := runtime.PlanIdentity(ctx, info, execCtx.preparedConfiguration.Sources)
	if err != nil {
		return err
	}
	plan, err := newConfigurationPlan(config, info, execCtx, identity, path)
	if err != nil {
		return err
	}
	data, err := json.Marshal(plan)
	if err != nil {
		return err
	}
	return writePlanBinding(path+configurationPlanSuffix, data)
}

// recordedPlanPath returns the plan file to bind, or empty when this invocation produces none.
func recordedPlanPath(info *schema.ConfigAndStacksInfo, execCtx *componentExecContext, args []string) string {
	if execCtx.preparedConfiguration == nil || info.DryRun || info.SubCommand != "plan" {
		return ""
	}
	return plannedOutputPath(execCtx.componentPath, args)
}

// newConfigurationPlan binds the identity to the digests of the plan, engine, and working directory.
func newConfigurationPlan(config *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo, execCtx *componentExecContext, identity, path string) (*configurationPlan, error) {
	planDigest, err := fileDigest(path)
	if err != nil {
		return nil, err
	}
	engineDigest, err := fileDigest(info.Command)
	if err != nil {
		return nil, err
	}
	workingDigest, err := configurationWorkdirDigest(execCtx.workingDir, path)
	if err != nil {
		return nil, err
	}
	backendDigest, err := backendIdentityDigest(execCtx.preparedConfiguration.Backend)
	if err != nil {
		return nil, err
	}
	rebuildDigest, err := configurationRebuildDigest(execCtx.workingDir, path, transientWorkdirFiles(info))
	if err != nil {
		return nil, err
	}
	lock, err := lockFileDigest(execCtx.workingDir)
	if err != nil {
		return nil, err
	}
	// Inputs and backend values stay out of the record; see configurationPlan. Sensitive generated
	// files, such as a backend file with credentials, keep only their place: apply reads their bytes
	// back from the retained workdir, whose digest proves they are the reviewed ones, and a rebuild
	// writes them again from a fresh preparation that must match RebuildDigest.
	stored := *execCtx.preparedConfiguration
	stored.Inputs, stored.Backend = nil, nil
	stored.Files = withoutSensitiveContent(stored.Files)
	return &configurationPlan{WorkingDigest: workingDigest, Version: configurationPlanVersion, Stack: info.Stack, Component: info.ComponentFromArg, Identity: identity, EngineDigest: engineDigest, PlanDigest: planDigest, WorkingDirectory: filepath.Clean(execCtx.workingDir), WorkingIdentity: relativeWorkdirIdentity(config, info, execCtx.workingDir), BackendDigest: backendDigest, RebuildDigest: rebuildDigest, LockDigest: lock, Modules: hasDownloadedModules(execCtx.workingDir), Prepared: &stored}, nil
}

// relativeWorkdirIdentity is set only when an explicit relative override actually resolves
// beneath the checkout root. Absolute and legacy workdirs retain exact-path verification.
func relativeWorkdirIdentity(config *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo, workdir string) string {
	if config == nil || info == nil {
		return ""
	}
	override := workingDirectoryOverride(info)
	if override == "" || filepath.IsAbs(override) {
		return ""
	}
	base, err := filepath.Abs(config.BasePath)
	if err != nil {
		return ""
	}
	actual, err := filepath.Abs(workdir)
	if err != nil {
		return ""
	}
	relative, err := filepath.Rel(base, actual)
	if err != nil || !matchesRelativeWorkdir(relative, override) {
		return ""
	}
	return "relative:" + filepath.ToSlash(relative)
}

func matchesRelativeWorkdir(relative, override string) bool {
	return relative != "." && relative != ".." &&
		!strings.HasPrefix(relative, ".."+string(filepath.Separator)) &&
		relative == filepath.Clean(override)
}

func workingDirectoryOverride(info *schema.ConfigAndStacksInfo) string {
	for _, section := range []string{"metadata", "settings"} {
		values, _ := info.ComponentSection[section].(map[string]any)
		if value, ok := values["working_directory"].(string); ok && value != "" {
			return value
		}
	}
	return ""
}

// withoutSensitiveContent copies files, dropping the bytes of every sensitive one.
func withoutSensitiveContent(files []schema.ComponentConfigurationFile) []schema.ComponentConfigurationFile {
	stored := slices.Clone(files)
	for i := range stored {
		if stored[i].Sensitive {
			stored[i].Content = nil
		}
	}
	return stored
}

// restoreSensitiveContent reads the bytes of each sensitive file back from the verified workdir.
func restoreSensitiveContent(workdir string, files []schema.ComponentConfigurationFile) error {
	if !slices.ContainsFunc(files, func(file schema.ComponentConfigurationFile) bool { return file.Sensitive }) {
		return nil
	}
	root, err := os.OpenRoot(workdir)
	if err != nil {
		return planIOError(err)
	}
	defer root.Close()
	for i := range files {
		if !files[i].Sensitive {
			continue
		}
		content, err := root.ReadFile(files[i].Path)
		if err != nil {
			return fmt.Errorf("%w: reviewed generated file %s is missing: %w", errUtils.ErrPlanVerificationFailed, files[i].Path, err)
		}
		files[i].Content = content
	}
	return nil
}

// backendIdentityDigest hashes the canonical JSON form of backend settings, which only ever need equality.
func backendIdentityDigest(backend map[string]any) (string, error) {
	data, err := json.Marshal(backend)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:]), nil
}

func fileDigest(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", planIOError(err)
	}
	defer file.Close()
	h := sha256.New()
	if _, err := io.Copy(h, file); err != nil {
		return "", planIOError(err)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func writePlanBinding(path string, data []byte) error {
	file, temporary, err := configuration.CreateSensitiveTemp(filepath.Dir(path))
	if err != nil {
		return planIOError(err)
	}
	defer os.Remove(temporary)
	if _, err := file.Write(data); err != nil {
		file.Close()
		return planIOError(err)
	}
	if err := file.Close(); err != nil {
		return planIOError(err)
	}
	if err := os.Rename(temporary, path); err != nil {
		return planIOError(err)
	}
	return nil
}

// planIOError wraps a saved-plan filesystem failure while keeping the underlying cause.
func planIOError(err error) error {
	return fmt.Errorf("%w: %w", errUtils.ErrPlanVerificationFailed, err)
}

// verifyConfigurationPlanState rejects a changed backend without reading dependency outputs.
func verifyConfigurationPlanState(ctx context.Context, config *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo, plan *configurationPlan) error {
	runtime, ok := config.ComponentConfigurations.(schema.ComponentConfigurationStateRuntime)
	if !ok {
		return errUtils.ErrPluginUnsupportedExtension
	}
	state, err := runtime.ResolveState(ctx, info)
	if err != nil {
		return err
	}
	if state == nil || state.BackendType != plan.Prepared.BackendType || state.Workspace != plan.Prepared.Workspace {
		return fmt.Errorf("%w: reviewed backend identity changed", errUtils.ErrPlanVerificationFailed)
	}
	digest, err := backendIdentityDigest(state.Backend)
	if err != nil || digest != plan.BackendDigest {
		return fmt.Errorf("%w: reviewed backend identity changed", errUtils.ErrPlanVerificationFailed)
	}
	// The verified current backend stands in for the one the record deliberately does not store.
	plan.Prepared.Backend = state.Backend
	return nil
}

// stateChangingSubcommands make an earlier saved plan stale once they succeed.
var stateChangingSubcommands = map[string]struct{}{"apply": {}, "deploy": {}, "destroy": {}}

// finishConfigurationPlan removes a saved plan and its record after a successful command made it
// unusable: the plan was applied, or a direct apply changed the state it was made against. Any
// other command leaves the reviewed plan alone, so plan, output, apply --from-plan keeps working.
// An explicit plan path is the user's file, so only its Atmos record is removed.
func finishConfigurationPlan(config *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo) {
	if !ownsConfigurationPlan(info) || info.DryRun {
		return
	}
	// A CI exit-code mapping can report a failed apply as success. The subprocess's own status
	// decides, so a plan whose apply failed (for example on a state lock) can be applied again.
	if info.ExecMetadataRawExitCode != 0 {
		return
	}
	if _, changes := stateChangingSubcommands[info.SubCommand]; !changes {
		return
	}
	canonical := ConstructTerraformComponentPlanfilePath(config, info)
	// A plan file the user named is theirs even when it is the canonical path, so it is kept.
	if info.PlanFile == "" || info.ConfigurationPlanImplicit || filepath.Clean(configurationPlanPath(config, info)) != filepath.Clean(canonical) {
		removeTerraformFile(canonical, "Failed to remove consumed plan file")
	}
	removeTerraformFile(canonical+configurationPlanSuffix, "Failed to remove consumed plan record")
	if info.PlanFile != "" {
		removeTerraformFile(configurationPlanPath(config, info)+configurationPlanSuffix, "Failed to remove consumed plan record")
	}
}
