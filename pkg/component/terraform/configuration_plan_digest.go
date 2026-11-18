package terraform

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	ciplanfile "github.com/cloudposse/atmos/pkg/ci/plugins/terraform/planfile"
	"github.com/cloudposse/atmos/pkg/schema"
)

// configurationWorkdirDigest binds retained module/configuration bytes, excluding mutable execution data.
func configurationWorkdirDigest(root, planPath string) (string, error) {
	return workdirDigest(root, planPath, nil)
}

// workdirDigest hashes the retained files below root, additionally skipping the top-level files
// that ignore selects by their slash-separated path relative to root.
func workdirDigest(root, planPath string, ignore func(relative string) bool) (string, error) {
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
		if skipWorkdirFile(root, path, planPath, entry.Name(), ignore) {
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

func skipWorkdirFile(root, path, planPath, name string, ignore func(string) bool) bool {
	if path == planPath || path == planPath+configurationPlanSuffix || insideTerraformCacheOutsideModules(root, path) {
		return true
	}
	return (!insideTerraformModules(root, path) && ignoredWorkdirFile(path, planPath, name)) || ignoredRelative(root, path, ignore)
}

// ignoredRelative applies a caller's filter to the slash-separated path relative to root.
func ignoredRelative(root, path string, ignore func(relative string) bool) bool {
	if ignore == nil {
		return false
	}
	relative, err := filepath.Rel(root, path)
	return err == nil && ignore(filepath.ToSlash(relative))
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
	return !insideTerraformModules(root, path) && (name == ".git" || name == ".terragrunt-cache" || name == ".atmos" || name == stateWorkspaceDir || strings.HasPrefix(name, ".atmos-policy-plan-"))
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

// ignoredWorkdirFile reports whether a file is a plan, its binding, or mutable state. Plan files are
// reviewed artifacts, not configuration: besides the plan itself that includes an earlier plan
// still lying in the directory, the stored plan `deploy` downloads for comparison, and the fresh
// plan it generates.
func ignoredWorkdirFile(path, planPath, name string) bool {
	return path == planPath || strings.HasSuffix(name, planBindingSuffix) || strings.HasSuffix(name, ".planfile") ||
		name == ciplanfile.StoredPlanPrefix+ciplanfile.PlanFilename || name == freshPlanName ||
		strings.HasPrefix(name, ".atmos-plan-") || mutableStateFile(name)
}

// stateWorkspaceDir is the directory Terraform keeps the state of its non-default workspaces in.
const stateWorkspaceDir = "terraform.tfstate.d"

// mutableStateFile reports whether a file name is state Terraform writes: the state file, its backup, or a
// timestamped backup. A prefix match would also drop configuration such as terraform.tfstate.tf or
// terraform.tfstate.tofu.json, which a saved plan must stay bound to.
func mutableStateFile(name string) bool {
	if name == "terraform.tfstate" || name == "terraform.tfstate.backup" {
		return true
	}
	stamp, found := strings.CutPrefix(name, "terraform.tfstate.")
	if !found {
		return false
	}
	stamp, found = strings.CutSuffix(stamp, ".backup")
	if !found || stamp == "" {
		return false
	}
	for _, r := range stamp {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
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

// planSnapshot is the configuration a saved plan is bound to: the provider identity and the
// digests of the retained working directory, excluding the plan and the state Terraform writes.
// The rebuild and lock digests let a plan restored on another runner be rebuilt and checked
// against the bytes Terraform read, not against whatever an after hook left behind.
type planSnapshot struct {
	path, identity, workingDigest, rebuildDigest, lockDigest string
	modules                                                  bool
	// sensitiveDigest binds the bytes of every sensitive generated file, including files the working
	// directory digest excludes, such as those below a Terragrunt cache.
	sensitiveDigest string
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
	rebuild, err := configurationRebuildDigest(execCtx.workingDir, path, transientWorkdirFiles(info))
	if err != nil {
		return nil, err
	}
	lock, err := lockFileDigest(execCtx.workingDir)
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
	return &planSnapshot{path: path, identity: identity, workingDigest: digest, rebuildDigest: rebuild, lockDigest: lock, modules: hasDownloadedModules(execCtx.workingDir), sensitiveDigest: sensitive, engineDigest: engineDigest, publicFiles: publicFiles}, nil
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
			// Fingerprint the actual engine output before after hooks, including private policy plans.
			writtenPath := path
			if s.execCtx.planPolicy != nil {
				writtenPath = plannedOutputPath(s.execCtx.componentPath, phaseArgs)
			}
			if digest, digestErr := fileDigest(writtenPath); digestErr == nil {
				after.planDigest = digest
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

// restorePublicFiles restores observed non-sensitive bytes into a plan's copied file list.
func (s *planSnapshot) restorePublicFiles(files []schema.ComponentConfigurationFile) error {
	if s.publicFiles == "" {
		return nil
	}
	var observed map[string][]byte
	if err := json.Unmarshal([]byte(s.publicFiles), &observed); err != nil {
		return err
	}
	for i := range files {
		if content, ok := observed[files[i].Path]; ok && !files[i].Sensitive {
			files[i].Content = content
			if content == nil {
				files[i].IfExists = "remove"
			}
		}
	}
	return nil
}
