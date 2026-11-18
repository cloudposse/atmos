package config

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
)

// billionLaughs builds the classic exponential alias graph: every level is a sequence of
// `fanout` aliases of the previous one, so level n expands to fanout^n leaves.
func billionLaughs(levels, fanout int) string {
	var b strings.Builder
	b.WriteString("l0: &l0 [" + strings.TrimSuffix(strings.Repeat("x, ", fanout), ", ") + "]\n")
	for level := 1; level <= levels; level++ {
		aliases := strings.TrimSuffix(strings.Repeat(fmt.Sprintf("*l%d, ", level-1), fanout), ", ")
		fmt.Fprintf(&b, "l%d: &l%d [%s]\n", level, level, aliases)
	}
	return b.String()
}

func TestDecodeNodeWithYamlFunctions_AliasExpansionIsBounded(t *testing.T) {
	t.Run("an exponential alias graph is rejected before it is expanded", func(t *testing.T) {
		start := time.Now()
		_, err := decodeYAMLForTest(t, billionLaughs(10, 10))
		require.ErrorIs(t, err, errUtils.ErrYamlAliasExpansionLimit)
		require.ErrorIs(t, err, ErrExecuteYamlFunctions)
		assert.Contains(t, err.Error(), "l", "the error names the alias that crossed the limit")
		assert.Less(t, time.Since(start), 10*time.Second, "the limit stops the expansion instead of allocating it")
	})

	t.Run("a large but bounded document still loads", func(t *testing.T) {
		var b strings.Builder
		b.WriteString("base: &base\n")
		for i := 0; i < 1000; i++ {
			fmt.Fprintf(&b, "  - {name: item%d, enabled: true, tags: [a, b]}\n", i)
		}
		for i := 0; i < 50; i++ {
			fmt.Fprintf(&b, "copy%d: *base\n", i)
		}
		got, err := decodeYAMLForTest(t, b.String())
		require.NoError(t, err)
		decoded, ok := got.(map[string]any)
		require.True(t, ok)
		require.Len(t, decoded, 51)
		last, ok := decoded["copy49"].([]any)
		require.True(t, ok)
		require.Len(t, last, 1000)
		assert.Equal(t, map[string]any{"name": "item0", "enabled": true, "tags": []any{"a", "b"}}, last[0])
		assert.Equal(t, map[string]any{"name": "item999", "enabled": true, "tags": []any{"a", "b"}}, last[999])
	})

	t.Run("the budget is checked against the configured limit exactly", func(t *testing.T) {
		// The anchor is a sequence of three scalars: four nodes per alias.
		source := "a: &a [1, 2, 3]\nb: *a\nc: *a\nd: *a\n"
		tests := []struct {
			name    string
			limit   int64
			wantErr bool
		}{
			{name: "one below the third alias", limit: 11, wantErr: true},
			{name: "exactly the three aliases", limit: 12, wantErr: false},
			{name: "well above", limit: 1000, wantErr: false},
			{name: "below the first alias", limit: 3, wantErr: true},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				document := mustParseYAML(t, source)
				decoder := configYamlDecoder{includes: map[string]bool{}, aliasLimit: tt.limit}
				got, err := decoder.decode(&document, filepath.Join(t.TempDir(), "atmos.yaml"))
				if tt.wantErr {
					require.ErrorIs(t, err, errUtils.ErrYamlAliasExpansionLimit)
					assert.Nil(t, got)
					return
				}
				require.NoError(t, err)
				assert.Equal(t, map[string]any{"a": []any{1, 2, 3}, "b": []any{1, 2, 3}, "c": []any{1, 2, 3}, "d": []any{1, 2, 3}}, got)
			})
		}
	})

	t.Run("merge keys are charged like aliases", func(t *testing.T) {
		document := mustParseYAML(t, "base: &base {a: 1, b: 2}\nx:\n  <<: *base\ny:\n  <<: *base\n")
		decoder := configYamlDecoder{includes: map[string]bool{}, aliasLimit: 5}
		_, err := decoder.decode(&document, filepath.Join(t.TempDir(), "atmos.yaml"))
		require.ErrorIs(t, err, errUtils.ErrYamlAliasExpansionLimit)
	})
}
