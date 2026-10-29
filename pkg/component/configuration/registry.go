// Package configuration composes alternate configuration formats with built-in component engines.
package configuration

import (
	"context"
	"sync"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// Factory constructs an invocation-local session. Registration stores no invocation state.
type Factory func(context.Context, *schema.AtmosConfiguration, string) (schema.ComponentConfigurationRuntime, error)

var factories = struct {
	sync.RWMutex
	values map[string]Factory
}{values: make(map[string]Factory)}

// Register adds a configuration service factory during host composition.
func Register(name string, factory Factory) error {
	defer perf.Track(nil, "configuration.Register")()

	if name == "" || factory == nil {
		return errUtils.ErrPluginInvalidConfig
	}
	factories.Lock()
	defer factories.Unlock()
	if _, exists := factories.values[name]; exists {
		return errUtils.ErrDuplicatePluginExtension
	}
	factories.values[name] = factory
	return nil
}

// Attach creates private state only when automatic configuration discovery is enabled.
func Attach(ctx context.Context, config *schema.AtmosConfiguration, operation string) error {
	defer perf.Track(config, "configuration.Attach")()

	if !config.HasComponentDiscovery() {
		return nil
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	factories.RLock()
	factory := factories.values["plugin"]
	factories.RUnlock()
	if factory == nil {
		return errUtils.ErrPluginUnsupportedExtension
	}
	session, err := factory(ctx, config, operation)
	if err != nil {
		return err
	}
	config.ComponentConfigurations = session
	return nil
}
