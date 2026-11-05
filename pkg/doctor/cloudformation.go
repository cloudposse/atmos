package doctor

import (
	"fmt"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"

	cfnmanifest "github.com/cloudposse/atmos/pkg/component/aws/cloudformation/manifest"
)

// cloudFormationScope limits repairs to actual defaults and component declarations.
func cloudFormationScope(path []string) bool {
	if len(path) == 0 {
		return true
	}
	if len(path) == 1 {
		return path[0] == "aws/cloudformation"
	}
	if len(path) < 3 || path[0] != "components" || path[1] != "aws/cloudformation" {
		return false
	}
	return len(path) == 3 || (len(path) == 4 && path[3] == "overrides")
}

// cloudFormationCandidate recognizes literal values without evaluating YAML functions or templates.
func cloudFormationCandidate(d *document, n *yaml.Node, path []string) *edit {
	if d.kind != documentStack || n.Kind != yaml.ScalarNode {
		return nil
	}
	if candidate := cloudFormationRegionCandidate(d, n, path); candidate != nil {
		return candidate
	}
	if len(path) > 0 && path[len(path)-1] == "timeout_in_minutes" && n.Tag == "!!int" && cloudFormationScope(path[:len(path)-1]) {
		return &edit{doc: d, path: path[:len(path)-1], kind: ruleCloudFormationTimeout, old: "timeout_in_minutes", value: n.Value + "m"}
	}
	return nil
}

// cloudFormationRegionCandidate identifies the nested legacy region path.
func cloudFormationRegionCandidate(d *document, n *yaml.Node, path []string) *edit {
	if len(path) >= 3 && slices.Equal(path[len(path)-3:], []string{"settings", "aws_cloudformation", "region"}) && n.Tag == "!!str" {
		scope := path[:len(path)-3]
		if cloudFormationScope(scope) {
			return &edit{doc: d, path: scope, kind: ruleCloudFormationRegion, old: "settings.aws_cloudformation.region"}
		}
	}
	return nil
}

// normalizeCloudFormationMigration compares the moved value and every unrelated YAML node.
func normalizeCloudFormationMigration(node *yaml.Node, e *edit) error {
	if e.kind == ruleCloudFormationTimeout {
		var section map[string]any
		if err := node.Decode(&section); err != nil {
			return err
		}
		timeout, err := cfnmanifest.Timeout(section)
		if err != nil {
			return err
		}
		removeKey(node, "timeout_in_minutes")
		removeKey(node, "timeout")
		node.Content = append(node.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: "timeout"}, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: timeout.String()})
		return nil
	}
	legacy := nodeAt(node, strings.Split(e.old, "."))
	modern := mappingValue(node, "region")
	if (legacy == nil) == (modern == nil) {
		return fmt.Errorf("%w: ambiguous CloudFormation region", ErrVerification)
	}
	if modern != nil {
		parent := nodeAt(node, []string{"settings", "aws_cloudformation"})
		if parent == nil || parent.Kind != yaml.MappingNode {
			return ErrVerification
		}
		removeKey(node, "region")
		parent.Content = append(parent.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: "region"}, modern)
	}
	return nil
}

func hasCloudFormationMigration(candidates []candidate) bool {
	for _, candidate := range candidates {
		for _, change := range candidate.edits {
			if change.kind == ruleCloudFormationRegion || change.kind == ruleCloudFormationTimeout {
				return true
			}
		}
	}
	return false
}
