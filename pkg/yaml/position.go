package yaml

import (
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/utils"
	goyaml "gopkg.in/yaml.v3"
)

// Position represents a line and column position in a YAML file.
type Position struct {
	Line   int // 1-indexed line number.
	Column int // 1-indexed column number.
}

// PositionMap maps JSONPath-style paths to their positions in a YAML file.
type PositionMap map[string]Position

// ExtractPositions extracts line/column positions from a YAML node tree.
// Returns a map of JSONPath -> Position for all values in the YAML.
// If enabled is false, returns an empty map immediately (zero overhead).
func ExtractPositions(node *goyaml.Node, enabled bool) PositionMap {
	defer perf.Track(nil, "yaml.ExtractPositions")()

	if !enabled || node == nil {
		return make(PositionMap)
	}

	positions := make(PositionMap)
	extractPositionsRecursive(node, "", positions)
	return positions
}

// extractPositionsRecursive recursively walks the YAML node tree and records positions.
//
//nolint:gocognit,revive // YAML node traversal requires multiple cases for different node types.
func extractPositionsRecursive(node *goyaml.Node, currentPath string, positions PositionMap) {
	if node == nil {
		return
	}

	switch node.Kind {
	case goyaml.DocumentNode:
		// Document node wraps the actual content.
		if len(node.Content) > 0 {
			extractPositionsRecursive(node.Content[0], currentPath, positions)
		}

	case goyaml.MappingNode:
		// Map: pairs of key-value nodes.
		for i := 0; i < len(node.Content); i += 2 {
			if i+1 >= len(node.Content) {
				break
			}

			keyNode := node.Content[i]
			valueNode := node.Content[i+1]

			// Get the key as a string.
			key := keyNode.Value

			// Build the path for this key.
			var path string
			if currentPath == "" {
				path = key
			} else {
				path = utils.AppendJSONPathKey(currentPath, key)
			}

			// Record position for this value.
			positions[path] = Position{
				Line:   valueNode.Line,
				Column: valueNode.Column,
			}

			// Recurse into the value.
			extractPositionsRecursive(valueNode, path, positions)
		}

	case goyaml.SequenceNode:
		// Array: list of nodes.
		for i, itemNode := range node.Content {
			// Build the path with array index.
			path := utils.AppendJSONPathIndex(currentPath, i)

			// Record position for this item.
			positions[path] = Position{
				Line:   itemNode.Line,
				Column: itemNode.Column,
			}

			// Recurse into the item.
			extractPositionsRecursive(itemNode, path, positions)
		}

	case goyaml.ScalarNode:
		// Leaf value - position already recorded by parent.
		// Nothing to do here.

	case goyaml.AliasNode:
		// YAML alias (*anchor) - recurse into the aliased node.
		if node.Alias != nil {
			extractPositionsRecursive(node.Alias, currentPath, positions)
		}
	}
}

// GetPosition gets the position for a specific JSONPath from the position map.
// Returns Position{0, 0} if not found.
func GetPosition(positions PositionMap, path string) Position {
	defer perf.Track(nil, "yaml.GetPosition")()

	if positions == nil {
		return Position{}
	}

	pos, exists := positions[path]
	if !exists {
		return Position{}
	}

	return pos
}

// HasPosition checks if a position exists for a specific JSONPath.
func HasPosition(positions PositionMap, path string) bool {
	defer perf.Track(nil, "yaml.HasPosition")()

	if positions == nil {
		return false
	}

	_, exists := positions[path]
	return exists
}
