package terraform

import (
	"bytes"
	stdio "io"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

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

func initTerraformTestUI(t *testing.T) *bytes.Buffer {
	t.Helper()
	stderr := &bytes.Buffer{}
	streams := &terraformTestStreams{
		stdin:  &bytes.Buffer{},
		stdout: &bytes.Buffer{},
		stderr: stderr,
	}
	ioCtx, err := iolib.NewContext(iolib.WithStreams(streams))
	require.NoError(t, err)
	ui.InitFormatter(ioCtx)
	t.Cleanup(ui.Reset)
	return stderr
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

func TestWriteTerraformTestTextStylesSummary(t *testing.T) {
	tests := []struct {
		name         string
		text         string
		wantContains []string
	}{
		{
			name: "success",
			text: "  ✓ run \"ok\"... pass\nSuccess! 1 passed, 0 failed.\n",
			wantContains: []string{
				"  ✓ run \"ok\"... pass\n",
				"✓ Success! 1 passed, 0 failed.",
			},
		},
		{
			name: "failure",
			text: "  ✗ run \"broken\"... fail\nFailure! 0 passed, 1 failed.\n",
			wantContains: []string{
				"  ✗ run \"broken\"... fail\n",
				"✗ Failure! 0 passed, 1 failed.",
			},
		},
		{
			name: "unknown summary stays plain",
			text: "No tests ran\n",
			wantContains: []string{
				"No tests ran\n",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stderr := initTerraformTestUI(t)

			writeTerraformTestText(tt.text)

			got := stderr.String()
			for _, want := range tt.wantContains {
				assert.Contains(t, got, want)
			}
		})
	}
}
