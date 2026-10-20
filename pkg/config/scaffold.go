package config

import (
	"fmt"
	"path/filepath"

	"github.com/spf13/viper"
	"go.yaml.in/yaml/v3"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/merge"
	"github.com/cloudposse/atmos/pkg/schema"
)

// resolveScaffoldConfig uses this load's retained sources, including imports and
// profiles. Viper loses literal template names and the origin of relative paths.
func resolveScaffoldConfig(v *viper.Viper, config *schema.AtmosConfiguration) error {
	resolved := &schema.ScaffoldConfiguration{Templates: make(map[string]any), SourceDirs: make(map[string]string)}
	for _, source := range collectConfigSourcesForCasePreservation(v, v.ConfigFileUsed()) {
		var document struct {
			Scaffold *schema.ScaffoldConfiguration `yaml:"scaffold"`
		}
		if err := yaml.Unmarshal([]byte(source.content), &document); err != nil {
			return fmt.Errorf("%w: scaffold configuration in %s: %w", errUtils.ErrInvalidScaffoldConfig, source.path, err)
		}
		if document.Scaffold == nil {
			continue
		}
		for name, raw := range document.Scaffold.Templates {
			if err := mergeScaffoldTemplate(resolved, name, raw, source.path); err != nil {
				return err
			}
		}
	}
	config.Scaffold = resolved
	if len(resolved.Templates) == 0 {
		config.Scaffold = nil
	}
	config.ScaffoldConfigLoaded = true
	return nil
}

func mergeScaffoldTemplate(resolved *schema.ScaffoldConfiguration, name string, raw any, path string) error {
	entry, ok := raw.(map[string]any)
	if !ok {
		resolved.Templates[name] = raw
		delete(resolved.SourceDirs, name)
		return nil
	}
	if _, declaresSource := entry["source"]; declaresSource {
		dir, err := filepath.Abs(filepath.Dir(path))
		if err != nil {
			return fmt.Errorf("%w: %w", errUtils.ErrInvalidScaffoldConfig, err)
		}
		resolved.SourceDirs[name] = dir
	}
	if previous, ok := resolved.Templates[name].(map[string]any); ok {
		merged, err := merge.MergeWithOptions(nil, []map[string]any{previous, entry}, false, false)
		if err != nil {
			return err
		}
		raw = merged
	}
	resolved.Templates[name] = raw
	return nil
}
