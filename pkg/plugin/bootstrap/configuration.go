package bootstrap

import (
	"context"

	"github.com/cloudposse/atmos/pkg/component/configuration"
	configurationadapter "github.com/cloudposse/atmos/pkg/plugin/adapter/configuration"
	"github.com/cloudposse/atmos/pkg/plugin/extension"
	"github.com/cloudposse/atmos/pkg/schema"
)

func init() {
	// Register the constructor, never an invocation's provider or discovered stack map.
	if err := configuration.Register("plugin", newConfigurationSession); err != nil {
		panic(err)
	}
}

func newConfigurationSession(ctx context.Context, config *schema.AtmosConfiguration, operation string) (schema.ComponentConfigurationRuntime, error) {
	snapshot := *config
	return configurationadapter.New(ctx, config, operation, func(call context.Context, names []string) (map[string]configurationadapter.Provider, error) {
		references := make([]ComponentReference, 0, len(names))
		for _, name := range names {
			references = append(references, ComponentReference{Provider: name, Capability: extension.ComponentDiscovery})
		}
		registry, err := LoadComponents(call, &snapshot, references)
		if err != nil {
			return nil, err
		}
		providers := make(map[string]configurationadapter.Provider)
		for _, name := range registry.Names() {
			providers[name], _ = registry.Get(name)
		}
		return providers, nil
	})
}
