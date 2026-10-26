package yamlsource

import "gopkg.in/yaml.v3"

// RestoreTags restores explicit tags for public YAML output and YAML template
// passes. Plain strings, including quoted function expressions, are unchanged.
//
//nolint:lintroller // Cannot import perf: schema conditions depend on this leaf package.
func RestoreTags(node *yaml.Node) {
	if node == nil {
		return
	}
	if node.Kind == yaml.ScalarNode {
		if source, ok := Decode(node.Value); ok {
			node.Tag, node.Value = source.Tag, source.Argument
			node.Style = yaml.TaggedStyle | yaml.DoubleQuotedStyle
		}
	}
	for _, child := range node.Content {
		RestoreTags(child)
	}
}

// Display returns an ordinary representation for output formats without YAML
// tags. Internal JSON cloning should retain the transport strings instead.
//
//nolint:lintroller // Cannot import perf: schema conditions depend on this leaf package.
func Display(value any) any {
	switch v := value.(type) {
	case string:
		return Expression(v)
	case map[string]any:
		result := make(map[string]any, len(v))
		for key, item := range v {
			result[key] = Display(item)
		}
		return result
	case map[any]any:
		result := make(map[any]any, len(v))
		for key, item := range v {
			result[key] = Display(item)
		}
		return result
	case []any:
		result := make([]any, len(v))
		for i, item := range v {
			result[i] = Display(item)
		}
		return result
	default:
		return value
	}
}
