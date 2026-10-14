package doctor

import (
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/cloudposse/atmos/pkg/tags"
)

// templateMarkers returns the left delimiters Atmos renders in a document of this kind.
// Stack manifests use stack-level settings.templates.settings.delimiters, then
// templates.settings.delimiters from atmos.yaml, then the Go defaults. Custom commands and
// vendor manifests always render with the Go defaults. GitHub workflows are not Atmos
// templates, so their ${{ }} expressions are literal text.
func (p *project) templateMarkers(d *document) []string {
	switch d.kind {
	case "stack":
		// A stack-level override replaces the atmos.yaml delimiters rather than adding to them, as
		// the runtime renderer does, so "{{" is literal text in a stack that switches to "<%".
		var overrides []string
		walk(d.node, nil, func(n *yaml.Node, path []string) {
			if !delimiterOverridePath(path) || n.Kind != yaml.SequenceNode {
				return
			}
			var configured []string
			if err := n.Decode(&configured); err == nil && len(configured) == 2 {
				override, _ := tags.TemplateDelims(configured)
				overrides = append(overrides, override)
			}
		})
		if len(overrides) > 0 {
			return overrides
		}
		left, _ := tags.TemplateDelims(stringList(p.config, "templates", "settings", "delimiters"))
		return []string{left}
	case documentConfig, "vendor":
		return []string{tags.DefaultLeftDelim}
	default:
		return nil
	}
}

// delimiterOverridePath matches stack- and component-level settings.templates.settings.delimiters.
func delimiterOverridePath(path []string) bool {
	suffix := []string{"settings", "templates", "settings", "delimiters"}
	if len(path) < len(suffix) {
		return false
	}
	for i, key := range suffix {
		if path[len(path)-len(suffix)+i] != key {
			return false
		}
	}
	return true
}

func stringList(m map[string]any, keys ...string) []string {
	var v any = m
	for _, key := range keys {
		object, ok := v.(map[string]any)
		if !ok {
			return nil
		}
		v = object[key]
	}
	items, _ := v.([]any)
	result := make([]string, 0, len(items))
	for _, item := range items {
		s, ok := item.(string)
		if !ok {
			return nil
		}
		result = append(result, s)
	}
	return result
}

func containsTemplate(value string, markers []string) bool {
	for _, marker := range markers {
		if marker != "" && strings.Contains(value, marker) {
			return true
		}
	}
	return false
}
