package terraform

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/component"
	cfg "github.com/cloudposse/atmos/pkg/config"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	provWorkdir "github.com/cloudposse/atmos/pkg/provisioner/workdir"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/terraform/plandiff"
)

// VerifyPlanfile verifies that a stored planfile matches the current state before applying.
// It generates a fresh plan and compares it to the stored one using the plan-diff infrastructure.
// The storedPlanFile parameter is the path to the stored planfile to verify against.
// On a match (or when mode is warn) info.PlanFile is set to the fresh planfile path and
// info.UseTerraformPlan is set to true so the subsequent apply uses the verified fresh plan.
// The stored planfile is cleaned up. When drift is detected and mode is fail, it returns an
// error wrapping ErrPlanVerificationFailed; when mode is warn, it logs the drift and proceeds.
func (s *Operations) VerifyPlanfile(info *schema.ConfigAndStacksInfo, storedPlanFile string, mode schema.PlanfileVerifyMode) error {
	defer perf.Track(nil, "exec.VerifyPlanfile")()

	// Validate stored planfile exists on disk.
	if _, err := os.Stat(storedPlanFile); os.IsNotExist(err) {
		return fmt.Errorf("%w: stored planfile does not exist: %s", errUtils.ErrPlanVerificationFailed, storedPlanFile)
	}

	vc, err := s.resolveVerificationContext(info)
	if err != nil {
		return err
	}

	policy, err := plandiff.Policy(&vc.atmosConfig)
	if err != nil {
		return err
	}
	plans, err := s.loadStoredAndFreshPlans(&vc.atmosConfig, &vc.processedInfo, vc.componentPath, storedPlanFile)
	if err != nil {
		os.Remove(storedPlanFile)
		return err
	}

	// Warn about ignore_changes entries that cannot match anything in these plans.
	plandiff.WarnUnmatchedIgnoreChanges(vc.atmosConfig.Components.Terraform.PlanDiff.IgnoreChanges, plans.stored, plans.fresh)

	// Generate diff. The stored planfile has served its purpose either way.
	diff, hasDiff := plandiff.Compare(plans.stored, plans.fresh, policy)
	os.Remove(storedPlanFile)

	return finalizeVerification(info, plans.freshPlanFile, diff, hasDiff, mode)
}

// verificationContext holds the resolved config, processed component info, and
// component working directory needed to generate the fresh plan for comparison.
type verificationContext struct {
	atmosConfig   schema.AtmosConfiguration
	processedInfo schema.ConfigAndStacksInfo
	componentPath string
}

// resolveVerificationContext initializes the CLI config, processes stacks, and
// resolves the component working directory used to generate the fresh plan.
func (s *Operations) resolveVerificationContext(info *schema.ConfigAndStacksInfo) (verificationContext, error) {
	defer perf.Track(nil, "exec.resolveVerificationContext")()

	vc := verificationContext{}

	atmosConfig, err := cfg.InitCliConfig(*info, true)
	if err != nil {
		return vc, fmt.Errorf("failed to initialize CLI config for plan verification: %w", err)
	}
	vc.atmosConfig = atmosConfig

	// Process stacks to resolve component metadata (metadata.component, component_folder_prefix).
	processedInfo, err := s.host.ProcessStacks(&atmosConfig, *info, true, true, true, nil, nil)
	if err != nil {
		return vc, fmt.Errorf("error processing stacks for plan verification: %w", err)
	}
	vc.processedInfo = processedInfo

	vc.componentPath = filepath.Join(atmosConfig.TerraformDirAbsolutePath, processedInfo.ComponentFolderPrefix, processedInfo.FinalComponent)

	// For workdir-enabled components, compute the workdir path from first principles
	// because ProcessStacks builds a fresh ComponentSection that does not include
	// WorkdirPathKey (only set by AutoProvisionSource at execution time).
	if provWorkdir.IsWorkdirEnabled(processedInfo.ComponentSection) {
		candidate, exists, resolveErr := component.BuildAndResolveWorkdirPath(&atmosConfig, &processedInfo, cfg.TerraformComponentType)
		if resolveErr != nil {
			return vc, resolveErr
		}
		if exists {
			vc.componentPath = candidate
		}
	}

	return vc, nil
}

// comparisonPlans holds the parsed stored and fresh plan JSON (and the fresh
// plan file path) used for the drift comparison.
type comparisonPlans struct {
	stored        map[string]interface{}
	fresh         map[string]interface{}
	freshPlanFile string
}

