package ci

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

// templateReporter returns a reporter whose templates base path holds the given files.
func templateReporter(t *testing.T, summary string, files map[string]string) *reporter {
	t.Helper()
	dir := t.TempDir()
	for name, content := range files {
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600))
	}
	cfg := &schema.AtmosConfiguration{}
	cfg.CI.Templates.BasePath = dir
	cfg.CI.Summary.Template = summary
	return &reporter{cfg: cfg}
}

func TestReporterRenderTemplates(t *testing.T) {
	files := map[string]string{
		"summary-default.md": "default summary {{ .N }}",
		"summary-other.md":   "other summary {{ .N }}",
		"comment-other.md":   "other comment {{ .N }}",
	}
	data := map[string]any{"N": 3}

	tests := []struct {
		name     string
		summary  string
		call     string
		template string
		want     string
		wantErr  error
	}{
		{name: "summary explicit name", call: "summary", template: "summary-other.md", want: "other summary 3"},
		{name: "summary explicit name without extension", call: "summary", template: "summary-other", want: "other summary 3"},
		{name: "summary explicit name wins over the native-plugin default", summary: "summary-default.md", call: "summary", template: "summary-other.md", want: "other summary 3"},
		{name: "summary empty name ignores ci.summary.template", summary: "summary-default.md", call: "summary", wantErr: errUtils.ErrCITemplateNotFound},
		{name: "summary with neither", call: "summary", wantErr: errUtils.ErrCITemplateNotFound},
		{name: "summary explicit name missing on disk", call: "summary", template: "absent.md", wantErr: errUtils.ErrCITemplateNotFound},
		{name: "comment explicit name", call: "comment", template: "comment-other.md", want: "other comment 3"},
		{name: "comment ignores the summary default", summary: "summary-default.md", call: "comment", wantErr: errUtils.ErrCITemplateNotFound},
		{name: "comment with neither", call: "comment", wantErr: errUtils.ErrCITemplateNotFound},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := templateReporter(t, tt.summary, files)
			var (
				got string
				err error
			)
			if tt.call == "summary" {
				got, err = r.RenderSummary(tt.template, data)
			} else {
				got, err = r.RenderComment(tt.template, data)
			}
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
				assert.Empty(t, got)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestReporterRenderTemplates_MissingKeyFails(t *testing.T) {
	r := templateReporter(t, "", map[string]string{"typo.md": "{{ .Stak }}"})
	_, err := r.RenderSummary("typo.md", map[string]any{"Stack": "dev"})
	require.ErrorIs(t, err, errUtils.ErrTemplateEvaluation)
	_, err = r.RenderComment("typo.md", map[string]any{"Stack": "dev"})
	require.ErrorIs(t, err, errUtils.ErrTemplateEvaluation)
}

func TestReporterRenderTemplates_NilConfig(t *testing.T) {
	r := &reporter{}
	_, err := r.RenderSummary("", nil)
	require.ErrorIs(t, err, errUtils.ErrCITemplateNotFound)
	_, err = r.RenderComment("", nil)
	require.ErrorIs(t, err, errUtils.ErrCITemplateNotFound)
}
