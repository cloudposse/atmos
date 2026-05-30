package ci

import (
	"testing"

	plugin "github.com/cloudposse/atmos/pkg/ci/internal/plugin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRegisterPlugin(t *testing.T) {
	// Clear registry before test.
	ClearPlugins()

	t.Run("successful registration", func(t *testing.T) {
		sp := &stubPlugin{componentType: "test-type"}
		err := RegisterPlugin(sp)
		require.NoError(t, err)

		// Verify registration.
		p, ok := GetPlugin("test-type")
		assert.True(t, ok)
		assert.Equal(t, "test-type", p.GetType())
	})

	t.Run("nil provider", func(t *testing.T) {
		err := RegisterPlugin(nil)
		require.Error(t, err)
	})

	t.Run("empty type", func(t *testing.T) {
		ClearPlugins()
		sp := &stubPlugin{componentType: ""}
		err := RegisterPlugin(sp)
		require.Error(t, err)
	})

	t.Run("duplicate registration", func(t *testing.T) {
		ClearPlugins()
		sp := &stubPlugin{componentType: "duplicate-type"}
		err := RegisterPlugin(sp)
		require.NoError(t, err)

		// Second registration should fail.
		err = RegisterPlugin(sp)
		require.Error(t, err)
	})
}

func TestGetPluginForEvent(t *testing.T) {
	ClearPlugins()

	bindings := []plugin.HookBinding{
		{Event: "after.test-terraform.plan", Handler: func(_ *plugin.HookContext) error { return nil }},
		{Event: "after.test-terraform.apply", Handler: func(_ *plugin.HookContext) error { return nil }},
	}
	sp := &stubPlugin{componentType: "test-terraform", bindings: bindings}
	err := RegisterPlugin(sp)
	require.NoError(t, err)

	t.Run("event found", func(t *testing.T) {
		p := GetPluginForEvent("after.test-terraform.plan")
		require.NotNil(t, p)
		assert.Equal(t, "test-terraform", p.GetType())
	})

	t.Run("event not found", func(t *testing.T) {
		p := GetPluginForEvent("after.unknown.plan")
		assert.Nil(t, p)
	})
}

func TestListPlugins(t *testing.T) {
	ClearPlugins()

	// Register multiple plugins.
	pluginNames := []string{"alpha", "beta", "gamma"}
	for _, name := range pluginNames {
		sp := &stubPlugin{componentType: name}
		err := RegisterPlugin(sp)
		require.NoError(t, err)
	}

	// List should return all plugins sorted.
	list := ListPlugins()
	assert.Equal(t, []string{"alpha", "beta", "gamma"}, list)
}

func TestHookBindingsGetBindingForEvent(t *testing.T) {
	bindings := plugin.HookBindings{
		{Event: "after.terraform.plan", Handler: func(_ *plugin.HookContext) error { return nil }},
		{Event: "after.terraform.apply", Handler: func(_ *plugin.HookContext) error { return nil }},
	}

	t.Run("found", func(t *testing.T) {
		b := bindings.GetBindingForEvent("after.terraform.plan")
		require.NotNil(t, b)
		assert.Equal(t, "after.terraform.plan", b.Event)
	})

	t.Run("not found", func(t *testing.T) {
		b := bindings.GetBindingForEvent("before.terraform.init")
		assert.Nil(t, b)
	})
}
