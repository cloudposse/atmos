package exec

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestMirrorTargetIncluded(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{}

	tests := []struct {
		name      string
		section   map[string]any
		query     string
		want      bool
		expectErr bool
	}{
		{
			name:    "abstract components are excluded",
			section: map[string]any{cfg.MetadataSectionName: map[string]any{"type": "abstract"}},
			want:    false,
		},
		{
			name:    "disabled components are excluded",
			section: map[string]any{cfg.MetadataSectionName: map[string]any{"enabled": false}},
			want:    false,
		},
		{
			name:    "concrete enabled component with no query is included",
			section: map[string]any{cfg.MetadataSectionName: map[string]any{"type": "real"}},
			want:    true,
		},
		{
			name:    "component without a metadata section is included",
			section: map[string]any{"vars": map[string]any{"enabled": true}},
			want:    true,
		},
		{
			name:    "matching query is included",
			section: map[string]any{"vars": map[string]any{"enabled": true}},
			query:   ".vars.enabled == true",
			want:    true,
		},
		{
			name:    "non-matching query is excluded",
			section: map[string]any{"vars": map[string]any{"enabled": false}},
			query:   ".vars.enabled == true",
			want:    false,
		},
		{
			name:    "query returning a non-bool is excluded",
			section: map[string]any{"vars": map[string]any{"region": "us-east-1"}},
			query:   ".vars.region",
			want:    false,
		},
		{
			name:      "invalid query surfaces an error",
			section:   map[string]any{"vars": map[string]any{}},
			query:     "..[invalid",
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := mirrorTargetIncluded(atmosConfig, "vpc", tt.section, tt.query)
			if tt.expectErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}
