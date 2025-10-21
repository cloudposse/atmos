package exec

import (
	log "github.com/cloudposse/atmos/pkg/logger"
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

// isComponentLocked checks if a component is locked based on its metadata.
// https://atmos.tools/core-concepts/stacks/define-components/#locking-components-with-metadatalocked.
func isComponentLocked(metadataSection map[string]any) bool {
	if locked, ok := metadataSection["locked"].(bool); ok {
		if locked {
			return true
		}
	}
	return false
}
