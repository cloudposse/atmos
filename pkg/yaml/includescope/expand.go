package includescope

import (
	"fmt"
	"os"
	"path/filepath"

	yaml "gopkg.in/yaml.v3"

	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/utils"
)

// ExpandLocalYAML expands local YAML includes without converting their tagged nodes
// to plain values. This preserves nested script includes and anchors each nested path
// to the file declaring it. Raw, queried, and remote includes remain with the generic loader.
func (s Scope) ExpandLocalYAML(root *yaml.Node) error {
	defer perf.Track(nil, "includescope.Scope.ExpandLocalYAML")()

	return s.expandLocalYAML(root, map[string]bool{s.File: true})
}

func (s Scope) expandLocalYAML(node *yaml.Node, active map[string]bool) error {
	if node == nil {
		return nil
	}
	if path := s.localYAMLInclude(node); path != "" {
		return s.expandYAMLFile(node, path, active)
	}
	for _, child := range node.Content {
		if err := s.expandLocalYAML(child, active); err != nil {
			return err
		}
	}
	return nil
}

func (s Scope) localYAMLInclude(node *yaml.Node) string {
	if node.Kind != yaml.ScalarNode || node.Tag != includeTag {
		return ""
	}
	resolved := s.Resolve(node.Value)
	ext := filepath.Ext(resolved.LocalPath)
	if !resolved.HasQuery && (ext == ".yaml" || ext == ".yml") {
		return resolved.LocalPath
	}
	return ""
}

func (s Scope) expandYAMLFile(node *yaml.Node, path string, active map[string]bool) error {
	if active[path] {
		return fmt.Errorf("%w: include cycle at %s", utils.ErrIncludeYamlFunctionInvalidFile, path)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("%w: %w", utils.ErrIncludeYamlFunctionInvalidFile, err)
	}
	var included yaml.Node
	if err := yaml.Unmarshal(content, &included); err != nil {
		return fmt.Errorf("%w: %s: %w", utils.ErrIncludeYamlFunctionInvalidFile, path, err)
	}
	if len(included.Content) == 0 {
		*node = yaml.Node{Kind: yaml.ScalarNode, Tag: "!!null", Value: "null"}
		return nil
	}
	*node = *included.Content[0]
	nested := Scope{File: path, BasePath: s.BasePath}
	nested.Rewrite(node)
	active[path] = true
	err = nested.expandLocalYAML(node, active)
	delete(active, path)
	return err
}
