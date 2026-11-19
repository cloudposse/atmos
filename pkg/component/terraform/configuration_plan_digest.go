package terraform

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	ciplanfile "github.com/cloudposse/atmos/pkg/ci/plugins/terraform/planfile"
	envpkg "github.com/cloudposse/atmos/pkg/env"
	"github.com/cloudposse/atmos/pkg/schema"
)

// configurationWorkdirDigest binds retained module/configuration bytes, excluding mutable execution data.
func configurationWorkdirDigest(root, planPath, dataDir string) (string, error) {
	return workdirDigest(root, planPath, dataDir, nil)
}

// workdirDigest hashes the retained files below root, additionally skipping the top-level files
// that ignore selects by their slash-separated path relative to root. Besides the default
// `.terraform` directory, the effective Terraform data directory (dataDir; empty means the default)
// is excluded except for its modules subtree.
func workdirDigest(root, planPath, dataDir string, ignore func(relative string) bool) (string, error) {
	// An explicit plan path may be absolute while the directory is relative, or the reverse.
	// Both must compare equal, or the plan file itself would be hashed into the directory digest.
	root, planPath, err := absolutePaths(root, planPath)
	if err != nil {
		return "", err
	}
	scope := newWorkdirScope(root, dataDir)
	files := make(map[string]string)
	err = filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			if scope.skipDirectory(path, entry.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if scope.skipFile(path, planPath, entry.Name(), ignore) {
			return nil
		}
		return recordWorkdirFile(root, path, entry, files)
	})
	if err != nil {
		return "", err
	}
	if err := scope.recordExternalModules(files); err != nil {
		return "", err
	}
	data, err := json.Marshal(files)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:]), nil
}

// workdirScope locates the mutable Terraform data of a working directory: the default `.terraform`
// directories and the effective data directory.
type workdirScope struct {
	root, dataDir string
	placement     dataDirPlacement
}

func (s workdirScope) skipFile(path, planPath, name string, ignore func(string) bool) bool {
	if path == planPath || path == planPath+configurationPlanSuffix || s.insideCacheOutsideModules(path) {
		return true
	}
	return (!s.insideModules(path) && ignoredWorkdirFile(path, planPath, name)) || ignoredRelative(s.root, path, ignore)
}

// insideCacheOutsideModules reports whether path holds mutable Terraform data: below a `.terraform`
// directory or the effective data directory, outside the downloaded modules.
func (s workdirScope) insideCacheOutsideModules(path string) bool {
	return insideTerraformCacheOutsideModules(s.root, path) || s.insideDataDirOutsideModules(path)
}

