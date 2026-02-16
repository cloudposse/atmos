package ci

import (
	"embed"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

// createMockPlugin creates a MockPlugin with the given type and bindings.
func createMockPlugin(ctrl *gomock.Controller, componentType string, bindings []HookBinding) *MockPlugin {
	mock := NewMockPlugin(ctrl)
	mock.EXPECT().GetType().Return(componentType).AnyTimes()
	mock.EXPECT().GetHookBindings().Return(bindings).AnyTimes()
	mock.EXPECT().GetDefaultTemplates().Return(embed.FS{}).AnyTimes()
	return mock
}

func TestRegisterPlugin(t *testing.T) {
	// Clear registry before test.
	ClearPlugins()

	t.Run("successful registration", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		plugin := createMockPlugin(ctrl, "test-type", nil)
		err := RegisterPlugin(plugin)
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
		ctrl := gomock.NewController(t)
		plugin := createMockPlugin(ctrl, "", nil)
		err := RegisterPlugin(plugin)
		require.Error(t, err)
	})

	t.Run("duplicate registration", func(t *testing.T) {
		ClearPlugins()
		ctrl := gomock.NewController(t)
		plugin := createMockPlugin(ctrl, "duplicate-type", nil)
		err := RegisterPlugin(plugin)
		require.NoError(t, err)

		// Second registration should fail.
		err = RegisterPlugin(plugin)
		require.Error(t, err)
	})
}

func TestGetPluginForEvent(t *testing.T) {
	ClearPlugins()
	ctrl := gomock.NewController(t)

	bindings := []HookBinding{
		{Event: "after.test-terraform.plan", Actions: []HookAction{ActionSummary}},
		{Event: "after.test-terraform.apply", Actions: []HookAction{ActionSummary}},
	}
	plugin := createMockPlugin(ctrl, "test-terraform", bindings)
	err := RegisterPlugin(plugin)
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
	ctrl := gomock.NewController(t)

	// Register multiple plugins.
	pluginNames := []string{"alpha", "beta", "gamma"}
	for _, name := range pluginNames {
		plugin := createMockPlugin(ctrl, name, nil)
		err := RegisterPlugin(plugin)
		require.NoError(t, err)
	}

	// List should return all plugins sorted.
	list := ListPlugins()
	assert.Equal(t, []string{"alpha", "beta", "gamma"}, list)
}

func TestHookBindingsGetBindingForEvent(t *testing.T) {
	bindings := HookBindings{
		{Event: "after.terraform.plan", Actions: []HookAction{ActionSummary}, Template: "plan"},
		{Event: "after.terraform.apply", Actions: []HookAction{ActionSummary}, Template: "apply"},
	}

	t.Run("found", func(t *testing.T) {
		b := bindings.GetBindingForEvent("after.terraform.plan")
		require.NotNil(t, b)
		assert.Equal(t, "plan", b.Template)
	})

	t.Run("not found", func(t *testing.T) {
		b := bindings.GetBindingForEvent("before.terraform.init")
		assert.Nil(t, b)
	})
}

func TestHookBindingHasAction(t *testing.T) {
	binding := HookBinding{
		Event:   "after.terraform.plan",
		Actions: []HookAction{ActionSummary, ActionOutput, ActionUpload},
	}

	assert.True(t, binding.HasAction(ActionSummary))
	assert.True(t, binding.HasAction(ActionOutput))
	assert.True(t, binding.HasAction(ActionUpload))
	assert.False(t, binding.HasAction(ActionDownload))
	assert.False(t, binding.HasAction(ActionCheck))
}
