package policy

import (
	"bytes"
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/tags"
)

// UnmarshalYAML recognizes literal !any/!all tag lists without executing YAML functions.
func (s *Selector) UnmarshalYAML(node *yaml.Node) error {
	defer perf.Track(nil, "policy.Selector.UnmarshalYAML")()
	if node.Kind != yaml.MappingNode {
		return fmt.Errorf("%w: match must be a mapping", errUtils.ErrPolicyInvalid)
	}
	var mode tags.TagMode
	for i := 0; i < len(node.Content); i += 2 {
		key, value := node.Content[i], node.Content[i+1]
		taggedMode, err := selectorTagMode(key.Value, value)
		if err != nil {
			return err
		}
		if taggedMode != "" {
			mode = taggedMode
		}
	}
	// Decode through an alias with a strict decoder: Node.Decode alone ignores unknown fields.
	type literalSelector Selector
	raw, err := yaml.Marshal(node)
	if err != nil {
		return err
	}
	decoder := yaml.NewDecoder(bytes.NewReader(raw))
	decoder.KnownFields(true)
	var result literalSelector
	if err := decoder.Decode(&result); err != nil {
		return fmt.Errorf("%w: selector: %w", errUtils.ErrPolicyInvalid, err)
	}
	if mode != "" {
		result.TagMode = mode
	}
	*s = Selector(result)
	return s.validate()
}

func selectorTagMode(field string, node *yaml.Node) (tags.TagMode, error) {
	if node.Kind == yaml.AliasNode {
		node = node.Alias
	}
	if field == "tags" && (node.Tag == "!any" || node.Tag == "!all") {
		if node.Kind != yaml.SequenceNode {
			return "", fmt.Errorf("%w: !any and !all require a tag list", errUtils.ErrPolicyInvalid)
		}
		for _, child := range node.Content {
			if err := validateLiteralSelectorNode(child); err != nil {
				return "", err
			}
		}
		return tags.TagMode(strings.TrimPrefix(node.Tag, "!")), nil
	}
	return "", validateLiteralSelectorNode(node)
}

func validateLiteralSelectorNode(node *yaml.Node) error {
	// Aliases are already bounded by the YAML decoder. Reject them inside values to avoid
	// recursive structures or hiding an executable tag behind an alias.
	if node.Kind == yaml.AliasNode {
		return fmt.Errorf("%w: selector values must be literal", errUtils.ErrPolicyInvalid)
	}
	if strings.HasPrefix(node.Tag, "!") && !strings.HasPrefix(node.Tag, "!!") {
		return fmt.Errorf("%w: only tags: !any or tags: !all annotations are supported", errUtils.ErrPolicyInvalid)
	}
	for _, child := range node.Content {
		if err := validateLiteralSelectorNode(child); err != nil {
			return err
		}
	}
	return nil
}
