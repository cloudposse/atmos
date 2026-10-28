package source

import (
	"archive/zip"
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/cache"
	"github.com/cloudposse/atmos/pkg/provisioner"
	"github.com/cloudposse/atmos/pkg/schema"
)

const sourceFilesManifest = ".atmos/source-files.json"

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
	lock := cache.NewFileLock(provisioner.LockCoordPath(canonical + ".source-refresh"))
	return lock.WithLock(func() error { return stageSourceRefresh(ctx, config, spec, target, opts) })
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
		return err
	}
	defer os.RemoveAll(stage)
	stagedOpts := opts
	stagedOpts.preserveRuntime, stagedOpts.replaceTarget = false, true
	if err := vendorSource(ctx, config, spec, stage, stagedOpts); err != nil {
		return err
	}
	if opts.metadata != nil {
		if err := opts.metadata(stage); err != nil {
			return err
		}
	}
	return publishSource(ctx, stage, target, opts.componentConfig, os.Rename)
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
	changes, incoming, err := sourceChanges(stage, target, next, previous, runtimePaths(target, config))
	if err != nil {
		return err
	}
	changes, err = prepareSourceReceipts(stage, target, next, changes, incoming)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(target, TargetDirPermissions); err != nil {
		return err
	}
	backup, err := os.MkdirTemp(filepath.Dir(target), ".atmos-source-rollback-")
	if err != nil {
		return err
	}
	publication := sourcePublication{stage: stage, target: target, backup: backup, rename: rename}
	return publication.publish(ctx, changes, incoming)
}

func readSourceInventory(target string) (sourceFiles, error) {
	previous := sourceFiles{}
	data, err := os.ReadFile(filepath.Join(target, sourceFilesManifest))
	if os.IsNotExist(err) {
		return previous, nil
	}
	if err != nil {
		return previous, err
	}
	if err := json.Unmarshal(data, &previous); err != nil {
		return previous, fmt.Errorf("read source ownership: %w", err)
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

func safeSourceRelative(name string) bool {
	return name != "" && name != "." && !filepath.IsAbs(name) && filepath.ToSlash(filepath.Clean(name)) == name && name != ".." && !strings.HasPrefix(name, "../")
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
	roots := []string{target}
	if component := extractComponentName(config); component != "" {
		roots = append(roots, filepath.Join(target, component))
	}
	protected := map[string]bool{}
	for _, root := range roots {
		for _, name := range []string{".terraform", "terraform.tfstate", "terraform.tfstate.backup", "terraform.tfstate.d"} {
			protected[filepath.Join(root, name)] = true
		}
		backend, _ := config["backend"].(map[string]any)
		local, _ := backend["local"].(map[string]any)
		for _, key := range []string{"path", "workspace_dir"} {
			if value, ok := local[key].(string); ok && value != "" {
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
	return protected
}

func runtimeOwned(path, relative string, protected map[string]bool) (bool, error) {
	if _, err := os.Lstat(path); os.IsNotExist(err) {
		return false, nil
	} else if err != nil {
		return false, err
	}
	if protectedRuntimePath(path, relative, protected) {
		return true, nil
	}
	state, err := isTerraformState(path)
	if err != nil || state {
		return state, err
	}
	return isTerraformPlan(path), nil
}

func protectedRuntimePath(path, relative string, protected map[string]bool) bool {
	for root := range protected {
		if path == root || strings.HasPrefix(path, root+string(filepath.Separator)) {
			return true
		}
	}
	return strings.HasSuffix(filepath.Base(path), ".terraform.lock.hcl") || relative == ".atmos/metadata.json" || relative == ".atmos/source.json"
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
	if err := rejectSourceSymlinkAncestors(target, name); err != nil {
		return false, err
	}
	path := filepath.Join(target, filepath.FromSlash(name))
	runtime, err := runtimeOwned(path, name, protected)
	if err != nil {
		return false, fmt.Errorf("inspect runtime artifact %s: %w", name, err)
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