// insideModules reports whether path is part of downloaded modules.
func (s workdirScope) insideModules(path string) bool {
	return insideTerraformModules(s.root, path) || insideDataDirModules(s.dataDir, path)
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
func (s workdirScope) skipDirectory(path, name string) bool {
	if path == s.root {
		return false
	}
	if s.insideCacheOutsideModules(path) {
		return true
	}
	return !s.insideModules(path) && (name == ".git" || name == ".terragrunt-cache" || name == ".atmos" || name == stateWorkspaceDir || strings.HasPrefix(name, ".atmos-policy-plan-"))
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
		if part == ".terraform" && i+1 < len(parts) && parts[i+1] != modulesDirName {
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
		if part == ".terraform" && i+1 < len(parts) && parts[i+1] == modulesDirName {
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
	relative, err := filepath.Rel(root, path)
	if err != nil {
		return err
	}
	return recordWorkdirFileAs(filepath.ToSlash(relative), path, entry, files)
}

// recordWorkdirFileAs digests one file under the given digest key.
func recordWorkdirFileAs(key, path string, entry fs.DirEntry, files map[string]string) error {
	if entry.Type()&os.ModeSymlink != 0 {
		return fmt.Errorf("%w: saved-plan workdir contains symlink", errUtils.ErrPluginUnsupportedExtension)
	}
	digest, err := fileDigest(path)
	if err != nil {
		return err
	}
	files[key] = digest
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
	// observed is the plan path of the last observed subprocess run. When it ran but kept no
	// snapshot, the binding must not fall back to a later observation that after hooks could change.
	observed string
}

// takePlanSnapshot fingerprints the configuration with the inputs the binding record uses.
func takePlanSnapshot(ctx context.Context, config *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo, execCtx *componentExecContext, path string) (*planSnapshot, error) {
	if err := rejectExtraArgumentsDataDir(info.ComponentExecution); err != nil {
		return nil, err
	}
	planProvider, err := planRuntime(config)
	if err != nil {
		return nil, err
	}
	identity, err := planProvider.PlanIdentity(ctx, info, execCtx.preparedConfiguration.Sources)
	if err != nil {
		return nil, err
	}
	dataDir := effectiveDataDir(config, info)
	digest, err := configurationWorkdirDigest(execCtx.workingDir, path, dataDir)
	if err != nil {
		return nil, err
	}
	rebuild, err := configurationRebuildDigest(execCtx.workingDir, path, dataDir, transientWorkdirFiles(info))
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
	return &planSnapshot{path: path, identity: identity, workingDigest: digest, rebuildDigest: rebuild, lockDigest: lock, modules: hasDownloadedModules(execCtx.workingDir, dataDir), sensitiveDigest: sensitive, engineDigest: engineDigest, publicFiles: publicFiles}, nil
}

// around wraps one run of the Terraform subprocess. The configuration must be the same before and
// after it; the unchanged snapshot is kept for the binding, and a changed one rejects the plan.
// The failed callback classifies the result with the phase's effective arguments and environment, the
// same classification the hooks use, so the "changes present" status of -detailed-exitcode
// (from the command line or TF_CLI_ARGS, including extra_arguments) still binds the plan.
func (s *planSnapshotter) around(phaseArgs []string, run func() error, failed func(error) bool) func() error {
	return func() error {
		s.result, s.err, s.observed = nil, nil, ""
		path := s.pathFrom(phaseArgs)
		if path == "" {
			return run()
		}
		s.observed = path
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
			if failed(runErr) {
				return runErr
			}
			digest, digestErr := fileDigest(writtenPath)
			if digestErr != nil {
				s.err = digestErr
				return digestErr
			}
			after.planDigest = digest
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

// modulesDirName is the directory of downloaded child modules below a Terraform data directory.
const modulesDirName = "modules"

// defaultDataDir is Terraform's data directory when TF_DATA_DIR is not set.
const defaultDataDir = ".terraform"

// tfDataDirEnv is the variable that moves Terraform's data directory.
const tfDataDirEnv = "TF_DATA_DIR"

// effectiveDataDir returns the Terraform data directory the plan and get subprocesses use, as the
// value the environment carries (relative values are resolved against the working directory by
// the callers that need a path). The layers apply in the order the subprocess sees them: the
// execution's base environment, the global Atmos environment, then the component environment.
// The default `.terraform` applies when no layer sets a value.
func effectiveDataDir(config *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo) string {
	entries := append([]string(nil), planBaseEnvironment(info, nil)...)
	if config != nil {
		entries = envpkg.MergeGlobalEnv(entries, config.Env)
	}
	entries = append(entries, info.ComponentEnvList...)
	for i := len(entries) - 1; i >= 0; i-- {
		if value, found := strings.CutPrefix(entries[i], tfDataDirEnv+"="); found {
			if value == "" {
				break
			}
			return value
		}
	}
	return defaultDataDir
}

// resolveDataDir returns the data directory as an absolute path: a relative value is resolved
// against root, as Terraform does against its working directory. An empty value is the default.
func resolveDataDir(root, dataDir string) string {
	if dataDir == "" {
		dataDir = defaultDataDir
	}
	if !filepath.IsAbs(dataDir) {
		return filepath.Join(root, dataDir)
	}
	resolved, err := resolvedPath(dataDir)
	if err != nil {
		return filepath.Clean(dataDir)
	}
	return resolved
}

// insideDataDirModules reports whether path is the modules subtree of a data directory or below it.
func insideDataDirModules(dataDir, path string) bool {
	relative, ok := relativeInside(dataDir, path)
	return ok && (relative == modulesDirName || strings.HasPrefix(relative, modulesDirName+string(filepath.Separator)))
}

// relativeInside returns path relative to dir when it is dir or below it.
func relativeInside(dir, path string) (string, bool) {
	if dir == "" {
		return "", false
	}
	relative, err := filepath.Rel(dir, path)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", false
	}
	return relative, true
}

// dataDirPlacement says where the effective Terraform data directory lies relative to the working directory.
type dataDirPlacement int

const (
	// Below the workdir: a data directory inside the workdir, the default `.terraform` included.
	dataDirBelowWorkdir dataDirPlacement = iota
	// Overlapping the workdir: a data directory that is the workdir or one of its parents. The workdir
	// configuration then shares its location with the data directory.
	dataDirOverlapsWorkdir
	// Outside the workdir: a data directory that neither contains nor lies inside the workdir.
	dataDirOutsideWorkdir
)

// externalModulesKeyPrefix names the downloaded modules of a data directory outside the workdir in the
// digest. A NUL byte cannot occur in a file name, so no workdir file collides with it, and the key does
// not depend on where the data directory is.
const externalModulesKeyPrefix = "\x00data-dir/" + modulesDirName + "/"

// establishedDataDirFiles are the files Terraform writes at the top level of its data directory: the
// workspace marker, the backend state, and the plugin path. The `providers` directory is the only
// established directory besides `modules`.
var establishedDataDirFiles = []string{"environment", "terraform.tfstate", "plugin_path"}

// providersDirName is the directory of installed providers below a Terraform data directory.
const providersDirName = "providers"

func newWorkdirScope(root, dataDir string) workdirScope {
	resolved := resolveDataDir(root, dataDir)
	return workdirScope{root: root, dataDir: resolved, placement: placeDataDir(root, resolved)}
}

// placeDataDir classifies an absolute data directory against an absolute workdir.
func placeDataDir(root, dataDir string) dataDirPlacement {
	if _, contains := relativeInside(dataDir, root); contains {
		return dataDirOverlapsWorkdir
	}
	if _, inside := relativeInside(root, dataDir); inside {
		return dataDirBelowWorkdir
	}
	return dataDirOutsideWorkdir
}

// insideDataDirOutsideModules reports whether path holds mutable data of the data directory, which is
// everything but its modules subtree. When the data directory contains the workdir itself, only the
// established runtime paths count: the rest is the reviewed configuration, which must stay bound.
func (s workdirScope) insideDataDirOutsideModules(path string) bool {
	relative, ok := relativeInside(s.dataDir, path)
	if !ok || relative == "." || inModulesSubtree(relative) {
		return false
	}
	if s.placement == dataDirOverlapsWorkdir {
		return establishedRuntimePath(relative)
	}
	return true
}

// establishedRuntimePath reports whether a path relative to a data directory is data Terraform
// writes there at run time.
func establishedRuntimePath(relative string) bool {
	for _, name := range establishedDataDirFiles {
		if relative == name {
			return true
		}
	}
	return relative == providersDirName || strings.HasPrefix(relative, providersDirName+string(filepath.Separator))
}

func inModulesSubtree(relative string) bool {
	return relative == modulesDirName || strings.HasPrefix(relative, modulesDirName+string(filepath.Separator))
}

// recordExternalModules adds the downloaded modules of a data directory outside the workdir to the
// digest files. They are executed during apply like the ones below the workdir, so a change to them
// must not look retained. An absent modules directory adds nothing.
func (s workdirScope) recordExternalModules(files map[string]string) error {
	if s.placement != dataDirOutsideWorkdir {
		return nil
	}
	modules := filepath.Join(s.dataDir, modulesDirName)
	if _, err := os.Stat(modules); errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	return filepath.WalkDir(modules, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		relative, err := filepath.Rel(modules, path)
		if err != nil {
			return err
		}
		return recordWorkdirFileAs(externalModulesKeyPrefix+filepath.ToSlash(relative), path, entry, files)
	})
}

// rejectExtraArgumentsDataDir fails a reviewed plan whose provider extra arguments set TF_DATA_DIR. They
// apply per command, so init, get, plan and apply could each use a different data directory, and one
// digest of the downloaded modules cannot bind them all. The data directory is set with the
// component's `env`, which every phase shares.
func rejectExtraArgumentsDataDir(execution *schema.ComponentExecution) error {
	for _, candidate := range []*schema.ComponentExecution{execution, applyExecution(execution)} {
		if candidate == nil {
			continue
		}
		for _, extra := range candidate.ExtraArguments {
			for name := range extra.Environment {
				if strings.EqualFold(name, tfDataDirEnv) {
					return errUtils.Build(fmt.Errorf("%w: %s is set by extra_arguments", errUtils.ErrPlanVerificationFailed, tfDataDirEnv)).
						WithExplanation("Extra arguments apply to individual Terraform commands, so the data directory that holds downloaded modules could differ between the plan, the module download, and the apply of a reviewed plan.").
						WithHint("Set `TF_DATA_DIR` in the component `env` instead, which every command shares.").
						Err()
				}
			}
		}
	}
	return nil
}

func applyExecution(execution *schema.ComponentExecution) *schema.ComponentExecution {
	if execution == nil {
		return nil
	}
	return execution.Apply
}
