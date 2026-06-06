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

	// Build the list from the sequence node.
	var list []any
	for _, child := range node.Content {
		var value any
		if err := child.Decode(&value); err != nil {
			log.Debug("Failed to decode list item", "error", err)
			return fmt.Errorf("%w: failed to decode list item in !append: %w", ErrExecuteYamlFunctions, err)
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
