package exec

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestNormalizeComponentSourceSection verifies the stack processor accepts both the documented
// string "simple form" of a component `source` (normalized to {uri: <string>}) and the map form,
// and rejects other types — matching the JIT source provisioner.
func TestNormalizeComponentSourceSection(t *testing.T) {
	tests := []struct {
		name   string
		raw    any
		want   map[string]any
		wantOK bool
	}{
		{
			name:   "string simple form is normalized to {uri}",
			raw:    "github.com/cloudposse-sandbox/atmos-pro-qa-2-xl//components/terraform/queue",
			want:   map[string]any{"uri": "github.com/cloudposse-sandbox/atmos-pro-qa-2-xl//components/terraform/queue"},
			wantOK: true,
		},
		{
			name:   "map form passes through unchanged",
			raw:    map[string]any{"uri": "github.com/org/repo//mod", "version": "1.2.3"},
			want:   map[string]any{"uri": "github.com/org/repo//mod", "version": "1.2.3"},
			wantOK: true,
		},
		{
			name:   "empty string is treated as no source",
			raw:    "",
			want:   map[string]any{},
			wantOK: true,
		},
		{
			name:   "invalid type is rejected",
			raw:    []any{"not", "valid"},
			want:   nil,
			wantOK: false,
		},
		{
			name:   "integer is rejected",
			raw:    42,
			want:   nil,
			wantOK: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := normalizeComponentSourceSection(tt.raw)
			assert.Equal(t, tt.wantOK, ok)
			assert.Equal(t, tt.want, got)
		})
	}
}
