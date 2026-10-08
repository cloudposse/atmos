package markdown

import (
	"testing"

	"github.com/muesli/termenv"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStripFrontmatter(t *testing.T) {
	for _, tc := range []struct{ name, input, want string }{
		{"yaml", "---\ntitle: Example\ndescription: >-\n  Example description\ncast:\n  file: demo.cast\n---\n# Example\n{{ literal }}", "# Example\n{{ literal }}"},
		{"windows", "\ufeff---\r\ntitle: Example\r\n---\r\n# Example\r\n", "# Example\r\n"},
		{"yaml-end", "---\ntitle: Example\n...\nBody", "Body"},
		{"empty", "---\n---\nBody", "Body"},
		{"metadata-only", "---\ntitle: Example\n---", ""},
		{"plain", "# Example\nBody", "# Example\nBody"},
		{"horizontal-rule", "---\nSome prose\n---\nBody", "---\nSome prose\n---\nBody"},
		{"malformed", "---\ntitle: [\n---\nBody", "---\ntitle: [\n---\nBody"},
		{"unclosed", "---\ntitle: Example\nBody", "---\ntitle: Example\nBody"},
		{"code", "```yaml\n---\ntitle: Example\n---\n```", "```yaml\n---\ntitle: Example\n---\n```"},
	} {
		t.Run(tc.name, func(t *testing.T) { assert.Equal(t, tc.want, StripFrontmatter(tc.input)) })
	}
}

func TestFrontmatterIsNotRenderedAsProse(t *testing.T) {
	renderer, err := NewCustomRenderer(WithColorProfile(termenv.Ascii))
	require.NoError(t, err)
	defer renderer.Close()
	output, err := renderer.Render("---\ntitle: Metadata title\ncast:\n  file: demo.cast\n---\n# Visible title\n\nLiteral {{ .Example }}.")
	require.NoError(t, err)
	assert.Contains(t, output, "Visible title")
	assert.Contains(t, output, "{{ .Example }}")
	assert.NotContains(t, output, "Metadata title")
	assert.NotContains(t, output, "demo.cast")
}
