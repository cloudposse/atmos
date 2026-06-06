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

	// Build the list from the sequence node. Each item is run through
	// processScalarNodeValue so custom scalar tags inside the list (e.g. !env,
	// !exec, !cwd) are resolved here, matching the stack-manifest path
	// (rewriteAppendNode) instead of being silently left unevaluated.
	var list []any
	for _, child := range node.Content {
		value, err := processScalarNodeValue(child)
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
