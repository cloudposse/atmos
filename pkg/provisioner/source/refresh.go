package source

import (
	"archive/zip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/filelock"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/provisioner"
	"github.com/cloudposse/atmos/pkg/provisioner/workdir"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui"
)

const sourceFilesManifest = ".atmos/source-files.json"

// sourceRefreshLockWarnAfter is how long a refresh waits for another process's publication before
// it tells the user why it is blocked. The wait itself is bounded only by the caller's context.
var sourceRefreshLockWarnAfter = 2 * time.Second

// warnSourceRefreshLockWait reports a refresh blocked by another publisher. It is a seam for tests.
var warnSourceRefreshLockWait = func(ctx context.Context, target string) {
	if workdir.OutputSuppressed(ctx) {
		log.Debug("Waiting for another Atmos process to finish refreshing the component source", "target", target)
		return
	}
	ui.Warningf("Waiting for another Atmos process to finish refreshing the component source in %s", target)
}

// sourceFiles records only downloaded files, never files created by Terraform or hooks.
// An absent manifest is a legacy checkout: unrecognized files remain caller-owned.
type sourceFiles struct {
	Files []string `json:"files"`
}

func vendorSourcePreserving(ctx context.Context, config *schema.AtmosConfiguration, spec *schema.VendorComponentSource, target string, opts vendorSourceOptions) error {
	if err := validateVendorTargetDir(target); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(target), TargetDirPermissions); err != nil {
		return err
	}
	// The lock lives outside the mutable source and coordinates publishers for this exact checkout.
	absolute, err := filepath.Abs(target)
	if err != nil {
		return err
	}
	canonical, err := resolveExistingSymlinks(absolute)
	if err != nil {
		return err
	}
	// Publication protects caller state and ownership, so it requires a real lock
	// on every platform; the best-effort cache lock is intentionally a no-op on Windows.
	// Concurrent publishers of one checkout run one after another: a waiter later finds the target
	// up to date or republishes the same files. The wait is bounded by the caller's context.
	lock := filelock.New(provisioner.LockCoordPath(canonical+".source-refresh") + ".lock")
	warning := time.AfterFunc(sourceRefreshLockWarnAfter, func() { warnSourceRefreshLockWait(ctx, target) })
	defer warning.Stop()
	err = lock.WithExclusive(ctx, func() error {
		warning.Stop()
		return stageSourceRefresh(ctx, config, spec, target, opts)
	})
	if errors.Is(err, filelock.ErrAcquire) {
		return errors.Join(errUtils.ErrCacheLocked, err)
	}
	return err
}

func stageSourceRefresh(ctx context.Context, config *schema.AtmosConfiguration, spec *schema.VendorComponentSource, target string, opts vendorSourceOptions) error {
	if info, err := os.Lstat(target); err == nil && info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("%w: source target is a symlink: %s", errUtils.ErrSourceCopyFailed, target)
	}
	if !opts.replaceTarget {
		if _, err := os.Stat(target); err == nil {
			return fmt.Errorf("%w: target directory already exists", errUtils.ErrSourceCopyFailed)
		}
	}
	stage, err := os.MkdirTemp(filepath.Dir(target), ".atmos-source-stage-")
	if err != nil {
		return fmt.Errorf("%w: create source stage: %w", errUtils.ErrSourceCopyFailed, err)
	}
	defer os.RemoveAll(stage)
	stagedOpts := opts
	stagedOpts.preserveRuntime, stagedOpts.replaceTarget = false, true
	if err := vendorSource(ctx, config, spec, stage, stagedOpts); err != nil {
		return err
	}
	if err := rejectGeneratedBackendCollision(config, stage, target, opts.componentConfig); err != nil {
		return err
	}
	if opts.metadata != nil {
		if err := opts.metadata(stage); err != nil {
			return err
		}
	}
	return publishSource(ctx, stage, target, opts.componentConfig, os.Rename)
}

// generatedBackendFile is the backend configuration Atmos writes into the module root.
const generatedBackendFile = "backend.tf.json"

