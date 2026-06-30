package terraform

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/ansi"
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

func TestFormatTerraformTestStatusLine(t *testing.T) {
	tests := []struct {
		name string
		line string
		want string
	}{
		{
			name: "success",
			line: "  run \"ok\"... pass\n",
			want: "  run \"ok\"... ✓ pass\n",
		},
		{
			name: "failure",
			line: "  run \"broken\"... fail\n",
			want: "  run \"broken\"... ✗ fail\n",
		},
		{
			name: "error",
			line: "tests/app.tftest.hcl... error\n",
			want: "tests/app.tftest.hcl... ✗ error\n",
		},
		{
			name: "strips terraform status color",
			line: "tests/app.tftest.hcl... \x1b[32mpass\x1b[0m\n",
			want: "tests/app.tftest.hcl... ✓ pass\n",
		},
		{
			name: "in progress unchanged",
			line: "tests/app.tftest.hcl... in progress\n",
			want: "tests/app.tftest.hcl... in progress\n",
		},
		{
			name: "non test line unchanged",
			line: "Success! 1 passed, 0 failed.\n",
			want: "Success! 1 passed, 0 failed.\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, ansi.Strip(formatTerraformTestStatusLine(tt.line)))
		})
	}
}

func TestTerraformTestStatusWriterBuffersPartialLines(t *testing.T) {
	var out bytes.Buffer
	w := newTerraformTestStatusWriter(&out)

	n, err := w.Write([]byte("  run \"ok\"... pa"))
	require.NoError(t, err)
	assert.Equal(t, len("  run \"ok\"... pa"), n)
	assert.Empty(t, out.String())

	n, err = w.Write([]byte("ss\n  run \"broken\"... fail\n"))
	require.NoError(t, err)
	assert.Equal(t, len("ss\n  run \"broken\"... fail\n"), n)
	assert.Equal(t, "  run \"ok\"... ✓ pass\n  run \"broken\"... ✗ fail\n", ansi.Strip(out.String()))
}
