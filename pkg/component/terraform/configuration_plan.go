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
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

const configurationPlanSuffix = ".atmos.json"

// configurationPlanVersion is bumped whenever the stored binding changes shape.
const configurationPlanVersion = 2

// configurationPlan is the private record stored beside a saved plan. It binds the plan to the
// exact preparation it was made from. Evaluated inputs and backend settings are not stored:
// the plan file already embodies the inputs, and the backend is re-resolved at apply time and
// compared by BackendDigest, so credentials in backend settings never reach this file.
type configurationPlan struct {
	Version                                                                               int
	Stack, Component, Identity, EngineDigest, PlanDigest, WorkingDirectory, WorkingDigest string
	BackendDigest                                                                         string
	Prepared                                                                              *schema.PreparedComponentConfiguration
}

// ownsConfigurationPlan reports whether a provider manages this component's saved plans.
// Their plan files are reviewed artifacts bound to a record, not ephemeral scratch files.
func ownsConfigurationPlan(info *schema.ConfigAndStacksInfo) bool {
	_, owned := info.ComponentSettingsSection["configuration"]
	return owned
}

func usesConfigurationPlan(info *schema.ConfigAndStacksInfo) bool {
	return info.UseTerraformPlan || info.PlanFile != "" || info.StoredPlanFile != ""
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
// The original execution directory is retained; transport to another runner requires an artifact adapter.
func restoreConfigurationPlan(ctx context.Context, config *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo) (*schema.PreparedComponentConfiguration, error) {
	if !restoresConfigurationPlan(info) {
		return nil, fmt.Errorf("%w: plugin saved plans require direct apply of the original artifact", errUtils.ErrPluginUnsupportedExtension)
	}
	runtime, err := planRuntime(config)
	if err != nil {
		return nil, err
	}
	path := configurationPlanPath(config, info)
	plan, err := loadConfigurationPlan(path, info)
	if err != nil {
		return nil, err
	}
	identity, err := runtime.PlanIdentity(ctx, info, plan.Prepared.Sources)
	if err != nil {
		return nil, err
	}
	artifactMatches, err := configurationArtifactMatches(config, info, plan, path)
	if err != nil {
		return nil, err
	}
	if identity != plan.Identity || !artifactMatches {
		return nil, fmt.Errorf("%w: plugin, source, engine, workdir, or reviewed artifact changed; create a new plan", errUtils.ErrPlanVerificationFailed)
	}
	if err := verifyReviewedBackendAndWorkdir(ctx, config, info, plan, path); err != nil {
		return nil, err
	}
	// Terraform runs inside the workdir, so a path relative to the project would not resolve there.
	absolute, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	info.UseTerraformPlan = true
	info.PlanFile = absolute
	return plan.Prepared, nil
}

// verifyReviewedBackendAndWorkdir rejects a changed backend identity or a changed retained working directory.
func verifyReviewedBackendAndWorkdir(ctx context.Context, config *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo, plan *configurationPlan, path string) error {
	if err := verifyConfigurationPlanState(ctx, config, info, plan); err != nil {
		return err
	}
	return verifyConfigurationPlanWorkdir(plan, path)
}

// restoresConfigurationPlan reports whether the invocation applies the original, locally stored artifact.
func restoresConfigurationPlan(info *schema.ConfigAndStacksInfo) bool {
	return info.StoredPlanFile == "" && (info.SubCommand == "apply" || info.SubCommand == "deploy")
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

// verifyConfigurationPlanWorkdir rejects a working directory whose retained bytes changed since review.
func verifyConfigurationPlanWorkdir(plan *configurationPlan, path string) error {
	workingDigest, err := configurationWorkdirDigest(plan.WorkingDirectory, path)
	if err != nil {
		return err
	}
	if workingDigest != plan.WorkingDigest {
		return fmt.Errorf("%w: reviewed working directory changed", errUtils.ErrPlanVerificationFailed)
	}
	return nil
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
	plan, err := newConfigurationPlan(info, execCtx, identity, path)
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
func newConfigurationPlan(info *schema.ConfigAndStacksInfo, execCtx *componentExecContext, identity, path string) (*configurationPlan, error) {
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
	// Inputs and backend values stay out of the record; see configurationPlan.
	stored := *execCtx.preparedConfiguration
	stored.Inputs, stored.Backend = nil, nil
	return &configurationPlan{WorkingDigest: workingDigest, Version: configurationPlanVersion, Stack: info.Stack, Component: info.ComponentFromArg, Identity: identity, EngineDigest: engineDigest, PlanDigest: planDigest, WorkingDirectory: filepath.Clean(execCtx.workingDir), BackendDigest: backendDigest, Prepared: &stored}, nil
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

func plannedOutputPath(directory string, args []string) string {
	for i, arg := range args {
		path := ""
		if arg == "-out" && i+1 < len(args) {
			path = args[i+1]
		}
		if strings.HasPrefix(arg, "-out=") {
			path = strings.TrimPrefix(arg, "-out=")
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

func fileDigest(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	h := sha256.New()
	if _, err := io.Copy(h, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func writePlanBinding(path string, data []byte) error {
	file, err := os.CreateTemp(filepath.Dir(path), ".atmos-plan-*")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	if _, err := file.Write(data); err != nil {
		file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	return os.Rename(file.Name(), path)
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

// configurationWorkdirDigest binds retained module/configuration bytes, excluding mutable execution data.
func configurationWorkdirDigest(root, planPath string) (string, error) {
	// An explicit plan path may be absolute while the directory is relative, or the reverse.
	// Both must compare equal, or the plan file itself would be hashed into the directory digest.
	root, planPath, err := absolutePaths(root, planPath)
	if err != nil {
		return "", err
	}
	files := make(map[string]string)
	err = filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			if skipWorkdirDirectory(root, path, entry.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if ignoredWorkdirFile(path, planPath, entry.Name()) {
			return nil
		}
		return recordWorkdirFile(root, path, entry, files)
	})
	if err != nil {
		return "", err
	}
	data, err := json.Marshal(files)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:]), nil
}

// absolutePaths returns both paths absolute and with symlinks resolved, so one file reached through
// two spellings (for example /var and /private/var on macOS) compares equal.
func absolutePaths(root, planPath string) (string, string, error) {
	absoluteRoot, err := resolvedPath(root)
	if err != nil {
		return "", "", err
	}
	absolutePlan, err := resolvedPath(planPath)
	if err != nil {
		return "", "", err
	}
	return absoluteRoot, absolutePlan, nil
}

func resolvedPath(path string) (string, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	if resolved, err := filepath.EvalSymlinks(absolute); err == nil {
		return resolved, nil
	}
	return absolute, nil
}

// skipWorkdirDirectory reports whether a nested directory holds only mutable or cached data.
// The .atmos directory is provisioner bookkeeping (access timestamps and metadata lock files
// that even a read-only lookup of the retained directory creates), never executed content.
func skipWorkdirDirectory(root, path, name string) bool {
	return path != root && (name == ".terraform" || name == ".git" || name == ".terragrunt-cache" || name == ".atmos")
}

// ignoredWorkdirFile reports whether a file is the plan itself, its binding, or mutable state.
func ignoredWorkdirFile(path, planPath, name string) bool {
	return path == planPath || strings.HasSuffix(name, configurationPlanSuffix) || strings.HasPrefix(name, ".atmos-plan-") || strings.HasPrefix(name, "terraform.tfstate")
}

// recordWorkdirFile digests one retained file by its slash-separated path relative to the root.
func recordWorkdirFile(root, path string, entry fs.DirEntry, files map[string]string) error {
	if entry.Type()&os.ModeSymlink != 0 {
		return fmt.Errorf("%w: saved-plan workdir contains symlink", errUtils.ErrPluginUnsupportedExtension)
	}
	digest, err := fileDigest(path)
	if err != nil {
		return err
	}
	relative, err := filepath.Rel(root, path)
	if err != nil {
		return err
	}
	files[filepath.ToSlash(relative)] = digest
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
	if _, changes := stateChangingSubcommands[info.SubCommand]; !changes {
		return
	}
	canonical := ConstructTerraformComponentPlanfilePath(config, info)
	removeTerraformFile(canonical, "Failed to remove consumed plan file")
	removeTerraformFile(canonical+configurationPlanSuffix, "Failed to remove consumed plan record")
	if info.PlanFile != "" {
		removeTerraformFile(configurationPlanPath(config, info)+configurationPlanSuffix, "Failed to remove consumed plan record")
	}
}