// rejectGeneratedBackendCollision fails a refresh whose source ships the backend file Atmos generates.
// Installing it would replace the generated backend, and the next generation would overwrite the
// source's copy, so neither owner's version could be trusted.
func rejectGeneratedBackendCollision(config *schema.AtmosConfiguration, stage, target string, componentConfig map[string]any) error {
	if config == nil || !config.Components.Terraform.AutoGenerateBackendFile {
		return nil
	}
	for _, root := range moduleRoots(target, componentConfig) {
		rel, err := filepath.Rel(target, root)
		if err != nil {
			continue
		}
		if _, err := os.Lstat(filepath.Join(stage, rel, generatedBackendFile)); err == nil {
			return fmt.Errorf("%w: source ships %s, which Atmos generates because components.terraform.auto_generate_backend_file is enabled; exclude it with source.excluded_paths or disable backend generation",
				errUtils.ErrSourceCopyFailed, filepath.ToSlash(filepath.Join(rel, generatedBackendFile)))
		}
	}
	return nil
}

type sourceRename func(string, string) error

// publishSource updates source files in place. The component directory and runtime files
// never move. Every touched source file is journaled until publication succeeds.
func publishSource(ctx context.Context, stage, target string, config map[string]any, rename sourceRename) error {
	next, err := sourceInventory(stage)
	if err != nil {
		return err
	}
	previous, err := readSourceInventory(target)
	if err != nil {
		return err
	}
	if err := rejectActiveStateLock(target); err != nil {
		return err
	}
	protected := runtimePaths(target, config)
	if err := rejectLegacyResources(target, next, protected, hostGeneratedFiles(target, moduleRoots(target, config))); err != nil {
		return err
	}
	changes, incoming, err := sourceChanges(stage, target, next, previous, protected)
	if err != nil {
		return err
	}
	changes, err = prepareSourceReceipts(stage, target, next, changes, incoming)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(target, TargetDirPermissions); err != nil {
		return fmt.Errorf("%w: create source target: %w", errUtils.ErrSourceCopyFailed, err)
	}
	backup, err := os.MkdirTemp(filepath.Dir(target), ".atmos-source-rollback-")
	if err != nil {
		return fmt.Errorf("%w: create source rollback directory: %w", errUtils.ErrSourceCopyFailed, err)
	}
	publication := sourcePublication{stage: stage, target: target, backup: backup, rename: rename}
	return publication.publish(ctx, changes, incoming)
}

// Without a receipt, neither metadata nor a source stamp proves ownership of a
// removed file. Refuse to bless stale configuration; let the caller reconcile it.
// Every Terraform configuration file absent from the incoming source counts, except
// runtime files and the files Atmos itself writes into the workdir on every run.
func rejectLegacyResources(target string, next sourceFiles, protected, generated map[string]bool) error {
	if _, err := os.Lstat(filepath.Join(target, sourceFilesManifest)); !os.IsNotExist(err) {
		return err
	}
	incoming := make(map[string]bool, len(next.Files))
	for _, name := range next.Files {
		incoming[name] = true
	}
	err := filepath.WalkDir(target, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(target, path)
		if err != nil {
			return err
		}
		relative = filepath.ToSlash(relative)
		if protectedRuntimePath(path, relative, protected) {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if !staleLegacyFile(entry, relative, incoming, generated) {
			return nil
		}
		return fmt.Errorf("%w: legacy workdir %s has no source ownership receipt and contains %s absent from the incoming source; inspect this file and move it outside the workdir or remove it if obsolete, then retry", errUtils.ErrSourceCopyFailed, target, relative)
	})
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

// hostWorkdirFiles are Terraform files Atmos generates in a component workdir on every run.
var hostWorkdirFiles = map[string]bool{"backend.tf.json": true, "providers_override.tf.json": true}

// generatedFilesManifest lists, per module root, the files a configuration provider generated.
const generatedFilesManifest = ".atmos/generated-files.json"

// staleLegacyFile reports a Terraform configuration file that the incoming source no longer has
// and that Atmos did not write into the workdir itself.
func staleLegacyFile(entry fs.DirEntry, relative string, incoming, generated map[string]bool) bool {
	if entry.IsDir() || !terraformConfigurationFile(entry.Name()) {
		return false
	}
	return !incoming[relative] && !generated[relative] && !hostWorkdirFiles[entry.Name()]
}

func terraformConfigurationFile(name string) bool {
	return strings.HasSuffix(name, ".tf") || strings.HasSuffix(name, ".tf.json")
}

// hostGeneratedFiles returns the target-relative paths of files recorded as generated in each
// module root. An unreadable manifest records nothing, so its files are inspected like any other.
func hostGeneratedFiles(target string, roots []string) map[string]bool {
	generated := map[string]bool{}
	for _, root := range roots {
		data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(generatedFilesManifest)))
		if err != nil {
			continue
		}
		var recorded struct {
			Files map[string]string `json:"files"`
		}
		if json.Unmarshal(data, &recorded) != nil {
			continue
		}
		for name := range recorded.Files {
			relative, err := filepath.Rel(target, filepath.Join(root, filepath.FromSlash(name)))
			if err == nil && filepath.IsLocal(relative) {
				generated[filepath.ToSlash(relative)] = true
			}
		}
	}
	return generated
}

