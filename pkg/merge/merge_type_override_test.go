package merge

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestDeepMergeNative_TypeOverride verifies that src can override dst regardless
// of type differences.  This is the WithOverride contract: src always wins at
// leaf level.  PR #2201 introduced guards that rejected some of these overrides,
// breaking real-world configs where users override lists with {}, scalars, or null.
func TestDeepMergeNative_TypeOverride(t *testing.T) {
	tests := []struct {
		name    string
		dst     map[string]any
		src     map[string]any
		wantKey string
		wantVal any
	}{
		{
			name:    "list overridden by empty map",
			dst:     map[string]any{"accounts": []any{"a", "b"}},
			src:     map[string]any{"accounts": map[string]any{}},
			wantKey: "accounts",
			wantVal: map[string]any{},
		},
		{
			name:    "list overridden by non-empty map",
			dst:     map[string]any{"accounts": []any{"a", "b"}},
			src:     map[string]any{"accounts": map[string]any{"only": "one"}},
			wantKey: "accounts",
			wantVal: map[string]any{"only": "one"},
		},
		{
			name:    "list overridden by scalar string",
			dst:     map[string]any{"cidrs": []any{"10.0.0.0/16"}},
			src:     map[string]any{"cidrs": "10.99.0.0/16"},
			wantKey: "cidrs",
			wantVal: "10.99.0.0/16",
		},
		{
			name:    "list overridden by scalar int",
			dst:     map[string]any{"ports": []any{80, 443}},
			src:     map[string]any{"ports": 8080},
			wantKey: "ports",
			wantVal: 8080,
		},
		{
			name:    "list overridden by nil",
			dst:     map[string]any{"rules": []any{"allow"}},
			src:     map[string]any{"rules": nil},
			wantKey: "rules",
			wantVal: nil,
		},
		{
			name:    "list overridden by bool false",
			dst:     map[string]any{"features": []any{"a", "b"}},
			src:     map[string]any{"features": false},
			wantKey: "features",
			wantVal: false,
		},
		{
			name:    "list of maps overridden by empty map",
			dst:     map[string]any{"ingress": []any{map[string]any{"tenant": "core"}}},
			src:     map[string]any{"ingress": map[string]any{}},
			wantKey: "ingress",
			wantVal: map[string]any{},
		},
		{
			name:    "nested: list inside map overridden by empty map",
			dst:     map[string]any{"vars": map[string]any{"accounts": []any{"a"}}},
			src:     map[string]any{"vars": map[string]any{"accounts": map[string]any{}}},
			wantKey: "vars",
			// After deep merge, vars.accounts should be the empty map from src.
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := deepMergeNative(tt.dst, tt.src, false, false)
			require.NoError(t, err, "type override must not error")

			if tt.wantKey == "vars" {
				// Nested case: check vars.accounts.
				vars, ok := tt.dst["vars"].(map[string]any)
				require.True(t, ok)
				assert.Equal(t, map[string]any{}, vars["accounts"])
			} else {
				assert.Equal(t, tt.wantVal, tt.dst[tt.wantKey])
			}
		})
	}
}

// TestDeepMergeNative_TypeOverride_WithSliceFlags verifies type overrides work
// even when appendSlice or sliceDeepCopy flags are set.
func TestDeepMergeNative_TypeOverride_WithSliceFlags(t *testing.T) {
	tests := []struct {
		name          string
		appendSlice   bool
		sliceDeepCopy bool
	}{
		{"appendSlice", true, false},
		{"sliceDeepCopy", false, true},
		{"both flags", true, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dst := map[string]any{"accounts": []any{"a", "b"}}
			src := map[string]any{"accounts": map[string]any{}}

			err := deepMergeNative(dst, src, tt.appendSlice, tt.sliceDeepCopy)
			require.NoError(t, err, "type override must work regardless of slice flags")
			assert.Equal(t, map[string]any{}, dst["accounts"])
		})
	}
}
