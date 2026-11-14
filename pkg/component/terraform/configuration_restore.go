package terraform

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/component"
	"github.com/cloudposse/atmos/pkg/component/configuration"
	"github.com/cloudposse/atmos/pkg/provisioner"
	"github.com/cloudposse/atmos/pkg/schema"
)

// terraformLockFile is the provider lock file. It travels with the plan because Terraform checks the
// plan's provider selections against it, so a different lock can change which providers apply.
const terraformLockFile = ".terraform.lock.hcl"

// workdirMode is the permission of a re-created working directory.
const workdirMode = 0o755

// reviewedRebuild describes a reviewed plan that reached a runner without its working directory:
// the plan and its record were restored (for example from the planfile store into a fresh checkout)
// but nothing else of the original run was. The workdir is re-provisioned from the checkout and must
// then equal what was reviewed.
//
// What is verified before anything is rebuilt: the record's stack and component, the plan bytes, the
// engine binary, the provider identity with its options and watched source files, and the backend.
// What is rebuilt, never trusted: the working directory. The module source is provisioned again from
// the checkout and the generated files are written again from a fresh preparation, then the result
// must match RebuildDigest (and the lock file LockDigest) from the record. Any mismatch rejects the plan.
type reviewedRebuild struct {
	plan      *configurationPlan
	planPath  string
	workdir   string
	transient []string
	// prepared is the fresh preparation the workdir is regenerated from.
	prepared *schema.PreparedComponentConfiguration
}

// newReviewedRebuild accepts a workdir that is absent or holds only the restored artifacts, and
// rejects anything else: files that are not part of the transport cannot be verified.
func newReviewedRebuild(info *schema.ConfigAndStacksInfo, plan *configurationPlan, planPath string) (*reviewedRebuild, error) {
	rebuild := &reviewedRebuild{plan: plan, planPath: planPath, workdir: plan.WorkingDirectory, transient: transientWorkdirFiles(info)}
	if plan.RebuildDigest == "" {
		return nil, fmt.Errorf("%w: reviewed working directory changed", errUtils.ErrPlanVerificationFailed)
	}
	only, err := rebuild.holdsOnlyTransport()
	if err != nil {
		return nil, err
	}
	if !only {
		return nil, errUtils.Build(fmt.Errorf("%w: reviewed working directory changed", errUtils.ErrPlanVerificationFailed)).
			WithExplanationf("The working directory `%s` holds files that were not part of the reviewed plan, so it cannot be verified.", plan.WorkingDirectory).
			WithHint("Remove the directory and restore only the plan, its record, and the lock file, or apply on the runner that created the plan.").
			Err()
	}
	return rebuild, nil
}

// rebuiltWorkdirMatches reports whether the workdir already equals the reviewed rebuild: the files a
// rebuild regenerates match RebuildDigest and the lock file matches LockDigest. An absent workdir or
// one that differs does not match; only an unreadable one is an error.
func rebuiltWorkdirMatches(info *schema.ConfigAndStacksInfo, plan *configurationPlan, planPath string) (bool, error) {
	if plan.RebuildDigest == "" {
		return false, nil
	}
	rebuild := &reviewedRebuild{plan: plan, planPath: planPath, workdir: plan.WorkingDirectory, transient: transientWorkdirFiles(info)}
	if only, err := rebuild.holdsOnlyTransport(); err != nil || only {
		// Nothing was rebuilt yet: the normal rebuild path applies.
		return false, err
	}
	err := rebuild.verify(plan.WorkingDirectory)
	switch {
	case err == nil:
		return true, nil
	case errors.Is(err, errUtils.ErrPlanVerificationFailed), errors.Is(err, fs.ErrNotExist):
		return false, nil
	}
	return false, err
}

// transientWorkdirFiles names the files each execution rewrites from its inputs. They carry values
// the plan file already embodies, so they are not part of what a rebuild must reproduce.
func transientWorkdirFiles(info *schema.ConfigAndStacksInfo) []string {
	var names []string
	for _, suffix := range []string{".terraform.tfvars.json", ".terraform.test.tfvars.json"} {
		if name, ok := configurationArtifactName(info, suffix); ok {
			names = append(names, name)
		}
	}
	return names
}