func readSourceInventory(target string) (sourceFiles, error) {
	previous := sourceFiles{}
	data, err := os.ReadFile(filepath.Join(target, sourceFilesManifest))
	if os.IsNotExist(err) {
		return previous, nil
	}
	if err != nil {
		return previous, fmt.Errorf("%w: read source ownership: %w", errUtils.ErrSourceCopyFailed, err)
	}
	if err := json.Unmarshal(data, &previous); err != nil {
		return previous, fmt.Errorf("%w: read source ownership: %w", errUtils.ErrSourceCopyFailed, err)
	}
	for _, name := range previous.Files {
		if !safeSourceRelative(name) {
			return previous, fmt.Errorf("%w: invalid source ownership path %q", errUtils.ErrSourceCopyFailed, name)
		}
	}
	return previous, nil
}

func sourceChanges(stage, target string, next, previous sourceFiles, protected map[string]bool) ([]string, map[string]bool, error) {
	owned := make(map[string]bool, len(previous.Files))
	incoming := make(map[string]bool, len(previous.Files)+len(next.Files))
	for _, name := range previous.Files {
		owned[name], incoming[name] = true, false
	}
	for _, name := range next.Files {
		incoming[name] = true
	}
	var changes []string
	for name, install := range incoming {
		change, err := sourceChange(stage, target, name, sourceChangeFlags{install: install, owned: owned[name]}, protected)
		if err != nil {
			return nil, nil, err
		}
		if change {
			changes = append(changes, name)
		}
	}
	sort.Slice(changes, func(i, j int) bool {
		if incoming[changes[i]] != incoming[changes[j]] {
			return !incoming[changes[i]]
		}
		return changes[i] < changes[j]
	})
	return changes, incoming, nil
}

// Legacy directories lack ownership receipts. Terraform configuration is source;
// other existing files must match the incoming bytes before adopting them.
func validateUnownedIncoming(stage, target, name string) error {
	path := filepath.Join(target, name)
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("%w: source path conflicts with existing non-file %s", errUtils.ErrSourceCopyFailed, name)
	}
	if strings.HasSuffix(name, ".tf") || strings.HasSuffix(name, ".tf.json") {
		return nil
	}
	old, oldErr := os.ReadFile(path)
	fresh, freshErr := os.ReadFile(filepath.Join(stage, name))
	if oldErr != nil || freshErr != nil || string(old) != string(fresh) {
		return fmt.Errorf("%w: cannot establish ownership of existing file %s", errUtils.ErrSourceCopyFailed, name)
	}
	return nil
}

func prepareSourceReceipts(stage, target string, next sourceFiles, changes []string, incoming map[string]bool) ([]string, error) {
	encoded, err := json.Marshal(next)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Join(stage, ".atmos"), TargetDirPermissions); err != nil {
		return nil, err
	}
	// #nosec G306 -- ownership receipt contains source-relative filenames, not credentials.
	if err := os.WriteFile(filepath.Join(stage, sourceFilesManifest), encoded, provenanceFilePermissions); err != nil {
		return nil, err
	}
	// Include receipts in the source journal, never metadata-lock sidecars.
	for _, name := range []string{sourceFilesManifest, ".atmos/metadata.json", ".atmos/source.json"} {
		if _, err := os.Stat(filepath.Join(stage, name)); os.IsNotExist(err) {
			continue
		} else if err != nil {
			return nil, err
		}
		if err := rejectSourceSymlinkAncestors(target, name); err != nil {
			return nil, err
		}
		incoming[name] = true
		changes = append(changes, name)
	}
	return changes, nil
}

