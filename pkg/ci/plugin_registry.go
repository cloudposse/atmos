package ci

import (
	"github.com/cloudposse/atmos/pkg/ci/internal/plugin"
)

// RegisterPlugin registers a CI plugin for a component type.
// Plugins should call this in their init() function for self-registration.
func RegisterPlugin(p Plugin) error {
	return plugin.RegisterPlugin(p)
}

// GetPlugin returns a CI plugin by component type.
func GetPlugin(componentType string) (Plugin, bool) {
	return plugin.GetPlugin(componentType)
}

// GetPluginForEvent returns the plugin that handles a specific hook event.
// Returns nil if no plugin handles the event.
func GetPluginForEvent(event string) Plugin {
	return plugin.GetPluginForEvent(event)
}

// ListPlugins returns all registered plugin types.
func ListPlugins() []string {
	return plugin.ListPlugins()
}

// ClearPlugins removes all registered plugins.
// This is primarily for testing.
func ClearPlugins() {
	plugin.ClearPlugins()
}
