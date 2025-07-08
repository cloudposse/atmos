package exec

import (
	log "github.com/charmbracelet/log"
)

// isComponentEnabled checks if a component is enabled based on its metadata.
func isComponentEnabled(metadataSection map[string]any, componentName string) bool {
	if enabled, ok := metadataSection["enabled"].(bool); ok {
		if !enabled {
			log.Debug("Skipping disabled", "component", componentName)
			return false
		}
	}
	return true
}
