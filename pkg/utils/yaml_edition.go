package utils

import (
	"slices"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/cloudposse/atmos/pkg/edition"
	"github.com/cloudposse/atmos/pkg/function/yamlsource"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// ExplicitYAMLTags resolves the automatic function-dispatch policy. A nil
// configuration follows the latest edition, just like an unpinned project.
func ExplicitYAMLTags(config *schema.AtmosConfiguration) (bool, error) {
	defer perf.Track(config, "utils.ExplicitYAMLTags")()
	pin := ""
	if config != nil {
		pin = config.Edition
	}
	return edition.BehaviorChanged(pin, yamlsource.BehaviorKey)
}

func yamlCachePolicy(config *schema.AtmosConfiguration) string {
	if config == nil {
		return ""
	}
	policy := config.Settings.YAML.KeyDelimiter + "\x00edition=" + config.Edition
	if config.PreserveStackLiterals {
		policy += "\x00stack-literals"
	}
	if config.DeferStackIncludes {
		policy += "\x00deferred-stack-includes"
	}
	return policy
}

// preserveLegacyConditionFields records legacy CEL syntax only in structural
// configuration fields; component data sections are never treated as steps.
func preserveLegacyConditionFields(node *yaml.Node, seen map[*yaml.Node]bool) {
	if node == nil || seen[node] || node.Tag == AtmosYamlFuncLiteral {
		return
	}
	seen[node] = true
	if node.Kind == yaml.AliasNode {
		preserveLegacyConditionFields(node.Alias, seen)
		return
	}
	if node.Kind == yaml.MappingNode {
		preserveLegacyConditionMapping(node, seen)
		return
	}
	for _, child := range node.Content {
		preserveLegacyConditionFields(child, seen)
	}
}

func preserveLegacyConditionMapping(node *yaml.Node, seen map[*yaml.Node]bool) {
	for i := 0; i+1 < len(node.Content); i += 2 {
		key, value := node.Content[i].Value, node.Content[i+1]
		if key == "when" || key == "continue" {
			preserveLegacyCondition(value, make(map[*yaml.Node]bool))
			continue
		}
		if !slices.Contains(stackDataSections, key) {
			preserveLegacyConditionFields(value, seen)
		}
	}
}

func preserveLegacyCondition(node *yaml.Node, seen map[*yaml.Node]bool) {
	if node == nil || seen[node] {
		return
	}
	seen[node] = true
	if node.Kind == yaml.AliasNode {
		preserveLegacyCondition(node.Alias, seen)
		return
	}
	if node.Kind == yaml.ScalarNode && node.Tag == "!!str" {
		text := strings.TrimSpace(node.Value)
		if strings.HasPrefix(text, "!cel ") {
			node.Value = (yamlsource.Source{Tag: "!cel", Argument: strings.TrimSpace(strings.TrimPrefix(text, "!cel")), Line: node.Line, Column: node.Column}).Encode()
		}
	}
	for _, child := range node.Content {
		preserveLegacyCondition(child, seen)
	}
}
