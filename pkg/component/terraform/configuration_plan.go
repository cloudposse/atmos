package terraform

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"math/big"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"slices"
	"strings"

	"github.com/google/shlex"

	errUtils "github.com/cloudposse/atmos/errors"
	envpkg "github.com/cloudposse/atmos/pkg/env"
	"github.com/cloudposse/atmos/pkg/schema"
)

const configurationPlanSuffix = ".atmos.json"

// subcommandPlan names the Terraform plan command.
const subcommandPlan = "plan"

// planBindingSuffix names the binding of a default saved plan, which Atmos writes as "*.planfile".
const planBindingSuffix = ".planfile" + configurationPlanSuffix

type configurationPlan struct {
	Version                                                                               int
	Stack, Component, Identity, EngineDigest, PlanDigest, WorkingDirectory, WorkingDigest string
	Prepared                                                                              *schema.PreparedComponentConfiguration
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
	data, err := os.ReadFile(path + configurationPlanSuffix)
	if err != nil {
		return nil, fmt.Errorf("%w: plugin plan binding is missing: %w", errUtils.ErrPlanVerificationFailed, err)
	}
	var plan configurationPlan
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.UseNumber()
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&plan); err != nil {
		return nil, errUtils.ErrPlanVerificationFailed
	}
	if plan.Version != 1 || plan.Prepared == nil || plan.Stack != info.Stack || plan.Component != info.ComponentFromArg {
		return nil, errUtils.ErrPlanVerificationFailed
	}
	return &plan, nil
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

// recordConfigurationPlan stores a private companion beside the completed plan. It binds the
// configuration Terraform read, which the plan snapshot captured before any after hook ran.
func recordConfigurationPlan(ctx context.Context, config *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo, execCtx *componentExecContext, args []string) error {
	path := recordedPlanPath(info, execCtx, args)
	if path == "" {
		return nil
	}
	snapshot, err := execCtx.reviewedSnapshot(ctx, config, info, path)
	if err != nil {
		return err
	}
	plan, err := newConfigurationPlan(info, execCtx, snapshot, path)
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
	if !bindsPlan(info, execCtx) {
		return ""
	}
	return plannedOutputPath(execCtx.componentPath, args)
}

// bindsPlan reports whether this invocation produces a plan that is bound to its configuration.
func bindsPlan(info *schema.ConfigAndStacksInfo, execCtx *componentExecContext) bool {
	return execCtx.preparedConfiguration != nil && !info.DryRun && info.SubCommand == subcommandPlan
}

