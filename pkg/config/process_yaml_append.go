package config

import (
	"fmt"

	"github.com/spf13/viper"
	"go.yaml.in/yaml/v3"

	log "github.com/cloudposse/atmos/pkg/logger"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// handleAppend processes a sequence node with an !append tag.
// It wraps the list with metadata to indicate it should be appended during merging.
func handleAppend(node *yaml.Node, v *viper.Viper, currentPath string) error {
	log.Debug("Processing !append tag", "path", currentPath)

	// Build the list from the sequence node. Each item is resolved via resolveAppendItem
	// so custom Atmos tags inside the list (e.g. !env, !exec, !cwd) — including tags
	// nested inside map or sub-list items — are evaluated here, matching the stack-manifest
	// path (rewriteAppendNode) instead of being silently left unevaluated.
	var list []any
	for _, child := range node.Content {
		value, err := resolveAppendItem(child)
		if err != nil {
			log.Debug("Failed to process list item", "error", err)
			return fmt.Errorf("%w: failed to process list item in !append: %w", ErrExecuteYamlFunctions, err)
		}
		list = append(list, value)
	}

	// Wrap the list with append metadata.
	wrappedValue := u.WrapWithAppendTag(list)

	// Set the wrapped value in Viper.
	v.Set(currentPath, wrappedValue)

	// Clear the tag to avoid re-processing.
	node.Tag = ""

	return nil
}

// resolveAppendItem resolves a single !append list item to a concrete value, evaluating
// any custom Atmos scalar tags it contains — including tags nested inside maps or sub-lists.
// Unlike processSequenceElement, it does NOT write intermediate indexed keys into Viper
// (which would leak here because the !append value is stored as a wrapper map, not an array,
// so the array-index cleanup would never remove them).
func resolveAppendItem(node *yaml.Node) (any, error) {
	switch node.Kind {
	case yaml.MappingNode:
		result := make(map[string]any, len(node.Content)/2)
		for i := 0; i+1 < len(node.Content); i += 2 {
			val, err := resolveAppendItem(node.Content[i+1])
			if err != nil {
				return nil, err
			}
			result[node.Content[i].Value] = val
		}
		return result, nil
	case yaml.SequenceNode:
		result := make([]any, 0, len(node.Content))
		for _, child := range node.Content {
			val, err := resolveAppendItem(child)
			if err != nil {
				return nil, err
			}
			result = append(result, val)
		}
		return result, nil
	default:
		// Scalars (tagged or plain) and aliases: resolve the tag or decode the value.
		return processScalarNodeValue(node)
	}
}