// configurationRebuildDigest hashes the files a rebuild regenerates: the retained directory without
// the plan, its record, state, variable files, and the lock file (which has its own digest).
func configurationRebuildDigest(root, planPath string, transient []string) (string, error) {
	return workdirDigest(root, planPath, func(relative string) bool {
		if relative == terraformLockFile {
			return true
		}
		for _, name := range transient {
			if relative == name {
				return true
			}
		}
		return false
	})
}

// lockFileDigest returns the digest of the provider lock file beside the plan, or "" when there is none.
func lockFileDigest(dir string) (string, error) {
	digest, err := fileDigest(filepath.Join(dir, terraformLockFile))
	if errors.Is(err, fs.ErrNotExist) {
		return "", nil
	}
	return digest, err
}

// transportFiles lists the restored artifacts that may sit in the workdir, as slash-separated paths relative
// to it: the lock file, and the plan with its record when the plan is inside the workdir, also in a
// subdirectory such as `plans/reviewed.planfile`.
func (r *reviewedRebuild) transportFiles() map[string]bool {
	files := map[string]bool{terraformLockFile: true}
	workdir, errWorkdir := resolvedPath(r.workdir)
	planDir, errPlan := resolvedPath(filepath.Dir(r.planPath))
	if errWorkdir != nil || errPlan != nil {
		return files
	}
	if relative, err := filepath.Rel(workdir, planDir); err == nil && filepath.IsLocal(relative) {
		plan := filepath.ToSlash(filepath.Join(relative, filepath.Base(r.planPath)))
		files[plan] = true
		files[plan+configurationPlanSuffix] = true
	}
	return files
}

func sameDirectory(first, second string) bool {
	a, errA := resolvedPath(first)
	b, errB := resolvedPath(second)
	return errA == nil && errB == nil && a == b
}

// holdsOnlyTransport reports whether the workdir is absent, empty, or contains just the restored artifacts.
// A subdirectory is accepted only on the way to an artifact, and then holds nothing else either.
func (r *reviewedRebuild) holdsOnlyTransport() (bool, error) {
	if _, err := os.Stat(r.workdir); errors.Is(err, fs.ErrNotExist) {
		return true, nil
	}
	allowed := r.transportFiles()
	only := true
	err := filepath.WalkDir(r.workdir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path == r.workdir {
			return nil
		}
		relative, err := filepath.Rel(r.workdir, path)
		if err != nil {
			return err
		}
		relative = filepath.ToSlash(relative)
		if entry.IsDir() {
			if !leadsToTransport(relative, allowed) {
				only = false
				return filepath.SkipAll
			}
			return nil
		}
		if !allowed[relative] {
			only = false
			return filepath.SkipAll
		}
		return nil
	})
	if err != nil {
		return false, err
	}
	return only, nil
}

// leadsToTransport reports whether a directory is a parent of one of the restored artifacts.
func leadsToTransport(directory string, allowed map[string]bool) bool {
	for name := range allowed {
		if strings.HasPrefix(name, directory+"/") {
			return true
		}
	}
	return false
}

// stage moves the restored artifacts aside so the workdir can be provisioned into an empty place, and
// returns the function that puts them back. Provisioning refuses a directory it does not own.
func (r *reviewedRebuild) stage() (func() error, error) {
	entries, err := os.ReadDir(r.workdir)
	if errors.Is(err, fs.ErrNotExist) || len(entries) == 0 {
		return func() error { return nil }, nil
	}
	if err != nil {
		return nil, err
	}
	holding, err := os.MkdirTemp(filepath.Dir(r.workdir), ".atmos-restore-*")
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if err := os.Rename(filepath.Join(r.workdir, entry.Name()), filepath.Join(holding, entry.Name())); err != nil {
			return nil, errors.Join(err, putBack(holding, r.workdir, names))
		}
		names = append(names, entry.Name())
	}
	if err := os.Remove(r.workdir); err != nil {
		return nil, errors.Join(err, putBack(holding, r.workdir, names))
	}
	return func() error { return putBack(holding, r.workdir, names) }, nil
}

// putBack returns staged artifacts to the workdir, replacing what provisioning wrote under the same
// name, and removes the holding directory.
func putBack(holding, workdir string, names []string) error {
	if err := os.MkdirAll(workdir, workdirMode); err != nil {
		return err
	}
	for _, name := range names {
		destination := filepath.Join(workdir, name)
		staged := filepath.Join(holding, name)
		if _, err := os.Lstat(staged); errors.Is(err, fs.ErrNotExist) {
			// An earlier restore attempt may already have moved this artifact.
			if _, destinationErr := os.Lstat(destination); destinationErr == nil {
				continue
			} else {
				return errors.Join(err, destinationErr)
			}
		} else if err != nil {
			return err
		}
		if err := os.Remove(destination); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
		if err := os.Rename(staged, destination); err != nil {
			return err
		}
	}
	return os.Remove(holding)
}

