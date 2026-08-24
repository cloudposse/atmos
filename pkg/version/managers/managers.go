// Package managers defines the file-manager registry for the Atmos Version
// Tracker: pluggable scanners/rewriters that keep project files (GitHub
// Actions workflows, marker-annotated files, rendered templates) in sync with
// the locked versions. Managers plan pure in-memory changes; shared drivers
// apply them or fail on drift for CI.
package managers

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
	"github.com/cloudposse/atmos/pkg/version/manager"
)

var (
	// ErrDrift is returned by Check when managed files differ from the lock.
	ErrDrift = errUtils.ErrVersionFilesDrift
	// ErrUnknownManager is returned for a file rule naming an unregistered manager.
	ErrUnknownManager = errUtils.ErrUnknownVersionFileManager
	// ErrDuplicateManager is returned when two managers register the same name.
	ErrDuplicateManager = errUtils.ErrDuplicateVersionFileManager
)

// Permission for files written by Apply when the file does not exist yet.
// Managed files are ordinary project files, not secrets.
const managedFilePerm os.FileMode = 0o644

// Permission for directories created to hold managed files.
const managedDirPerm os.FileMode = 0o755

// RenderFunc renders template content with the given data. The command layer
// injects the Atmos template engine so this package never depends on
// internal/exec.
type RenderFunc = manager.RenderFunc

// Input is everything a file manager needs to plan its changes.
type Input struct {
	// Config is the Atmos configuration.
	Config *schema.AtmosConfiguration
	// Track is the effective version track.
	Track string
	// Entries are the track's effective entries (policy applied).
	Entries map[string]manager.EffectiveEntry
	// Refs are the locked version references by entry name.
	Refs map[string]manager.VersionRef
	// Dir is the root directory globs are resolved from.
	Dir string
	// Paths are the glob patterns to scan (the manager's defaults when empty).
	Paths []string
	// Options carries manager-specific settings from the file rule.
	Options map[string]any
	// Render is the template engine (used by the template manager).
	Render RenderFunc
}

// FileChange is one planned file modification.
type FileChange struct {
	// Path is the file to write (relative to Input.Dir or absolute).
	Path string
	// Old is the current content (nil when the file does not exist).
	Old []byte
	// New is the desired content.
	New []byte
}

// PlannedChange associates a change with the manager that planned it.
type PlannedChange struct {
	Manager string
	FileChange
}

// Manager plans version updates for a class of project files. Plan must be
// pure: it never writes; the shared Apply/Check drivers act on its output.
type Manager interface {
	// Name returns the manager's registry name.
	Name() string
	// DefaultPaths returns the glob patterns scanned when no file rule
	// configures paths (empty means the manager only runs when configured).
	DefaultPaths() []string
	// Plan returns the file changes needed to match the locked versions.
	Plan(ctx context.Context, in *Input) ([]FileChange, error)
}

var (
	registryMu sync.RWMutex
	registry   = map[string]Manager{}
)

// Register adds a file manager. It panics on a duplicate name: registration
// happens in init() and a duplicate is a programming error.
func Register(m Manager) {
	defer perf.Track(nil, "managers.Register")()

	registryMu.Lock()
	defer registryMu.Unlock()
	if _, exists := registry[m.Name()]; exists {
		panic(fmt.Errorf("%w: %s", ErrDuplicateManager, m.Name()))
	}
	registry[m.Name()] = m
}

// Get returns the named file manager.
func Get(name string) (Manager, bool) {
	defer perf.Track(nil, "managers.Get")()

	registryMu.RLock()
	defer registryMu.RUnlock()
	m, ok := registry[name]
	return m, ok
}

// All returns the registered managers sorted by name.
func All() []Manager {
	defer perf.Track(nil, "managers.All")()

	registryMu.RLock()
	defer registryMu.RUnlock()
	names := make([]string, 0, len(registry))
	for name := range registry {
		names = append(names, name)
	}
	sort.Strings(names)
	result := make([]Manager, 0, len(names))
	for _, name := range names {
		result = append(result, registry[name])
	}
	return result
}

// RunOptions configures a plan across the configured file rules.
type RunOptions struct {
	// Config is the Atmos configuration.
	Config *schema.AtmosConfiguration
	// Track selects the version track ("" means the configured default).
	Track string
	// Dir is the root directory ("" means the current directory).
	Dir string
	// Only limits the run to the named managers (empty means all).
	Only []string
	// Render is the template engine for the template manager.
	Render RenderFunc
}

