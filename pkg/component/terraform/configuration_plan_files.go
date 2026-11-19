package terraform

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

// publicFileRecord binds one non-sensitive generated file without storing its bytes.
// A provider can mark a file public yet have a before-hook put a private value in it, so the
// record keeps only a presence marker and a digest; apply reads the bytes back from the workdir.
type publicFileRecord struct {
	// Present distinguishes an absent file from an empty one.
	Present bool
	// Digest is the hex SHA-256 of the observed bytes; empty when the file is absent.
	Digest string
}

// isRemovalPolicy reports whether the file policy deletes the file instead of writing it.
func isRemovalPolicy(policy string) bool {
	return policy == "remove" || policy == "remove_terragrunt"
}

// observePublicFile reads one file below root and describes it for the record.
func observePublicFile(root *os.Root, path string) (publicFileRecord, error) {
	data, err := root.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return publicFileRecord{}, nil
	}
	if err != nil {
		return publicFileRecord{}, planIOError(err)
	}
	sum := sha256.Sum256(data)
	return publicFileRecord{Present: true, Digest: hex.EncodeToString(sum[:])}, nil
}

// snapshotPublicFiles records the presence and digest of retained generated files after before-hooks have run.
// Neither sensitive nor public bytes enter the record: the Sensitive flag cannot be trusted to cover
// values a hook writes later, so apply verifies the workdir bytes against these digests instead.
func snapshotPublicFiles(workdir string, files []schema.ComponentConfigurationFile) (string, error) {
	root, err := os.OpenRoot(workdir)
	if err != nil {
		return "", planIOError(err)
	}
	defer root.Close()
	records := make(map[string]publicFileRecord)
	for _, file := range files {
		if file.Sensitive || isRemovalPolicy(file.IfExists) {
			continue
		}
		record, err := observePublicFile(root, file.Path)
		if err != nil {
			return "", err
		}
		records[file.Path] = record
	}
	data, err := json.Marshal(records)
	return string(data), err
}

// restorePublicContent reads each recorded public file back from the retained workdir and accepts it
// only when presence and bytes match the record. Absent files keep the removal policy set at record time.
func restorePublicContent(workdir string, files []schema.ComponentConfigurationFile, records map[string]publicFileRecord) error {
	root, err := os.OpenRoot(workdir)
	if err != nil {
		return planIOError(err)
	}
	defer root.Close()
	for i := range files {
		if files[i].Sensitive {
			continue
		}
		record, ok := records[files[i].Path]
		if !ok {
			if isRemovalPolicy(files[i].IfExists) {
				continue
			}
			return fmt.Errorf("%w: reviewed generated file %s has no recorded digest", errUtils.ErrPlanVerificationFailed, files[i].Path)
		}
		data, err := verifyPublicFile(root, files[i].Path, record)
		if err != nil {
			return err
		}
		files[i].Content = data
	}
	return nil
}

// verifyPublicFile returns the workdir bytes of a public file after checking them against its record.
func verifyPublicFile(root *os.Root, path string, record publicFileRecord) ([]byte, error) {
	data, err := root.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		if record.Present {
			return nil, fmt.Errorf("%w: reviewed generated file %s is missing: %w", errUtils.ErrPlanVerificationFailed, path, err)
		}
		return nil, nil
	}
	if err != nil {
		return nil, planIOError(err)
	}
	sum := sha256.Sum256(data)
	if !record.Present || hex.EncodeToString(sum[:]) != record.Digest {
		return nil, fmt.Errorf("%w: reviewed generated file %s changed", errUtils.ErrPlanVerificationFailed, path)
	}
	if data == nil {
		data = []byte{}
	}
	return data, nil
}

// planStackEnvironmentDigest binds overrides without persisting evaluated values.
// Explicit apply execution retains its provider defaults and is filtered per invocation.
func planStackEnvironmentDigest(info *schema.ConfigAndStacksInfo, prepared *schema.PreparedComponentConfiguration) (string, error) {
	if prepared.Execution == nil || prepared.Execution.Apply != nil {
		return "", nil
	}
	return backendIdentityDigest(info.ComponentEnvSection)
}

func verifyPlanStackEnvironment(info *schema.ConfigAndStacksInfo, plan *configurationPlan) error {
	digest, err := planStackEnvironmentDigest(info, plan.Prepared)
	if err != nil {
		return err
	}
	if digest != plan.StackEnvironmentDigest {
		return fmt.Errorf("%w: reviewed stack environment overrides changed; create a new plan", errUtils.ErrPlanVerificationFailed)
	}
	return nil
}

// settle restores or discards the stashed plan. A plan that could not be restored stays in its stash,
// and the binding fails naming it. When Terraform also failed, the stash location joins its error:
// the pipeline returns that error before the binding is read, and the exit code the hooks classify is
// unchanged.
func (s *planSnapshotter) settle(settleStale func() error, runResult error) error {
	err := settleStale()
	if err == nil {
		return runResult
	}
	s.result, s.err = nil, err
	if runResult != nil {
		return errors.Join(runResult, err)
	}
	return nil
}

// observe runs Terraform between two configuration snapshots and binds the plan it wrote.
func (s *planSnapshotter) observe(path, writtenPath string, run func() error, failed func(error) bool) error {
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
	case failed(runErr):
		return runErr
	default:
		// Fingerprint the actual engine output before after hooks, including private policy plans.
		digest, digestErr := writtenPlanDigest(writtenPath)
		if digestErr != nil {
			s.err = digestErr
			return digestErr
		}
		after.planDigest = digest
		s.result = after
	}
	return runErr
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
