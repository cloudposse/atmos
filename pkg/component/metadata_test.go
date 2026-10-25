package component

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestProcessMetadata(t *testing.T) {
	for _, tc := range []struct {
		name                      string
		section                   map[string]any
		base                      string
		abstract, enabled, locked bool
	}{
		{name: "defaults", section: map[string]any{}, enabled: true},
		{name: "component alias", section: map[string]any{"component": "physical/app"}, base: "physical/app", enabled: true},
		{name: "metadata takes precedence", section: map[string]any{"component": "ignored", "metadata": map[string]any{"component": "physical/app", "type": "abstract", "enabled": false, "locked": true}}, base: "physical/app", abstract: true, locked: true},
		{name: "self reference is not inheritance", section: map[string]any{"component": "app"}, enabled: true},
		{name: "metadata self reference overrides alias", section: map[string]any{"component": "ignored", "metadata": map[string]any{"component": "app"}}, enabled: true},
		{name: "nonboolean flags retain defaults", section: map[string]any{"metadata": map[string]any{"enabled": "false", "locked": "true", "type": "real"}}, enabled: true},
		{name: "explicit defaults", section: map[string]any{"metadata": map[string]any{"enabled": true, "locked": false}}, enabled: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result := ProcessMetadata("app", tc.section)
			wantMetadata, _ := tc.section["metadata"].(map[string]any)
			require.Equal(t, wantMetadata, result.Fields)
			require.Equal(t, tc.base, result.BaseComponent)
			require.Equal(t, tc.abstract, result.Abstract)
			require.Equal(t, tc.enabled, result.Enabled)
			require.Equal(t, tc.locked, result.Locked)
		})
	}
}
