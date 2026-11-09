package exec

import (
	"fmt"
	"path/filepath"
	"slices"
	"sync/atomic"

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