// verify compares the re-provisioned directory to the reviewed one.
func (r *reviewedRebuild) verify(workdir string) error {
	digest, err := configurationRebuildDigest(workdir, r.planPath, r.transient)
	if err != nil {
		return err
	}
	if digest != r.plan.RebuildDigest {
		return errUtils.Build(fmt.Errorf("%w: the re-provisioned working directory differs from the reviewed one", errUtils.ErrPlanVerificationFailed)).
			WithExplanation("The module source or generated files in this checkout do not reproduce what was reviewed (a different module revision, a floating source reference, or a different checkout path).").
			WithHint("Create a new plan from this revision with `atmos terraform plan`.").
			Err()
	}
	lock, err := lockFileDigest(workdir)
	if err != nil {
		return err
	}
	if lock != r.plan.LockDigest {
		return errUtils.Build(fmt.Errorf("%w: the provider lock file differs from the reviewed one", errUtils.ErrPlanVerificationFailed)).
			WithHint("Restore `.terraform.lock.hcl` together with the plan (`atmos terraform planfile download` does), or create a new plan.").
			Err()
	}
	return nil
}

// downloadedModulesDir is where `init` and `get` install child modules.
var downloadedModulesDir = filepath.Join(".terraform", "modules")

// hasDownloadedModules reports whether a workdir holds downloaded child modules.
func hasDownloadedModules(workdir string) bool {
	info, err := os.Stat(filepath.Join(workdir, downloadedModulesDir))
	return err == nil && info.IsDir()
}

// installReviewedModules downloads the child modules of a reviewed plan into the rebuilt workdir before it
// is verified, as `init` did before the plan was made. The modules stay covered by RebuildDigest, so a
// source that now resolves to different code fails verification instead of being applied; nothing is
// restored from the plan artifacts.
func (e *Executor) installReviewedModules(atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo, rebuild *reviewedRebuild, workdir string) error {
	if rebuild == nil || !rebuild.plan.Modules || info.DryRun {
		return nil
	}
	if err := e.services.ExecuteShellCommand(*atmosConfig, info.Command, []string{"get"}, workdir, info.ComponentEnvList, false, info.RedirectStdErr, component.WithEnvironment(info.SanitizedEnv)); err != nil {
		return errUtils.Build(fmt.Errorf("%w: downloading the reviewed child modules: %w", errUtils.ErrPlanVerificationFailed, err)).
			WithExplanation("The reviewed plan uses downloaded child modules, which must be downloaded again and match before the plan is applied.").
			WithHint("Check that the module sources are reachable from this runner, or apply on the runner that created the plan.").
			Err()
	}
	return nil
}

// rebuildReviewedWorkdir re-provisions the working directory of a reviewed plan on a runner that only
// holds the restored artifacts. The caller verifies the result once execution has written its own
// files (generated backend configuration), before anything runs.
func (e *Executor) rebuildReviewedWorkdir(ctx context.Context, writers provisioner.OutputWriters, atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo, rebuild *reviewedRebuild) (path string, err error) {
	restore, err := rebuild.stage()
	if err != nil {
		return "", err
	}
	defer func() {
		// The restored artifacts return in every case so the user can inspect or retry.
		if restoreErr := restore(); restoreErr != nil {
			err = errors.Join(err, restoreErr)
		}
	}()
	path, err = e.resolveAndProvisionComponentPath(ctx, writers, atmosConfig, info)
	if err != nil {
		return "", err
	}
	if !sameDirectory(path, rebuild.workdir) {
		return "", fmt.Errorf("%w: the working directory resolved to %s instead of the reviewed %s", errUtils.ErrPlanVerificationFailed, path, rebuild.workdir)
	}
	if err = configuration.WriteFiles(path, rebuild.prepared.Files); err != nil {
		return "", err
	}
	// The artifacts go back before verification: a restored lock file replaces any copied from beside the unit.
	if err = restore(); err != nil {
		return "", err
	}
	restore = func() error { return nil }
	return path, nil
}
