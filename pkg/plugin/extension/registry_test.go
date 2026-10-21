package extension

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
)

func TestThirdDescriptorRequiresNoLifecycleChanges(t *testing.T) {
	t.Parallel()
	registry := New()
	activated := ""
	require.NoError(t, registry.Register(Descriptor{
		Capability: "test.example", Path: []string{"test", "examples"}, Version: 1, Scaffold: "test-template",
		Validate: func(binding Binding) error {
			if binding.Metadata["value"] != "original" {
				return errUtils.ErrPluginInvalidConfig
			}
			return nil
		},
		Activate: func(_ context.Context, binding *Activation) error { activated = binding.Name; return nil },
	}))
	inputs := map[string]any{"test": map[string]any{"examples": map[string]any{"third": map[string]any{"value": "original", "items": []any{"first", "last"}}}}}
	bindings, err := registry.Validate(t.TempDir(), inputs)
	require.NoError(t, err)
	require.Len(t, bindings, 1)
	assert.Equal(t, "test.example", bindings[0].Capability)
	assert.Equal(t, "third", bindings[0].Name)
	assert.Empty(t, activated, "static validation cannot execute adapters")
	descriptor, found := registry.Get(bindings[0].Capability)
	require.True(t, found)
	require.NoError(t, descriptor.Activate(t.Context(), &Activation{Binding: bindings[0]}))
	assert.Equal(t, "third", activated)
	// Isolate metadata in both directions, including slices nested inside maps.
	original := inputs["test"].(map[string]any)["examples"].(map[string]any)["third"].(map[string]any)
	bindings[0].Metadata["items"].([]any)[0] = "changed"
	assert.Equal(t, []any{"first", "last"}, original["items"])
	original["value"] = "source changed"
	assert.Equal(t, "original", bindings[0].Metadata["value"])
	descriptor.Path[0] = "changed"
	again, found := registry.Get("test.example")
	require.True(t, found)
	assert.Equal(t, []string{"test", "examples"}, again.Path)
}

func TestDescriptorDuplicateAndUnsupportedPackage(t *testing.T) {
	t.Parallel()
	registry := New()
	descriptor := Descriptor{Capability: "test.example", Path: []string{"test", "examples"}, Version: 1, Validate: func(Binding) error { return nil }}
	require.NoError(t, registry.Register(descriptor))
	require.ErrorIs(t, registry.Register(descriptor), errUtils.ErrDuplicatePluginExtension)
	descriptor.Capability = "test.other"
	descriptor.Path = []string{"test"}
	require.ErrorIs(t, registry.Register(descriptor), errUtils.ErrDuplicatePluginExtension)
	descriptor.Path = nil
	require.ErrorIs(t, registry.Register(descriptor), errUtils.ErrPluginInvalidConfig)
	mixed := map[string]any{"test": map[string]any{"examples": map[string]any{"known": map[string]any{}}}, "future": map[string]any{"unknown": map[string]any{"required": true}}}
	bindings, err := registry.Validate(t.TempDir(), mixed)
	require.ErrorIs(t, err, errUtils.ErrPluginUnsupportedExtension)
	assert.Nil(t, bindings, "unsupported export rejects the entire package")
}

func TestDuplicateRegistrationNamesTheConflictingCapabilityDeterministically(t *testing.T) {
	t.Parallel()
	registry := New()
	for _, name := range []string{"zeta", "alpha", "mike"} {
		require.NoError(t, registry.Register(Descriptor{Capability: name + ".cap", Path: []string{name}, Version: 1, Validate: func(Binding) error { return nil }}))
	}
	// The new descriptor claims the metadata path already owned by alpha.cap.
	conflict := Descriptor{Capability: "new.cap", Path: []string{"alpha"}, Version: 1, Validate: func(Binding) error { return nil }}
	first := registry.Register(conflict)
	require.ErrorIs(t, first, errUtils.ErrDuplicatePluginExtension)
	assert.Contains(t, first.Error(), "new.cap conflicts with alpha.cap")
	for range 25 {
		require.EqualError(t, registry.Register(conflict), first.Error())
	}
}