// loadStoredAndFreshPlans returns the parsed JSON of the stored plan and of a
// freshly generated plan (plus the fresh plan file path) for comparison.
func (s *Operations) loadStoredAndFreshPlans(atmosConfig *schema.AtmosConfiguration, processedInfo *schema.ConfigAndStacksInfo, componentPath, storedPlanFile string) (comparisonPlans, error) {
	defer perf.Track(nil, "exec.loadStoredAndFreshPlans")()

	var plans comparisonPlans

	storedPlanJSON, err := s.getTerraformPlanJSON(atmosConfig, processedInfo, componentPath, storedPlanFile)
	if err != nil {
		return plans, fmt.Errorf("error getting JSON for stored plan: %w", err)
	}

	// Generate a fresh plan in the component dir, where terraform expects it, rather than in a temp
	// dir. For a workdir-enabled component that is its workdir, which componentPath already is.
	plans.freshPlanFile, err = s.GenerateNewPlanFile(atmosConfig, processedInfo, componentPath, componentPath)
	if err != nil {
		return plans, fmt.Errorf("error generating fresh plan for verification: %w", err)
	}

	freshPlanJSON, err := s.getTerraformPlanJSON(atmosConfig, processedInfo, componentPath, plans.freshPlanFile)
	if err != nil {
		return plans, fmt.Errorf("error getting JSON for fresh plan: %w", err)
	}

	if err := json.Unmarshal([]byte(storedPlanJSON), &plans.stored); err != nil {
		return plans, fmt.Errorf("error parsing stored plan JSON: %w", err)
	}
	if err := json.Unmarshal([]byte(freshPlanJSON), &plans.fresh); err != nil {
		return plans, fmt.Errorf("error parsing fresh plan JSON: %w", err)
	}

	return plans, nil
}

// finalizeVerification applies the verification verdict. On drift with mode fail
// it returns ErrPlanVerificationFailed; otherwise (match, or drift under warn) it
// selects the freshly generated plan for the subsequent apply, logging a warning
// when proceeding despite drift.
func finalizeVerification(info *schema.ConfigAndStacksInfo, freshPlanFile, diff string, hasDiff bool, mode schema.PlanfileVerifyMode) error {
	if hasDiff && mode == schema.PlanfileVerifyFail {
		return fmt.Errorf("%w:\n%s", errUtils.ErrPlanVerificationFailed, diff)
	}

	// Match, or warn-on-drift: apply the freshly generated plan (current state).
	info.PlanFile = freshPlanFile
	info.UseTerraformPlan = true

	if hasDiff {
		// mode == warn: surface the drift but do not block the deploy.
		log.Warn("Plan verification detected drift; proceeding because verify mode is 'warn'", "diff", diff)
	} else {
		log.Info("Plan verification passed: stored plan matches current state")
	}
	return nil
}

// withoutContent copies files, dropping the bytes of every one.
func withoutContent(files []schema.ComponentConfigurationFile) []schema.ComponentConfigurationFile {
	stored := slices.Clone(files)
	for i := range stored {
		stored[i].Content = nil
	}
	return stored
}

// restoreSensitiveContent reads the bytes of each sensitive file back from the working directory and
// restores them only when they are the reviewed ones. The working directory digest excludes some
// locations, so the reviewed bytes are verified here against the digest recorded with the plan.
func restoreSensitiveContent(workdir string, files []schema.ComponentConfigurationFile, reviewed string) error {
	contents, err := readSensitiveFiles(workdir, files)
	if err != nil {
		return err
	}
	if contents == nil {
		return nil
	}
	// The digest binds absence as well as bytes: a file that was already absent when the plan was
	// reviewed and is still absent matches, while one that disappeared or appeared since does not.
	if digestSensitiveContents(contents) != reviewed {
		for i := range files {
			// A file the entry removes is absent by design, so only other policies report it as missing.
			if files[i].Sensitive && contents[files[i].Path] == nil && !slices.Contains([]string{"remove", "remove_terragrunt"}, files[i].IfExists) {
				return fmt.Errorf("%w: reviewed generated file %s is missing: %w", errUtils.ErrPlanVerificationFailed, files[i].Path, fs.ErrNotExist)
			}
		}
		return fmt.Errorf("%w: reviewed generated file changed", errUtils.ErrPlanVerificationFailed)
	}
	for i := range files {
		if files[i].Sensitive {
			files[i].Content = contents[files[i].Path]
		}
	}
	return nil
}

// sensitiveDigest fingerprints the sensitive generated files as they are in the working directory.
// It is empty when no file is sensitive.
func sensitiveDigest(workdir string, files []schema.ComponentConfigurationFile) (string, error) {
	contents, err := readSensitiveFiles(workdir, files)
	if err != nil || contents == nil {
		return "", err
	}
	return digestSensitiveContents(contents), nil
}

// readSensitiveFiles reads every sensitive file below workdir, keyed by path. A missing file maps
// to nil content. It returns nil when no file is sensitive.
func readSensitiveFiles(workdir string, files []schema.ComponentConfigurationFile) (map[string][]byte, error) {
	if !slices.ContainsFunc(files, func(file schema.ComponentConfigurationFile) bool { return file.Sensitive }) {
		return nil, nil
	}
	root, err := os.OpenRoot(workdir)
	if err != nil {
		return nil, planIOError(err)
	}
	defer root.Close()
	contents := make(map[string][]byte)
	for i := range files {
		if !files[i].Sensitive {
			continue
		}
		content, err := root.ReadFile(files[i].Path)
		if errors.Is(err, fs.ErrNotExist) {
			contents[files[i].Path] = nil
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("%w: reviewed generated file %s cannot be read: %w", errUtils.ErrPlanVerificationFailed, files[i].Path, err)
		}
		if content == nil {
			content = []byte{}
		}
		contents[files[i].Path] = content
	}
	return contents, nil
}