// newConfigurationPlan binds the snapshot to the digests of the plan and engine.
func newConfigurationPlan(info *schema.ConfigAndStacksInfo, execCtx *componentExecContext, snapshot *planSnapshot, path string) (*configurationPlan, error) {
	planDigest, err := fileDigest(path)
	if err != nil {
		return nil, err
	}
	if snapshot.planDigest != "" && snapshot.planDigest != planDigest {
		return nil, fmt.Errorf("%w: plan file changed after Terraform wrote it", errUtils.ErrPlanVerificationFailed)
	}
	engineDigest, err := fileDigest(info.Command)
	if err != nil {
		return nil, err
	}
	return &configurationPlan{WorkingDigest: snapshot.workingDigest, Version: 1, Stack: info.Stack, Component: info.ComponentFromArg, Identity: snapshot.identity, EngineDigest: engineDigest, PlanDigest: planDigest, WorkingDirectory: filepath.Clean(execCtx.workingDir), Prepared: execCtx.preparedConfiguration}, nil
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
	return rejectEnvironmentPlanOutput(planEnvironment(atmosConfig, info), runtime.GOOS)
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
	file, err := os.CreateTemp(filepath.Dir(path), ".atmos-plan-*")
	if err != nil {
		return planIOError(err)
	}
	defer os.Remove(file.Name())
	if _, err := file.Write(data); err != nil {
		file.Close()
		return planIOError(err)
	}
	if err := file.Close(); err != nil {
		return planIOError(err)
	}
	if err := os.Rename(file.Name(), path); err != nil {
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
	if state == nil || state.BackendType != plan.Prepared.BackendType || state.Workspace != plan.Prepared.Workspace || !equalBackendConfiguration(state.Backend, plan.Prepared.Backend) {
		return fmt.Errorf("%w: reviewed backend identity changed", errUtils.ErrPlanVerificationFailed)
	}
	return nil
}

// excludedWorkdirFile reports a file the working directory digest ignores: the plan and its
// binding, Terraform's cache outside downloaded modules, and runtime files outside modules.
func excludedWorkdirFile(root, path, planPath, name string) bool {
	if path == planPath || path == planPath+configurationPlanSuffix || insideTerraformCacheOutsideModules(root, path) {
		return true
	}
	return !insideTerraformModules(root, path) && ignoredWorkdirFile(path, planPath, name)
}

// configurationWorkdirDigest binds retained module/configuration bytes, excluding mutable execution data.
func configurationWorkdirDigest(root, planPath string) (string, error) {
	files := make(map[string]string)
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			if skipWorkdirDirectory(root, path, entry.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if excludedWorkdirFile(root, path, planPath, entry.Name()) {
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

// skipWorkdirDirectory reports whether a nested directory holds only mutable or cached data.
// The .atmos directory is provisioner bookkeeping (access timestamps and metadata lock files
// that even a read-only lookup of the retained directory creates), never executed content. The
// private plan directory of a plan policy exists only while Terraform runs.
// Name-based exclusions apply only outside downloaded modules: a module's own subdirectories
// are content Terraform may execute during apply, whatever they are called.
func skipWorkdirDirectory(root, path, name string) bool {
	if path == root {
		return false
	}
	if insideTerraformCacheOutsideModules(root, path) {
		return true
	}
	return !insideTerraformModules(root, path) && (name == ".git" || name == ".terragrunt-cache" || name == ".atmos" || strings.HasPrefix(name, ".atmos-policy-plan-"))
}

// Terraform's downloaded modules can contain scripts executed during apply. Other cache
// content, including providers and state, remains mutable and is excluded.
func insideTerraformCacheOutsideModules(root, path string) bool {
	relative, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	parts := strings.Split(relative, string(filepath.Separator))
	for i, part := range parts {
		if part == ".terraform" && i+1 < len(parts) && parts[i+1] != "modules" {
			return true
		}
	}
	return false
}

func insideTerraformModules(root, path string) bool {
	relative, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	parts := strings.Split(relative, string(filepath.Separator))
	for i, part := range parts {
		if part == ".terraform" && i+1 < len(parts) && parts[i+1] == "modules" {
			return true
		}
	}
	return false
}

// ignoredWorkdirFile reports whether a file is the plan itself, a plan binding, or mutable state.
// Only actual bindings are skipped: this plan's own, or one beside a default ".planfile" plan.
// An ordinary file such as settings.atmos.json is configuration and stays in the digest.
func ignoredWorkdirFile(path, planPath, name string) bool {
	return path == planPath || path == planPath+configurationPlanSuffix || strings.HasSuffix(name, planBindingSuffix) ||
		strings.HasPrefix(name, ".atmos-plan-") || strings.HasPrefix(name, "terraform.tfstate")
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

// equalBackendConfiguration compares JSON backend values without losing numeric precision.
func equalBackendConfiguration(left, right map[string]any) bool {
	a, errA := normalizedBackend(left)
	b, errB := normalizedBackend(right)
	return errA == nil && errB == nil && reflect.DeepEqual(a, b)
}

func normalizedBackend(value map[string]any) (any, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	var decoded any
	if err := decoder.Decode(&decoded); err != nil {
		return nil, err
	}
	return normalizeBackendNumbers(decoded), nil
}

func normalizeBackendNumbers(value any) any {
	switch typed := value.(type) {
	case json.Number:
		number, ok := new(big.Rat).SetString(string(typed))
		if ok {
			return number
		}
	case map[string]any:
		for key, child := range typed {
			typed[key] = normalizeBackendNumbers(child)
		}
	case []any:
		for index, child := range typed {
			typed[index] = normalizeBackendNumbers(child)
		}
	}
	return value
}
