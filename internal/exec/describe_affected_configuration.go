package exec

import (
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