func sourceInventory(root string) (sourceFiles, error) {
	inventory := sourceFiles{}
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if rel == ".atmos/metadata.json" || rel == ".atmos/metadata.json.lock" || rel == ".atmos/source.json" {
			return nil
		}
		if rel == sourceFilesManifest {
			return fmt.Errorf("%w: source contains reserved ownership manifest %s", errUtils.ErrSourceCopyFailed, rel)
		}
		inventory.Files = append(inventory.Files, rel)
		return nil
	})
	return inventory, err
}

// The safeSourceRelative check accepts only canonical, slash-separated paths that stay inside the target.
// Requiring filepath.IsLocal also rejects volume-qualified names such as "C:state" and reserved device names
// on Windows, where joining them to the target would address an alternate data stream or device.
func safeSourceRelative(name string) bool {
	return name != "" && name != "." && !filepath.IsAbs(name) && filepath.IsLocal(name) && filepath.ToSlash(filepath.Clean(name)) == name && name != ".." && !strings.HasPrefix(name, "../")
}

func rejectSourceSymlinkAncestors(target, relative string) error {
	for path := filepath.Join(target, filepath.Dir(relative)); ; path = filepath.Dir(path) {
		if info, err := os.Lstat(path); err == nil && info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("%w: source destination traverses symlink %s", errUtils.ErrSourceCopyFailed, path)
		}
		if path == filepath.Clean(target) {
			return nil
		}
		if filepath.Dir(path) == path {
			return fmt.Errorf("%w: source destination escaped target", errUtils.ErrSourceCopyFailed)
		}
	}
}

func rejectActiveStateLock(target string) error {
	err := filepath.WalkDir(target, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if strings.HasSuffix(entry.Name(), ".lock.info") {
			return fmt.Errorf("%w: active Terraform state lock %s", errUtils.ErrSourceCopyFailed, path)
		}
		return nil
	})
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

// runtimePaths accounts for nested module roots and custom local backend locations.
func runtimePaths(target string, config map[string]any) map[string]bool {
	protected := map[string]bool{}
	for _, root := range moduleRoots(target, config) {
		for _, name := range []string{".terraform", "terraform.tfstate", "terraform.tfstate.backup", "terraform.tfstate.d"} {
			protected[filepath.Join(root, name)] = true
		}
		protectLocalBackendPaths(protected, root, config)
	}
	return protected
}

// moduleRoots lists every directory Terraform may run in under target: the target itself, the
// component name and the metadata.component subpath joined onto it, and an already-resolved
// workdir path. The subpath candidates are included before they exist, because refresh runs
// before the module root is resolved against the downloaded tree.
func moduleRoots(target string, config map[string]any) []string {
	roots := []string{target}
	candidates := []string{extractComponentName(config)}
	if metadata, ok := config["metadata"].(map[string]any); ok {
		subpath, _ := metadata["component"].(string)
		candidates = append(candidates, subpath)
		moduleSubdir, _ := metadata["source_subdir"].(string)
		candidates = append(candidates, moduleSubdir)
	}
	for _, candidate := range candidates {
		if candidate != "" && filepath.IsLocal(candidate) {
			roots = append(roots, filepath.Join(target, candidate))
		}
	}
	if resolved, ok := config[workdir.WorkdirPathKey].(string); ok && resolved != "" && isWithinBase(resolved, target) {
		roots = append(roots, filepath.Clean(resolved))
	}
	return roots
}

// protectLocalBackendPaths marks custom local backend locations as runtime-owned. The state reader
// accepts both the flat form (backend.path) and the nested form (backend.local.path), so both are
// protected here.
func protectLocalBackendPaths(protected map[string]bool, root string, config map[string]any) {
	backend, _ := config["backend"].(map[string]any)
	local, _ := backend["local"].(map[string]any)
	for _, section := range []map[string]any{backend, local} {
		for _, key := range []string{"path", "workspace_dir"} {
			value, ok := section[key].(string)
			if !ok || value == "" {
				continue
			}
			path := value
			if !filepath.IsAbs(path) {
				path = filepath.Join(root, path)
			}
			protected[filepath.Clean(path)] = true
			if key == "path" {
				protected[filepath.Clean(path)+".backup"] = true
			}
		}
	}
}

