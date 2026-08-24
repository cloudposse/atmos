package terraform

import (
	"bytes"
	stdio "io"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/ansi"
	iolib "github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/pkg/ui"
)

type terraformTestStreams struct {
	stdin  stdio.Reader
	stdout stdio.Writer
	stderr stdio.Writer
}

func (s *terraformTestStreams) Input() stdio.Reader     { return s.stdin }
func (s *terraformTestStreams) Output() stdio.Writer    { return s.stdout }
func (s *terraformTestStreams) Error() stdio.Writer     { return s.stderr }
func (s *terraformTestStreams) RawOutput() stdio.Writer { return s.stdout }
func (s *terraformTestStreams) RawError() stdio.Writer  { return s.stderr }

func initTerraformTestUI(t *testing.T) {
	t.Helper()
	streams := &terraformTestStreams{
		stdin:  &bytes.Buffer{},
		stdout: &bytes.Buffer{},
		stderr: &bytes.Buffer{},
	}
	ioCtx, err := iolib.NewContext(iolib.WithStreams(streams))
	require.NoError(t, err)
	ui.InitFormatter(ioCtx)
	t.Cleanup(ui.Reset)
}

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
			want: "✓ run \"ok\"... pass\n",
		},
		{
			name: "failure",
			line: "  run \"broken\"... fail\n",
			want: "✗ run \"broken\"... fail\n",
		},
		{
			name: "error",
			line: "tests/app.tftest.hcl... error\n",
			want: "✗ tests/app.tftest.hcl... error\n",
		},
		{
			name: "strips terraform status color",
			line: "tests/app.tftest.hcl... \x1b[32mpass\x1b[0m\n",
			want: "✓ tests/app.tftest.hcl... pass\n",
		},
		{
			name: "in progress unchanged",
			line: "tests/app.tftest.hcl... in progress\n",
			want: "▶ tests/app.tftest.hcl... in progress\n",
		},
		{
			name: "tearing down",
			line: "tests/app.tftest.hcl... tearing down\n",
			want: "▶ tests/app.tftest.hcl... tearing down\n",
		},
		{
			name: "non test line unchanged",
			line: "Success! 1 passed, 0 failed.\n",
			want: "Success! 1 passed, 0 failed.\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			initTerraformTestUI(t)
			assert.Equal(t, tt.want, ansi.Strip(formatTerraformTestStatusLine(tt.line)))
		})
	}
}

func TestTerraformTestStatusWriterBuffersPartialLines(t *testing.T) {
	initTerraformTestUI(t)
	var out bytes.Buffer
	w := newTerraformTestStatusWriter(&out)

	n, err := w.Write([]byte("  run \"ok\"... pa"))
	require.NoError(t, err)
	assert.Equal(t, len("  run \"ok\"... pa"), n)
	assert.Empty(t, out.String())

	n, err = w.Write([]byte("ss\n  run \"broken\"... fail\n"))
	require.NoError(t, err)
	assert.Equal(t, len("ss\n  run \"broken\"... fail\n"), n)
	assert.Equal(t, "✓ run \"ok\"... pass\n✗ run \"broken\"... fail\n", ansi.Strip(out.String()))
}

func TestTerraformTestStatusWriterFlushesPartialFinalLine(t *testing.T) {
	initTerraformTestUI(t)
	var out bytes.Buffer
	w := newTerraformTestStatusWriter(&out)

	n, err := w.Write([]byte("  run \"ok\"... pass"))
	require.NoError(t, err)
	assert.Equal(t, len("  run \"ok\"... pass"), n)
	assert.Empty(t, out.String())

	require.NoError(t, w.Flush())
	assert.Equal(t, "✓ run \"ok\"... pass", ansi.Strip(out.String()))
	assert.NoError(t, w.Flush(), "second flush should be a no-op")
}

func TestTerraformTestStatusWriterTeeKeepsRawOutputForHooks(t *testing.T) {
	initTerraformTestUI(t)
	var rawOut, terminalOut bytes.Buffer
	statusWriter := newTerraformTestStatusWriter(&terminalOut)
	w := stdio.MultiWriter(&rawOut, statusWriter)

	_, err := w.Write([]byte("  run \"ok\"... pass"))
	require.NoError(t, err)
	require.NoError(t, statusWriter.Flush())

	assert.Equal(t, "  run \"ok\"... pass", rawOut.String())
	assert.Equal(t, "✓ run \"ok\"... pass", ansi.Strip(terminalOut.String()))
}
