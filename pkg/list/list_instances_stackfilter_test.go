package list

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
)

func TestMatchStackPattern(t *testing.T) {
	tests := []struct {
		name    string
		stack   string
		pattern string
		want    bool
	}{
		{"empty pattern matches anything", "tenant1-ue2-dev", "", true},
		{"empty pattern matches empty stack", "", "", true},
		{"exact name matches", "tenant1-ue2-dev", "tenant1-ue2-dev", true},
		{"exact name mismatches", "tenant1-ue2-dev", "tenant1-uw2-dev", false},
		{"glob prefix matches", "tenant1-ue2-dev", "tenant1-*", true},
		{"glob prefix no match", "tenant2-ue2-dev", "tenant1-*", false},
		{"glob suffix matches", "tenant1-ue2-dev", "*-dev", true},
		{"glob middle matches", "tenant1-ue2-dev", "*-ue2-*", true},
		{"question mark matches single char", "ue2", "ue?", true},
		{"question mark no match for longer", "ue22", "ue?", false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := matchStackPattern(tc.stack, tc.pattern)
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}

	t.Run("invalid pattern returns invalid flag error", func(t *testing.T) {
		_, err := matchStackPattern("tenant1-ue2-dev", "[invalid")
		assert.ErrorIs(t, err, errUtils.ErrInvalidFlag)
	})
}

func TestFilterStacksMapByPattern(t *testing.T) {
	stacks := map[string]any{
		"tenant1-ue2-dev":  map[string]any{"components": map[string]any{}},
		"tenant1-ue2-prod": map[string]any{"components": map[string]any{}},
		"tenant2-uw2-dev":  map[string]any{"components": map[string]any{}},
	}

	t.Run("empty pattern returns input unchanged", func(t *testing.T) {
		got, err := filterStacksMapByPattern(stacks, "")
		require.NoError(t, err)
		assert.Len(t, got, 3)
	})

	t.Run("exact name returns single entry", func(t *testing.T) {
		got, err := filterStacksMapByPattern(stacks, "tenant1-ue2-dev")
		require.NoError(t, err)
		assert.Len(t, got, 1)
		assert.Contains(t, got, "tenant1-ue2-dev")
	})

	t.Run("glob returns matching subset", func(t *testing.T) {
		got, err := filterStacksMapByPattern(stacks, "tenant1-*")
		require.NoError(t, err)
		assert.Len(t, got, 2)
		assert.Contains(t, got, "tenant1-ue2-dev")
		assert.Contains(t, got, "tenant1-ue2-prod")
		assert.NotContains(t, got, "tenant2-uw2-dev")
	})

	t.Run("non-matching pattern returns empty map", func(t *testing.T) {
		got, err := filterStacksMapByPattern(stacks, "nope-*")
		require.NoError(t, err)
		assert.Empty(t, got)
	})

	t.Run("invalid pattern returns invalid flag error", func(t *testing.T) {
		_, err := filterStacksMapByPattern(stacks, "[invalid")
		assert.ErrorIs(t, err, errUtils.ErrInvalidFlag)
	})

	t.Run("invalid pattern with empty map returns invalid flag error", func(t *testing.T) {
		_, err := filterStacksMapByPattern(map[string]any{}, "[invalid")
		assert.ErrorIs(t, err, errUtils.ErrInvalidFlag)
	})
}

// TestCollectInstances_StackPattern verifies that the stack filter is applied
// during instance collection, end-to-end.
func TestCollectInstances_StackPattern(t *testing.T) {
	stacks := map[string]any{
		"tenant1-ue2-dev": map[string]any{
			"components": map[string]any{
				"terraform": map[string]any{
					"vpc": map[string]any{},
				},
			},
		},
		"tenant2-uw2-dev": map[string]any{
			"components": map[string]any{
				"terraform": map[string]any{
					"vpc": map[string]any{},
					"eks": map[string]any{},
				},
			},
		},
	}

	t.Run("empty pattern returns all instances", func(t *testing.T) {
		got, err := collectInstances(stacks, "")
		require.NoError(t, err)
		assert.Len(t, got, 3)
	})

	t.Run("exact pattern filters to one stack", func(t *testing.T) {
		got, err := collectInstances(stacks, "tenant1-ue2-dev")
		require.NoError(t, err)
		assert.Len(t, got, 1)
		assert.Equal(t, "tenant1-ue2-dev", got[0].Stack)
		assert.Equal(t, "vpc", got[0].Component)
	})

	t.Run("glob pattern filters", func(t *testing.T) {
		got, err := collectInstances(stacks, "tenant2-*")
		require.NoError(t, err)
		assert.Len(t, got, 2)
		for _, inst := range got {
			assert.Equal(t, "tenant2-uw2-dev", inst.Stack)
		}
	})

	t.Run("invalid pattern returns invalid flag error", func(t *testing.T) {
		_, err := collectInstances(stacks, "[invalid")
		assert.ErrorIs(t, err, errUtils.ErrInvalidFlag)
	})

	t.Run("invalid pattern with empty map returns invalid flag error", func(t *testing.T) {
		_, err := collectInstances(map[string]any{}, "[invalid")
		assert.ErrorIs(t, err, errUtils.ErrInvalidFlag)
	})
}
