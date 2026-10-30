package config

import (
	"fmt"
	"maps"
	"path/filepath"
	"slices"

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
		templates, err := decodeScaffoldTemplates(source)
		if err != nil {
			return fmt.Errorf("%w: scaffold configuration in %s: %w", errUtils.ErrInvalidScaffoldConfig, source.path, err)
		}
		for name, raw := range templates {
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

// decodeScaffoldTemplates evaluates only the scaffold section, retaining literal
// template names and resolving includes relative to the declaring configuration.
func decodeScaffoldTemplates(source mergedConfigSource) (map[string]any, error) {
	var root map[string]yaml.Node
	if err := yaml.Unmarshal([]byte(source.content), &root); err != nil {
		return nil, err
	}
	node, exists := root["scaffold"]
	if !exists {
		return nil, nil
	}
	// Validate aliases before expanding them, preserving yaml.v3's protections
	// against recursive or excessively expanded aliases without evaluating tags.
	var plain any
	if err := node.Decode(&plain); err != nil {
		return nil, err
	}
	expanded, err := expandScaffoldYAMLNode(&node)
	if err != nil {
		return nil, err
	}
	decoded, err := decodeNodeWithYamlFunctionsForFile(expanded, source.path)
	if err != nil || decoded == nil {
		return nil, err
	}
	scaffold, ok := decoded.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("%w: scaffold must be a mapping", errUtils.ErrInvalidScaffoldConfig)
	}
	raw := scaffold["templates"]
	if raw == nil {
		return nil, nil
	}
	templates, ok := raw.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("%w: scaffold.templates must be a mapping", errUtils.ErrInvalidScaffoldConfig)
	}
	return templates, nil
}

// expandScaffoldYAMLNode resolves YAML aliases and merge keys while retaining
// scalar tags for the source-aware function decoder. The YAML library owns merge precedence.
func expandScaffoldYAMLNode(node *yaml.Node) (*yaml.Node, error) {
	if node.Kind == yaml.AliasNode {
		return expandScaffoldYAMLNode(node.Alias)
	}
	expanded := *node
	switch node.Kind {
	case yaml.MappingNode:
		var fields map[string]yaml.Node
		if err := node.Decode(&fields); err != nil {
			return nil, err
		}
		expanded.Content = nil
		for _, key := range slices.Sorted(maps.Keys(fields)) {
			value := fields[key]
			child, err := expandScaffoldYAMLNode(&value)
			if err != nil {
				return nil, err
			}
			expanded.Content = append(expanded.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key}, child)
		}
	case yaml.SequenceNode:
		expanded.Content = make([]*yaml.Node, len(node.Content))
		for i, value := range node.Content {
			child, err := expandScaffoldYAMLNode(value)
			if err != nil {
				return nil, err
			}
			expanded.Content[i] = child
		}
	}
	return &expanded, nil
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
