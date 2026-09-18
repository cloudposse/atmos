package githubactions

import (
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/rhysd/actionlint"
	"go.yaml.in/yaml/v3"
)

// The bundled actionlint predates cache-mode. Validate only that new key and
// suppress its unsupported-key diagnostic at the exact YAML position. All other
// syntax errors (including misplaced keys and duplicates) remain visible.
func cacheModeDiagnostics(root string, errors []*actionlint.Error) ([]*actionlint.Error, bool) {
	result := make([]*actionlint.Error, 0, len(errors))
	changed := false
	for _, diagnostic := range errors {
		if diagnostic.Kind != "syntax-check" || !strings.HasPrefix(diagnostic.Message, `unexpected key "cache-mode"`) {
			result = append(result, diagnostic)
			continue
		}
		value := cacheModeAt(root, diagnostic)
		if value == nil {
			result = append(result, diagnostic)
			continue
		}
		changed = true
		if value.Kind == yaml.AliasNode && value.Alias != nil {
			value = value.Alias
		}
		if value.Kind == yaml.ScalarNode && value.Tag == "!!str" && slices.Contains([]string{"read", "write", "write-only", "none"}, value.Value) {
			continue
		}
		copy := *diagnostic
		copy.Message = "cache-mode must be one of read, write, write-only, or none"
		result = append(result, &copy)
	}
	return result, changed
}

func cacheModeAt(root string, diagnostic *actionlint.Error) *yaml.Node {
	path := diagnostic.Filepath
	if !filepath.IsAbs(path) {
		path = filepath.Join(root, path)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var document yaml.Node
	if yaml.Unmarshal(data, &document) != nil || len(document.Content) != 1 {
		return nil
	}
	return cacheModeInWorkflow(document.Content[0], diagnostic)
}

func cacheModeInWorkflow(workflow *yaml.Node, diagnostic *actionlint.Error) *yaml.Node {
	if node := cacheModeInMapping(workflow, diagnostic); node != nil {
		return node
	}
	for index := 0; index+1 < len(workflow.Content); index += 2 {
		if workflow.Content[index].Value != "jobs" {
			continue
		}
		jobs := workflow.Content[index+1]
		if jobs.Kind != yaml.MappingNode {
			continue
		}
		for job := 1; job < len(jobs.Content); job += 2 {
			if node := cacheModeInMapping(jobs.Content[job], diagnostic); node != nil {
				return node
			}
		}
	}
	return nil
}

func cacheModeInMapping(mapping *yaml.Node, diagnostic *actionlint.Error) *yaml.Node {
	if mapping.Kind != yaml.MappingNode {
		return nil
	}
	for index := 0; index+1 < len(mapping.Content); index += 2 {
		key := mapping.Content[index]
		if key.Value == "cache-mode" && key.Line == diagnostic.Line && key.Column == diagnostic.Column {
			return mapping.Content[index+1]
		}
	}
	return nil
}
