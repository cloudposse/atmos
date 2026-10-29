package configuration

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestFactoryRegistrationAndOptIn(t *testing.T) {
	factories.Lock()
	previous := factories.values
	factories.values = make(map[string]Factory)
	factories.Unlock()
	t.Cleanup(func() { factories.Lock(); factories.values = previous; factories.Unlock() })
	config := &schema.AtmosConfiguration{}
	require.NoError(t, Attach(t.Context(), config, "list"))
	require.Nil(t, config.ComponentConfigurations)
	settings := schema.ComponentConfiguration{Enabled: true, Discovery: schema.ComponentDiscoveryConfig{Enabled: true}}
	config.Components.Terraform.ConfigurationProviders = map[string]schema.ComponentConfiguration{"sample": settings}
	require.ErrorIs(t, Attach(t.Context(), config, "list"), errUtils.ErrPluginUnsupportedExtension)
	require.ErrorIs(t, Register("", func(context.Context, *schema.AtmosConfiguration, string) (schema.ComponentConfigurationRuntime, error) {
		return nil, nil
	}), errUtils.ErrPluginInvalidConfig)
	require.ErrorIs(t, Register("plugin", nil), errUtils.ErrPluginInvalidConfig)
	called := false
	factory := func(ctx context.Context, got *schema.AtmosConfiguration, operation string) (schema.ComponentConfigurationRuntime, error) {
		called = true
		require.Same(t, config, got)
		require.Equal(t, "list", operation)
		return nil, errUtils.ErrPluginInvalidConfig
	}
	require.NoError(t, Register("plugin", factory))
	require.ErrorIs(t, Register("plugin", factory), errUtils.ErrDuplicatePluginExtension)
	require.False(t, called)
	require.ErrorIs(t, Attach(t.Context(), config, "list"), errUtils.ErrPluginInvalidConfig)
	require.True(t, called)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	require.ErrorIs(t, Attach(ctx, config, "list"), context.Canceled)
	factories.Lock()
	factories.values["plugin"] = func(context.Context, *schema.AtmosConfiguration, string) (schema.ComponentConfigurationRuntime, error) {
		return nil, nil
	}
	factories.Unlock()
	require.NoError(t, Attach(t.Context(), config, "list"))
}
