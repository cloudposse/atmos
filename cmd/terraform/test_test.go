package terraform

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAppendJSONFlag(t *testing.T) {
	t.Run("appends when missing", func(t *testing.T) {
		got := appendJSONFlag([]string{"-run=smoke"})
		assert.Equal(t, []string{"-run=smoke", "-json"}, got)
	})

	t.Run("preserves existing short flag", func(t *testing.T) {
		in := []string{"-json", "-run=smoke"}
		got := appendJSONFlag(in)
		require.Same(t, &in[0], &got[0], "existing -json should return the original slice")
		assert.Equal(t, in, got)
	})

	t.Run("preserves existing long flag", func(t *testing.T) {
		in := []string{"--json"}
		got := appendJSONFlag(in)
		require.Same(t, &in[0], &got[0], "existing --json should return the original slice")
		assert.Equal(t, in, got)
	})

	t.Run("handles nil", func(t *testing.T) {
		assert.Equal(t, []string{"-json"}, appendJSONFlag(nil))
	})
}