// Plan runs the configured file rules (or every registered manager's default
// paths when version.files is empty) and returns all planned changes.
func Plan(ctx context.Context, opts *RunOptions) ([]PlannedChange, error) {
	defer perf.Track(opts.Config, "managers.Plan")()

	track := manager.EffectiveTrack(opts.Config, opts.Track)
	entries, err := manager.EffectiveEntries(opts.Config, track)
	if err != nil && !errors.Is(err, manager.ErrTrackNotFound) {
		return nil, err
	}
	lock, err := manager.LoadLock(opts.Config)
	if err != nil {
		return nil, err
	}
	refs := manager.VersionRefs(entries, lock.Tracks[track])

	var planned []PlannedChange
	for _, rule := range fileRules(opts.Config) {
		if len(opts.Only) > 0 && !containsString(opts.Only, rule.Manager) {
			continue
		}
		fileManager, ok := Get(rule.Manager)
		if !ok {
			return nil, fmt.Errorf("%w: %s", ErrUnknownManager, rule.Manager)
		}
		input := &Input{
			Config:  opts.Config,
			Track:   track,
			Entries: entries,
			Refs:    refs,
			Dir:     opts.Dir,
			Paths:   rule.Paths,
			Options: rule.Options,
			Render:  opts.Render,
		}
		changes, err := fileManager.Plan(ctx, input)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", rule.Manager, err)
		}
		for i := range changes {
			planned = append(planned, PlannedChange{Manager: rule.Manager, FileChange: changes[i]})
		}
	}
	return planned, nil
}

// fileRules returns the configured version.files rules, or one default rule
// per registered manager that declares default paths.
func fileRules(atmosConfig *schema.AtmosConfiguration) []schema.VersionFileRule {
	if atmosConfig != nil && len(atmosConfig.Version.Files) > 0 {
		return atmosConfig.Version.Files
	}
	var rules []schema.VersionFileRule
	for _, m := range All() {
		if len(m.DefaultPaths()) == 0 {
			continue
		}
		rules = append(rules, schema.VersionFileRule{Manager: m.Name()})
	}
	return rules
}

// Apply writes the planned changes to disk, preserving existing file modes.
func Apply(changes []PlannedChange) error {
	defer perf.Track(nil, "managers.Apply")()

	for i := range changes {
		change := &changes[i]
		perm := managedFilePerm
		if info, err := os.Stat(change.Path); err == nil {
			perm = info.Mode().Perm()
		}
		if err := os.MkdirAll(filepath.Dir(change.Path), managedDirPerm); err != nil {
			return err
		}
		if err := writeFileAtomic(change.Path, change.New, perm); err != nil {
			return err
		}
	}
	return nil
}

func writeFileAtomic(path string, content []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".atmos-version-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(tmpName)
		}
	}()

	if _, err := tmp.Write(content); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmpName, perm); err != nil { // #nosec G703 -- tmpName is returned by os.CreateTemp in the target directory.
		return err
	}
	if err := os.Rename(tmpName, path); err != nil { // #nosec G703 -- path is the caller-selected managed file destination.
		return err
	}
	cleanup = false
	return nil
}

// Check fails with ErrDrift when any planned change would modify a file,
// listing the stale paths for CI output.
func Check(changes []PlannedChange) error {
	defer perf.Track(nil, "managers.Check")()

	if len(changes) == 0 {
		return nil
	}
	paths := make([]string, 0, len(changes))
	for i := range changes {
		paths = append(paths, fmt.Sprintf("%s (%s)", changes[i].Path, changes[i].Manager))
	}
	return fmt.Errorf("%w: %s", ErrDrift, strings.Join(paths, ", "))
}

// ExpandPaths resolves glob patterns relative to dir into matching file paths.
func ExpandPaths(dir string, patterns []string) ([]string, error) {
	defer perf.Track(nil, "managers.ExpandPaths")()

	seen := map[string]bool{}
	var files []string
	for _, pattern := range patterns {
		updated, err := expandPattern(dir, pattern, seen)
		if err != nil {
			return nil, err
		}
		files = append(files, updated...)
	}
	sort.Strings(files)
	return files, nil
}

func expandPattern(dir, pattern string, seen map[string]bool) ([]string, error) {
	if dir != "" && dir != "." && !filepath.IsAbs(pattern) {
		pattern = filepath.Join(dir, pattern)
	}
	matches, err := u.GetGlobMatches(filepath.ToSlash(pattern))
	if err != nil {
		// A pattern with no matches is not an error for file managers.
		if errors.Is(err, errUtils.ErrFailedToFindImport) {
			return nil, nil
		}
		return nil, err
	}
	var files []string
	for _, match := range matches {
		info, err := os.Stat(match)
		if err != nil || info.IsDir() || seen[match] {
			continue
		}
		seen[match] = true
		files = append(files, match)
	}
	return files, nil
}

// containsString reports whether values contains value.
func containsString(values []string, value string) bool {
	for _, v := range values {
		if v == value {
			return true
		}
	}
	return false
}
