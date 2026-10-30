package exec

import (
	"fmt"
	"path/filepath"

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
	provider, _ := settings["configuration"].(map[string]any)
	excludes, _ := provider["watch_excludes"].([]any)
	if len(excludes) == 0 {
		return original, nil
	}
	patterns := make([]string, 0, len(excludes))
	for _, value := range excludes {
		pattern, ok := value.(string)
		if !ok || !doublestar.ValidatePattern(pattern) {
			return nil, errUtils.ErrPluginInvalidConfig
		}
		patterns = append(patterns, pattern)
	}
	files := make([]string, 0, len(original.getAllFiles()))
	for _, path := range original.getAllFiles() {
		relative, err := filepath.Rel(config.BasePathAbsolute, path)
		if err != nil {
			return nil, err
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
	return newChangedFilesIndex(config, files, config.BasePathAbsolute), nil
}

// configurationWatchPaths resolves provider-owned watches against the project
// root without putting checkout-specific absolute paths into stack comparisons.
// Existing YAML dependency path semantics remain unchanged.
func configurationWatchPaths(config *schema.AtmosConfiguration, settings map[string]any, deps []schema.ComponentDependency) {
	if _, provider := settings["configuration"]; !provider {
		return
	}
	for i := range deps {
		if !filepath.IsAbs(deps[i].Path) {
			deps[i].Path = filepath.Join(config.BasePathAbsolute, deps[i].Path)
		}
	}
}
