package exec

import (
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"

	"github.com/bmatcuk/doublestar/v4"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

// rebaseConfigurationDiscovery replaces only the inventory session. Plugin code
// and policy still come from the trusted invocation, not the historical checkout.
func rebaseConfigurationDiscovery(config *schema.AtmosConfiguration) error {
	if config.ComponentConfigurations == nil {
		return nil
	}
	revisions, ok := config.ComponentConfigurations.(schema.ComponentConfigurationRevisionRuntime)
	if !ok {
		return fmt.Errorf("%w: configuration provider does not support revision discovery", errUtils.ErrPluginUnsupportedExtension)
	}
	session, err := revisions.ForRevision(config.BasePathAbsolute)
	if err != nil {
		return err
	}
	config.ComponentConfigurations = session
	return nil
}

// configurationWatchIndex applies provider-declared cache exclusions only to its
// file watches. It never changes YAML dependencies or the shared Git change set.
func configurationWatchIndex(config *schema.AtmosConfiguration, settings map[string]any, original *changedFilesIndex) (*changedFilesIndex, error) {
	patterns, err := configurationWatchExcludes(settings)
	if err != nil {
		return nil, err
	}
	if len(patterns) == 0 {
		return original, nil
	}
	key := config.BasePathAbsolute + "\x00" + strings.Join(patterns, "\x00")
	if view := lastConfigurationWatchIndex.Load(); view != nil && view.source == original && view.key == key {
		return view.filtered, nil
	}
	files, err := unexcludedWatchFiles(config.BasePathAbsolute, original.getAllFiles(), patterns)
	if err != nil {
		return nil, err
	}
	filtered := newChangedFilesIndex(config, files, config.BasePathAbsolute)
	lastConfigurationWatchIndex.Store(&configurationWatchIndexView{source: original, key: key, filtered: filtered})
	return filtered, nil
}

// configurationWatchExcludes reads and validates the provider's watch_excludes.
// An absent or empty setting yields no patterns; any other shape is rejected.
func configurationWatchExcludes(settings map[string]any) ([]string, error) {
	provider, _ := settings["configuration"].(map[string]any)
	raw := provider["watch_excludes"]
	if raw == nil {
		return nil, nil
	}
	excludes, ok := raw.([]any)
	if !ok {
		return nil, fmt.Errorf("%w: watch_excludes must be a list of patterns", errUtils.ErrPluginInvalidConfig)
	}
	patterns := make([]string, 0, len(excludes))
	for _, value := range excludes {
		pattern, ok := value.(string)
		if !ok || !doublestar.ValidatePattern(pattern) {
			return nil, errUtils.ErrPluginInvalidConfig
		}
		patterns = append(patterns, pattern)
	}
	return patterns, nil
}

// unexcludedWatchFiles returns the files under root that match none of the patterns.
func unexcludedWatchFiles(root string, all []string, patterns []string) ([]string, error) {
	files := make([]string, 0, len(all))
	for _, path := range all {
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return nil, fmt.Errorf("%w: watch path %q is not under %q: %w", errUtils.ErrPluginInvalidConfig, path, root, err)
		}
		excluded := false
		for _, pattern := range patterns {
			matched, _ := doublestar.Match(pattern, filepath.ToSlash(relative))
			excluded = excluded || matched
		}
		if !excluded {
			files = append(files, path)
		}
	}
	return files, nil
}

// configurationWatchIndexView pairs a change-set index and exclusion set with its filtered copy.
type configurationWatchIndexView struct {
	source, filtered *changedFilesIndex
	key              string
}

// lastConfigurationWatchIndex holds the most recent filtered index, so provider
// components that share exclusions reuse one index (and its resolved view)
// instead of rebuilding it per component. Indexes are immutable after construction.
var lastConfigurationWatchIndex atomic.Pointer[configurationWatchIndexView]

// configurationWatchPaths resolves provider-owned watches against the project
// root without putting checkout-specific absolute paths into stack comparisons.
// Existing YAML dependency path semantics remain unchanged. It reports whether
// the dependencies belong to a provider-discovered component.
func configurationWatchPaths(config *schema.AtmosConfiguration, settings map[string]any, deps []schema.ComponentDependency) bool {
	if _, provider := settings["configuration"]; !provider {
		return false
	}
	for i := range deps {
		if !filepath.IsAbs(deps[i].Path) {
			deps[i].Path = filepath.Join(config.BasePathAbsolute, deps[i].Path)
		}
	}
	return true
}

// configurationMatchInputs returns the change set and dependencies to match.
// Provider watches are rooted at BasePathAbsolute, and base-path resolution
// can spell that root differently from the repository root the changed files
// are joined to: through a symlinked temp dir on macOS (/var vs /private/var)
// or as an 8.3 short name on Windows (RUNNER~1 vs runneradmin). Two spellings
// of one directory never match, so provider watches compare both sides with
// symlinks and short names resolved. YAML dependencies keep exact matching.
func configurationMatchInputs(filesIndex *changedFilesIndex, deps []schema.ComponentDependency, provider bool) (*changedFilesIndex, []schema.ComponentDependency) {
	if !provider {
		return filesIndex, deps
	}
	resolved := slices.Clone(deps)
	for i := range resolved {
		resolved[i].Path = evalSymlinksBestEffort(resolved[i].Path)
	}
	return resolvedChangedFiles(filesIndex), resolved
}

// configuredDependencyPath reports a matched dependency in its configured spelling.
func configuredDependencyPath(deps, matched []schema.ComponentDependency, path string) string {
	for i := range matched {
		if matched[i].Path == path {
			return deps[i].Path
		}
	}
	return path
}

// resolvedChangedFilesView pairs a change-set index with its resolved copy.
type resolvedChangedFilesView struct {
	source, resolved *changedFilesIndex
}

// lastResolvedChangedFiles holds the view for the most recent index, so a run
// resolves its change set once rather than once per provider component. The
// index is immutable after construction, so pointer identity is a safe key.
var lastResolvedChangedFiles atomic.Pointer[resolvedChangedFilesView]

// resolvedChangedFiles returns the change set with symlinks and short names resolved.
func resolvedChangedFiles(index *changedFilesIndex) *changedFilesIndex {
	if view := lastResolvedChangedFiles.Load(); view != nil && view.source == index {
		return view.resolved
	}
	files := index.getAllFiles()
	resolved := make([]string, len(files))
	for i, file := range files {
		resolved[i] = evalSymlinksBestEffort(file)
	}
	view := &resolvedChangedFilesView{source: index, resolved: &changedFilesIndex{allFiles: resolved}}
	lastResolvedChangedFiles.Store(view)
	return view.resolved
}
