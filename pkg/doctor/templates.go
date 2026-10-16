package doctor

import (
	"slices"
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
		// The stack-level settings.templates.settings.delimiters replaces the atmos.yaml delimiters
		// for the whole manifest, as the runtime renderer does, so "{{" is literal text in a stack
		// that switches to "<%". A component-level override applies only to that component, so its
		// marker is added while siblings keep the stack-wide one.
		stackWide := ""
		var componentLevel []string
		walk(d.node, nil, func(n *yaml.Node, path []string) {
			if !delimiterOverridePath(path) || n.Kind != yaml.SequenceNode {
				return
			}
			var configured []string
			if err := n.Decode(&configured); err != nil || len(configured) != 2 {
				return
			}
			override, _ := tags.TemplateDelims(configured)
			if len(path) == len(delimiterSettingsPath) {
				stackWide = override
				return
			}
			componentLevel = append(componentLevel, override)
		})
		if stackWide == "" {
			stackWide, _ = tags.TemplateDelims(stringList(p.config, "templates", "settings", "delimiters"))
		}
		markers := []string{stackWide}
		for _, marker := range componentLevel {
			if !slices.Contains(markers, marker) {
				markers = append(markers, marker)
			}
		}
		return markers
	case documentConfig, "vendor":
		return []string{tags.DefaultLeftDelim}
	default:
		return nil
	}
}

// delimiterSettingsPath is the key path of a template delimiter override.
var delimiterSettingsPath = []string{"settings", "templates", "settings", "delimiters"}

// delimiterOverridePath matches stack- and component-level settings.templates.settings.delimiters.
func delimiterOverridePath(path []string) bool {
	suffix := delimiterSettingsPath
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