// digestSensitiveContents hashes the paths and bytes of the sensitive files in a stable order.
func digestSensitiveContents(contents map[string][]byte) string {
	hash := sha256.New()
	for _, path := range slices.Sorted(maps.Keys(contents)) {
		if contents[path] == nil {
			fmt.Fprintf(hash, "%s\x00absent\n", path)
			continue
		}
		fmt.Fprintf(hash, "%s\x00%x\n", path, sha256.Sum256(contents[path]))
	}
	return hex.EncodeToString(hash.Sum(nil))
}

// stashStalePlan keeps a private copy of a plan that already exists at the plan output path and
// empties the file in place, so a non-empty file found there after the subprocess was written by that
// run. Emptying in place keeps the file's mode and follows a symlink, so Terraform writes the new plan
// exactly where it would have without the stash, and a private plan is never recreated with broader
// permissions. The returned settle function must run once the observation is complete: it discards
// the copy when the run wrote a new plan and otherwise restores the earlier contents. A directory or
// other non-regular output is rejected without being touched.
func stashStalePlan(root, path string) (func() error, error) {
	noop := func() error { return nil }
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return noop, nil
		}
		return noop, fmt.Errorf(errWrapFmt, errUtils.ErrPlanVerificationFailed, err)
	}
	if !info.Mode().IsRegular() {
		return noop, fmt.Errorf("%w: plan output %s is not a regular file", errUtils.ErrPlanVerificationFailed, path)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return noop, fmt.Errorf(errWrapFmt, errUtils.ErrPlanVerificationFailed, err)
	}
	// The stash directory is private and skipped by the working directory digest.
	dir, err := os.MkdirTemp(stalePlanStashParent(root, path), stalePlanDirPrefix)
	if err != nil {
		return noop, fmt.Errorf(errWrapFmt, errUtils.ErrPlanVerificationFailed, err)
	}
	stashed := filepath.Join(dir, "plan")
	if err := os.WriteFile(stashed, data, planFileMode); err != nil { //nolint:gosec // G703: stashed is inside a directory from os.MkdirTemp.
		_ = os.RemoveAll(dir)
		return noop, fmt.Errorf(errWrapFmt, errUtils.ErrPlanVerificationFailed, err)
	}
	if err := os.Truncate(path, 0); err != nil {
		_ = os.RemoveAll(dir)
		return noop, fmt.Errorf(errWrapFmt, errUtils.ErrPlanVerificationFailed, err)
	}
	return func() error { return settleStalePlan(path, stashed, dir, info.Mode().Perm()) }, nil
}

// settleStalePlan discards the copy when the run wrote a new plan and otherwise writes the earlier
// contents back through the same path. The copy is removed only after the restore succeeds or a
// replacement is confirmed; otherwise it is kept for recovery and the error names it.
func settleStalePlan(path, stashed, dir string, mode os.FileMode) error {
	info, err := os.Stat(path)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("%w: earlier plan kept at %s: %w", errUtils.ErrPlanVerificationFailed, stashed, err)
	}
	if err != nil || info.Size() == 0 {
		if err := restoreStalePlan(path, stashed, mode); err != nil {
			return fmt.Errorf("%w: restore the earlier plan from %s: %w", errUtils.ErrPlanVerificationFailed, stashed, err)
		}
	}
	if err := os.RemoveAll(dir); err != nil {
		return fmt.Errorf(errWrapFmt, errUtils.ErrPlanVerificationFailed, err)
	}
	return nil
}

// restoreStalePlan writes the stashed contents back to the plan path with its original mode.
func restoreStalePlan(path, stashed string, mode os.FileMode) error {
	data, err := os.ReadFile(stashed)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, mode) //nolint:gosec // G703: path is the plan output Terraform was told to write.
}

// writtenPlanDigest fingerprints the plan the run wrote. A stale plan is emptied before the run and
// Terraform never writes an empty plan, so an empty file means this run wrote nothing.
func writtenPlanDigest(path string) (string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return "", planIOError(err)
	}
	if info.Size() == 0 {
		return "", fmt.Errorf("%w: Terraform did not write the plan %s", errUtils.ErrPlanVerificationFailed, path)
	}
	return fileDigest(path)
}

// stalePlanStashParent returns where the stash directory is created: beside the plan, except inside
// downloaded modules, whose every file is digested, where it moves to the component directory.
func stalePlanStashParent(root, path string) string {
	parent := filepath.Dir(path)
	if root != "" && insideTerraformModules(root, parent) {
		return root
	}
	return parent
}

// stalePlanDirPrefix names the directory that holds a plan found at the output path before a run.
const stalePlanDirPrefix = ".atmos-stale-plan-"
