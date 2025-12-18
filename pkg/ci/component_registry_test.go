package ci

import (
	"embed"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

// mockComponentProvider is a test implementation of ComponentCIProvider.
type mockComponentProvider struct {
	componentType string
	bindings      []HookBinding
}

func (m *mockComponentProvider) GetType() string {
	return m.componentType
}

func (m *mockComponentProvider) GetHookBindings() []HookBinding {
	return m.bindings
}

func (m *mockComponentProvider) GetDefaultTemplates() embed.FS {
	return embed.FS{}
}

func (m *mockComponentProvider) BuildTemplateContext(
	info *schema.ConfigAndStacksInfo,
	ciCtx *Context,
	output string,
	command string,
) (any, error) {
	return &TemplateContext{
		Component:     info.ComponentFromArg,
		ComponentType: m.componentType,
		Stack:         info.Stack,
		Command:       command,
	}, nil
}

func (m *mockComponentProvider) ParseOutput(output string, command string) (*OutputResult, error) {
	return &OutputResult{
		ExitCode:   0,
		HasChanges: false,
		HasErrors:  false,
	}, nil
}

func (m *mockComponentProvider) GetOutputVariables(result *OutputResult, command string) map[string]string {
	return map[string]string{
		"test": "value",
	}
}

func (m *mockComponentProvider) GetArtifactKey(info *schema.ConfigAndStacksInfo, command string) string {
	return info.Stack + "/" + info.ComponentFromArg + ".tfplan"
}

func TestRegisterComponentProvider(t *testing.T) {
	// Clear registry before test.
	ClearComponentProviders()

	t.Run("successful registration", func(t *testing.T) {
		provider := &mockComponentProvider{componentType: "test-type"}
		err := RegisterComponentProvider(provider)
		require.NoError(t, err)

		// Verify registration.
		p, ok := GetComponentProvider("test-type")
		assert.True(t, ok)
		assert.Equal(t, "test-type", p.GetType())
	})

	t.Run("nil provider", func(t *testing.T) {
		err := RegisterComponentProvider(nil)
		require.Error(t, err)
	})

	t.Run("empty type", func(t *testing.T) {
		ClearComponentProviders()
		provider := &mockComponentProvider{componentType: ""}
		err := RegisterComponentProvider(provider)
		require.Error(t, err)
	})

	t.Run("duplicate registration", func(t *testing.T) {
		ClearComponentProviders()
		provider := &mockComponentProvider{componentType: "duplicate-type"}
		err := RegisterComponentProvider(provider)
		require.NoError(t, err)

		// Second registration should fail.
		err = RegisterComponentProvider(provider)
		require.Error(t, err)
	})
}

func TestGetComponentProviderForEvent(t *testing.T) {
	ClearComponentProviders()

	provider := &mockComponentProvider{
		componentType: "test-terraform",
		bindings: []HookBinding{
			{Event: "after.test-terraform.plan", Actions: []HookAction{ActionSummary}},
			{Event: "after.test-terraform.apply", Actions: []HookAction{ActionSummary}},
		},
	}
	err := RegisterComponentProvider(provider)
	require.NoError(t, err)

	t.Run("event found", func(t *testing.T) {
		p := GetComponentProviderForEvent("after.test-terraform.plan")
		require.NotNil(t, p)
		assert.Equal(t, "test-terraform", p.GetType())
	})

	t.Run("event not found", func(t *testing.T) {
		p := GetComponentProviderForEvent("after.unknown.plan")
		assert.Nil(t, p)
	})
}

func TestListComponentProviders(t *testing.T) {
	ClearComponentProviders()

	// Register multiple providers.
	providers := []string{"alpha", "beta", "gamma"}
	for _, name := range providers {
		err := RegisterComponentProvider(&mockComponentProvider{componentType: name})
		require.NoError(t, err)
	}

	// List should return all providers sorted.
	list := ListComponentProviders()
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
