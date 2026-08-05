package pro

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestResolveSection(t *testing.T) {
	t.Run("top-level pro wins when both present", func(t *testing.T) {
		proSection := map[string]any{"enabled": true}
		settingsSection := map[string]any{"pro": map[string]any{"enabled": false}}
		got := ResolveSection(proSection, settingsSection)
		assert.Equal(t, map[string]any{"enabled": true}, got)
	})

	t.Run("falls back to legacy settings.pro when top-level absent", func(t *testing.T) {
		settingsSection := map[string]any{"pro": map[string]any{"enabled": true}}
		got := ResolveSection(nil, settingsSection)
		assert.Equal(t, map[string]any{"enabled": true}, got)
	})

	t.Run("nil when neither present", func(t *testing.T) {
		assert.Nil(t, ResolveSection(nil, nil))
		assert.Nil(t, ResolveSection(map[string]any{}, map[string]any{}))
	})

	t.Run("YAML-shaped legacy pro block is normalized", func(t *testing.T) {
		settingsSection := map[string]any{"pro": map[interface{}]interface{}{"enabled": true}}
		got := ResolveSection(nil, settingsSection)
		assert.Equal(t, map[string]any{"enabled": true}, got)
	})

	t.Run("non-map legacy pro is treated as absent", func(t *testing.T) {
		settingsSection := map[string]any{"pro": "invalid"}
		assert.Nil(t, ResolveSection(nil, settingsSection))
	})
}

func TestEffectiveEnabledState(t *testing.T) {
	tests := []struct {
		name           string
		pro            map[string]any
		metadata       map[string]any
		wantEnabled    bool
		wantDriftState bool
	}{
		{name: "nil pro is disabled", pro: nil, metadata: nil, wantEnabled: false, wantDriftState: false},
		{name: "pro enabled defaults true", pro: map[string]any{}, metadata: nil, wantEnabled: true, wantDriftState: false},
		{name: "explicit disabled", pro: map[string]any{"enabled": false}, metadata: nil, wantEnabled: false, wantDriftState: false},
		{
			name:           "enabled with drift on",
			pro:            map[string]any{"enabled": true, "drift_detection": map[string]any{"enabled": true}},
			metadata:       nil,
			wantEnabled:    true,
			wantDriftState: true,
		},
		{
			name:           "metadata.enabled false overrides pro.enabled true",
			pro:            map[string]any{"enabled": true, "drift_detection": map[string]any{"enabled": true}},
			metadata:       map[string]any{"enabled": false},
			wantEnabled:    false,
			wantDriftState: false,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			enabled, drift := EffectiveEnabledState(tc.pro, tc.metadata)
			assert.Equal(t, tc.wantEnabled, enabled)
			assert.Equal(t, tc.wantDriftState, drift)
		})
	}
}

func TestMetadataDisabledPro(t *testing.T) {
	t.Run("nil pro is never a metadata squash", func(t *testing.T) {
		assert.False(t, MetadataDisabledPro(nil, map[string]any{"enabled": false}))
	})

	t.Run("metadata false squashes an otherwise-enabled pro", func(t *testing.T) {
		assert.True(t, MetadataDisabledPro(map[string]any{"enabled": true}, map[string]any{"enabled": false}))
	})

	t.Run("pro already disabled is not a metadata squash", func(t *testing.T) {
		assert.False(t, MetadataDisabledPro(map[string]any{"enabled": false}, map[string]any{"enabled": false}))
	})
}

func TestSanitizeForJSON(t *testing.T) {
	t.Run("converts nested map[interface{}]interface{}", func(t *testing.T) {
		in := map[interface{}]interface{}{
			"a": map[interface{}]interface{}{"b": 1},
		}
		got := SanitizeForJSON(in)
		assert.Equal(t, map[string]interface{}{"a": map[string]interface{}{"b": 1}}, got)
	})

	t.Run("converts slices recursively", func(t *testing.T) {
		in := []interface{}{map[interface{}]interface{}{"a": 1}}
		got := SanitizeForJSON(in)
		assert.Equal(t, []interface{}{map[string]interface{}{"a": 1}}, got)
	})

	t.Run("scalars pass through unchanged", func(t *testing.T) {
		assert.Equal(t, "x", SanitizeForJSON("x"))
		assert.Equal(t, 5, SanitizeForJSON(5))
	})
}
