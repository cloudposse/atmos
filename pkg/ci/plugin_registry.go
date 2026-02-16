package ci

import (
	"sort"
	"sync"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
)

var (
	pluginsMu sync.RWMutex
	plugins   = make(map[string]Plugin)
)

// RegisterPlugin registers a CI plugin for a component type.
// Plugins should call this in their init() function for self-registration.
func RegisterPlugin(p Plugin) error {
	defer perf.Track(nil, "ci.RegisterPlugin")()

	if p == nil {
		return errUtils.ErrNilParam
	}

	componentType := p.GetType()
	if componentType == "" {
		return errUtils.Build(errUtils.ErrInvalidConfig).
			WithExplanation("Plugin has empty type").
			Err()
	}

	pluginsMu.Lock()
	defer pluginsMu.Unlock()

	if _, exists := plugins[componentType]; exists {
		return errUtils.Build(errUtils.ErrInvalidConfig).
			WithExplanation("Plugin already registered").
			WithContext("component_type", componentType).
			Err()
	}

	plugins[componentType] = p
	return nil
}

// GetPlugin returns a CI plugin by component type.
func GetPlugin(componentType string) (Plugin, bool) {
	defer perf.Track(nil, "ci.GetPlugin")()

	pluginsMu.RLock()
	defer pluginsMu.RUnlock()

	p, ok := plugins[componentType]
	return p, ok
}

// GetPluginForEvent returns the plugin that handles a specific hook event.
// Returns nil if no plugin handles the event.
func GetPluginForEvent(event string) Plugin {
	defer perf.Track(nil, "ci.GetPluginForEvent")()

	pluginsMu.RLock()
	defer pluginsMu.RUnlock()

	for _, p := range plugins {
		bindings := HookBindings(p.GetHookBindings())
		if bindings.GetBindingForEvent(event) != nil {
			return p
		}
	}
	return nil
}

// ListPlugins returns all registered plugin types.
func ListPlugins() []string {
	defer perf.Track(nil, "ci.ListPlugins")()

	pluginsMu.RLock()
	defer pluginsMu.RUnlock()

	types := make([]string, 0, len(plugins))
	for t := range plugins {
		types = append(types, t)
	}
	sort.Strings(types)
	return types
}

// ClearPlugins removes all registered plugins.
// This is primarily for testing.
func ClearPlugins() {
	defer perf.Track(nil, "ci.ClearPlugins")()

	pluginsMu.Lock()
	defer pluginsMu.Unlock()
	plugins = make(map[string]Plugin)
}
