package configuration

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/bmatcuk/doublestar/v4"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// RegisterImport routes configured provider schemes before YAML or
// remote-source resolution. Providers add instances to the shared inventory;
// importing a root does not rename its stacks or merge HCL into YAML variables.
func RegisterImport(config *schema.AtmosConfiguration, uri string, declaration *schema.StackImport) (bool, error) {
	defer perf.Track(nil, "configuration.RegisterImport")()

	scheme, _, found := strings.Cut(uri, "://")
	if !found || config == nil {
		return false, nil
	}
	settings, configured := config.Components.Terraform.ConfigurationProviders[scheme]
	if !configured {
		return false, nil
	}
	if !settings.Enabled {
		return true, fmt.Errorf("%w: configuration provider %s is disabled", errUtils.ErrPluginInvalidConfig, scheme)
	}
	if len(declaration.Context) != 0 || declaration.SkipIfMissing || declaration.TTL != "" || declaration.NestedImports != "" {
		return true, fmt.Errorf("%w: provider imports do not support context, skip_if_missing, TTL, or nested_imports", errUtils.ErrPluginUnsupportedExtension)
	}
	imports, ok := config.ComponentConfigurations.(schema.ComponentConfigurationImportRuntime)
	if !ok {
		return true, fmt.Errorf("%w: provider %s has no import runtime", errUtils.ErrPluginUnsupportedExtension, scheme)
	}
	return imports.RegisterImport(context.Background(), uri)
}

// WatchExcludes reads and validates the provider's watch_excludes.
// An absent or empty setting yields no patterns; any other shape is rejected.
func WatchExcludes(settings map[string]any) ([]string, error) {
	defer perf.Track(nil, "configuration.WatchExcludes")()

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

// UnexcludedWatchFiles returns the files under root that match none of the patterns.
func UnexcludedWatchFiles(root string, all []string, patterns []string) ([]string, error) {
	defer perf.Track(nil, "configuration.UnexcludedWatchFiles")()

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
