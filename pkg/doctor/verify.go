package doctor

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/cloudposse/atmos/pkg/hooks"
	"github.com/cloudposse/atmos/pkg/schema"
)

// verifyDocument uses the real field consumers and compares every other authored value.
// It intentionally refuses transformations that lose comments.
func verifyDocument(before, after []byte, edits []edit) error {
	a, err := parse(before)
	if err != nil {
		return err
	}
	b, err := parse(after)
	if err != nil {
		return err
	}
	if err = verifyComments(a, b, edits); err != nil {
		return err
	}
	am, err := semanticDocument(a, edits)
	if err != nil {
		return err
	}
	bm, err := semanticDocument(b, edits)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(am, bm) {
		return fmt.Errorf("%w: effective or unrelated YAML values changed", ErrVerification)
	}
	return nil
}

func semanticDocument(n *yaml.Node, edits []edit) (any, error) {
	for _, e := range edits {
		node := nodeAt(rootNode(n), e.path)
		if node == nil {
			return nil, ErrVerification
		}
		if err := normalizeNode(node, &e); err != nil {
			return nil, err
		}
	}
	return canonicalNode(rootNode(n)), nil
}

func canonicalNode(n *yaml.Node) any {
	if n == nil {
		return nil
	}
	switch n.Kind {
	case yaml.MappingNode:
		m := map[string]any{}
		for i := 0; i+1 < len(n.Content); i += 2 {
			key, _ := json.Marshal(canonicalNode(n.Content[i]))
			m[string(key)] = canonicalNode(n.Content[i+1])
		}
		return m
	case yaml.SequenceNode:
		result := []any{}
		for _, child := range n.Content {
			result = append(result, canonicalNode(child))
		}
		return result
	case yaml.AliasNode:
		return []string{n.Tag, n.Value, n.Anchor}
	default:
		return []string{n.Tag, n.Value, n.Anchor}
	}
}

func nodeAt(n *yaml.Node, path []string) *yaml.Node {
	for _, part := range path {
		if n == nil {
			return nil
		}
		if n.Kind == yaml.SequenceNode {
			index, err := strconv.Atoi(part)
			if err != nil || index < 0 || index >= len(n.Content) {
				return nil
			}
			n = n.Content[index]
		} else {
			n = mappingValue(n, part)
		}
	}
	return n
}

func removeKey(n *yaml.Node, key string) {
	for i := 0; i+1 < len(n.Content); i += 2 {
		if n.Content[i].Value == key {
			n.Content = append(n.Content[:i], n.Content[i+2:]...)
			return
		}
	}
}

func normalizeNode(node *yaml.Node, e *edit) error {
	switch e.kind {
	case ruleLegacyNaming:
		return normalizeNaming(node, e)
	case ruleCloudFormationRegion, ruleCloudFormationTimeout:
		return normalizeCloudFormationMigration(node, e)
	case ruleComponentDependencies:
		return normalizeDependencyMigration(node)
	case ruleCommandProvides:
		return normalizeProvides(node, e)
	case ruleComponentMonorepo:
		return normalizeMonorepoSource(node, e)
	case ruleHookEvent:
		node.Value = string(hooks.HookEvent(strings.ReplaceAll(node.Value, "-", dot)).Normalize())
	case ruleTerraformReconfigure:
		return normalizeReconfigure(node)
	}
	return nil
}

// normalizeProvides replaces a legacy semantic type with the provides value the runtime derives from it.
func normalizeProvides(node *yaml.Node, e *edit) error {
	var effective string
	if e.old == "type" {
		var a schema.CommandArgument
		if err := node.Decode(&a); err != nil {
			return err
		}
		effective = a.EffectiveProvides()
	} else {
		var f schema.CommandFlag
		if err := node.Decode(&f); err != nil {
			return err
		}
		effective = f.EffectiveProvides()
	}
	removeKey(node, e.old)
	removeKey(node, "provides")
	node.Content = append(node.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: "provides"}, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: effective})
	return nil
}

// normalizeReconfigure replaces the legacy reconfigure flag with the effective policy the runtime derives from it.
func normalizeReconfigure(node *yaml.Node) error {
	value := schema.Terraform{InitRunReconfigure: true}
	if err := node.Decode(&value); err != nil {
		return err
	}
	effective := string(value.EffectiveInitReconfigure())
	removeKey(node, "init_run_reconfigure")
	init := mappingValue(node, "init")
	if init != nil {
		removeKey(init, "reconfigure")
		if len(init.Content) == 0 {
			removeKey(node, "init")
		}
	}
	node.Content = append(node.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: "init_run_reconfigure"}, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: effective})
	return nil
}
