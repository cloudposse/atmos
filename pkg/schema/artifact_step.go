package schema

import (
	"fmt"
	"reflect"

	"gopkg.in/yaml.v3"
)

// TaskTypeArtifact is the step type of the artifact operations (publish, pull, copy, inspect,
// list, search, delete, sign, verify, and show).
const TaskTypeArtifact = "artifact"

// ArtifactStepMetadata is the `metadata:` block of an artifact publish step. Only labels are
// accepted: they are the explicit user metadata that participates in the publication's intent.
type ArtifactStepMetadata struct {
	// Labels are the exact key/value labels attached to the published artifact version.
	Labels map[string]string `yaml:"labels,omitempty" json:"labels,omitempty" mapstructure:"labels"`
}

// The `query` key is shape-dependent: a mapping holds the query-string parameters of an http step,
// and a scalar holds the search text of an artifact step. The mapstructure path sees the scalar
// form under the search_query key.
const (
	taskMapKeyQuery       = "query"
	taskMapKeySearchQuery = "search_query"
)

// decodeStepQuery routes the polymorphic `query:` key. A mapping is the http step's query-string
// parameters; a scalar is the artifact step's search text. Both live in one YAML key because the
// artifact step contract names its search text `query`, while the http step already owns the
// mapping form.
func decodeStepQuery(node *yaml.Node, params *map[string]string, search *string) error {
	if node == nil {
		return nil
	}
	if node.Kind == yaml.AliasNode {
		node = node.Alias
	}
	if node.Kind == yaml.ScalarNode {
		if node.Tag == "!!null" {
			return nil
		}
		*search = node.Value
		return nil
	}
	if node.Kind != yaml.MappingNode {
		return fmt.Errorf("%w: `query` must be a mapping (http) or a string (artifact search text)", ErrWorkflowControlStepInvalid)
	}
	return node.Decode(params)
}

// normalizeSearchQueryMap moves a scalar `query` into `search_query`, so mapstructure, which cannot
// decode a string into the http query mapping, sees each form under its own key. It returns the
// input map unchanged when `query` is absent or already a mapping, and never mutates the input.
func normalizeSearchQueryMap(m map[string]any) map[string]any {
	value, ok := m[taskMapKeyQuery]
	if !ok {
		return m
	}
	if value == nil || reflect.ValueOf(value).Kind() == reflect.Map {
		return m
	}
	copied := make(map[string]any, len(m))
	for key, val := range m {
		if key != taskMapKeyQuery {
			copied[key] = val
		}
	}
	copied[taskMapKeySearchQuery] = fmt.Sprint(value)
	return copied
}
