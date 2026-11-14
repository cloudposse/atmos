package exec

import (
	"encoding/json"
	"fmt"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/component/configuration"
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
	patterns, err := configuration.WatchExcludes(settings)
	if err != nil {
		return nil, err
	}
	if len(patterns) == 0 {
		return original, nil
	}
	keyBytes, err := json.Marshal(append([]string{config.BasePathAbsolute}, patterns...))
	if err != nil {
		return nil, err
	}
	key := string(keyBytes)
	original.watchMu.Lock()
	defer original.watchMu.Unlock()
	if filtered := original.watchIndexes[key]; filtered != nil {
		return filtered, nil
	}
	files, err := configuration.UnexcludedWatchFiles(config.BasePathAbsolute, original.getAllFiles(), patterns)
	if err != nil {
		return nil, err
	}
	filtered := newChangedFilesIndex(config, files, config.BasePathAbsolute)
	if original.watchIndexes == nil {
		original.watchIndexes = make(map[string]*changedFilesIndex)
	}
	original.watchIndexes[key] = filtered
	return filtered, nil
}

// configurationMatchInputs returns the change set and dependencies to match.
// The configuration package owns provider dependency normalization; exec only
// supplies the shared resolver and the cached resolved change set.
func configurationMatchInputs(filesIndex *changedFilesIndex, deps []schema.ComponentDependency, provider bool) (*changedFilesIndex, []schema.ComponentDependency) {
	if !provider {
		return filesIndex, deps
	}
	return resolvedChangedFiles(filesIndex), configuration.MatchDependencies(deps, provider, evalSymlinksBestEffort)
}

// resolvedChangedFiles returns the change set with symlinks and short names resolved.
func resolvedChangedFiles(index *changedFilesIndex) *changedFilesIndex {
	index.resolvedOnce.Do(func() {
		files := index.getAllFiles()
		resolved := make([]string, len(files))
		for i, file := range files {
			resolved[i] = evalSymlinksBestEffort(file)
		}
		index.resolved = &changedFilesIndex{allFiles: resolved}
	})
	return index.resolved
}