func runtimeOwned(path, relative string, protected map[string]bool) (bool, error) {
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		// An absent destination under a protected runtime root (local state, .terraform, a configured
		// backend path) is still runtime-owned: installing a source file there would plant initial state.
		// An absent provider lock file is not under such a root, so it still reaches installation.
		return underProtectedRoot(path, protected), nil
	} else if err != nil {
		return false, err
	}
	if protectedRuntimePath(path, relative, protected) {
		return true, nil
	}
	state, err := isTerraformState(path)
	if info.Mode()&os.ModeSymlink != 0 && os.IsNotExist(err) {
		return false, nil
	}
	if err != nil || state {
		return state, err
	}
	return isTerraformPlan(path), nil
}

func protectedRuntimePath(path, relative string, protected map[string]bool) bool {
	if underProtectedRoot(path, protected) {
		return true
	}
	return strings.HasSuffix(filepath.Base(path), ".terraform.lock.hcl") || relative == ".atmos/metadata.json" || relative == ".atmos/source.json"
}

// underProtectedRoot reports whether path is a protected runtime root or lies beneath one.
func underProtectedRoot(path string, protected map[string]bool) bool {
	for root := range protected {
		if path == root || strings.HasPrefix(path, root+string(filepath.Separator)) {
			return true
		}
	}
	return false
}

// checkSourceDestination rejects a destination that traverses a symlink and, for an incoming file,
// one whose type differs from the source path.
func checkSourceDestination(target, name string, install bool) error {
	if err := rejectSourceSymlinkAncestors(target, name); err != nil {
		return err
	}
	if !install {
		return nil
	}
	return rejectSourceTypeChange(target, name)
}

// rejectSourceTypeChange fails before publication when an incoming file would replace a nonempty
// directory, or would need a directory where a file exists. Refresh replaces files in place and cannot
// change a path's type transactionally, so the error names the path and how to recover. An empty
// directory is still replaced by publication, as before.
func rejectSourceTypeChange(target, name string) error {
	const recovery = "; refresh cannot change a path between a file and a directory in place. Move only that path out of the component directory (keep anything it holds that you still need), then refresh again; do not delete the whole directory, which would also remove local state, `.terraform` and saved plans"
	local := filepath.FromSlash(name)
	if entries, err := os.ReadDir(filepath.Join(target, local)); err == nil && len(entries) > 0 {
		return fmt.Errorf("%w: source file %s conflicts with a nonempty directory in the target%s",
			errUtils.ErrSourceCopyFailed, name, recovery)
	}
	for dir := filepath.Dir(local); dir != "." && dir != string(filepath.Separator); dir = filepath.Dir(dir) {
		if info, err := os.Lstat(filepath.Join(target, dir)); err == nil && info.Mode().IsRegular() {
			return fmt.Errorf("%w: source path %s needs a directory where the target has the file %s%s",
				errUtils.ErrSourceCopyFailed, name, filepath.ToSlash(dir), recovery)
		}
	}
	return nil
}

func isTerraformPlan(path string) bool {
	archive, err := zip.OpenReader(path)
	if err != nil {
		return false
	}
	defer archive.Close()
	for _, file := range archive.File {
		if file.Name == "tfplan" {
			return true
		}
	}
	return false
}

// sourceChange distinguishes files owned by the source from runtime artifacts.
type sourceChangeFlags struct{ install, owned bool }

func sourceChange(stage, target, name string, flags sourceChangeFlags, protected map[string]bool) (bool, error) {
	install, owned := flags.install, flags.owned
	if err := checkSourceDestination(target, name, install); err != nil {
		return false, err
	}
	path := filepath.Join(target, filepath.FromSlash(name))
	runtime, err := runtimeOwned(path, name, protected)
	if err != nil {
		return false, fmt.Errorf("%w: inspect runtime artifact %s: %w", errUtils.ErrSourceCopyFailed, name, err)
	}
	if runtime {
		if install && !strings.HasSuffix(filepath.Base(name), ".terraform.lock.hcl") {
			return false, fmt.Errorf("%w: downloaded source collides with runtime artifact %s", errUtils.ErrSourceCopyFailed, name)
		}
		return false, nil
	}
	if !install {
		_, err := os.Lstat(path)
		return !os.IsNotExist(err), nil
	}
	if !owned {
		if err := validateUnownedIncoming(stage, target, name); err != nil {
			return false, err
		}
	}
	return true, nil
}
