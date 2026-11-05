package markdown

import (
	"strings"
	"testing"

	"github.com/muesli/termenv"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCustomRendererASCIIListMarkers(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "ordered", input: "1. numbered with dot", want: "1. numbered with dot"},
		{name: "indented ordered", input: "  1. indented numbered", want: "1. indented numbered"},
		{name: "nondefault start", input: "2) paren style", want: "2. paren style"},
		{name: "dash bullet", input: "- dash bullet", want: "• dash bullet"},
		{name: "star bullet", input: "* star bullet", want: "• star bullet"},
		{name: "midline", input: "Step 1. mid-line 1. not a list", want: "Step 1. mid-line 1. not a list"},
		{name: "inline markdown", input: "1. **Create** a `token`", want: "1. Create a token"},
		{name: "heading", input: "# heading", want: "heading"},
		{name: "subheading", input: "## heading", want: "heading"},
		{name: "quote", input: "> quote", want: "quote"},
		{name: "escaped number", input: `1\. literal`, want: "1. literal"},
		{name: "escaped bullet", input: `\* literal`, want: "* literal"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			renderer, err := NewCustomRenderer(WithColorProfile(termenv.Ascii), WithWordWrap(0))
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, renderer.Close()) })
			rendered, err := renderer.Render(tt.input)
			require.NoError(t, err)
			assert.NotContains(t, rendered, "\x1b")
			assert.Equal(t, tt.want, strings.TrimSpace(rendered))
		})
	}
}

func TestCustomRendererASCIINestedWrappedLists(t *testing.T) {
	renderer, err := NewCustomRenderer(WithColorProfile(termenv.Ascii), WithWordWrap(30))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, renderer.Close()) })
	rendered, err := renderer.Render("- outer\n  - inner item has enough words to wrap onto another line\n- sibling")
	require.NoError(t, err)
	assert.NotContains(t, rendered, "\x1b")

	var lines []string
	for _, line := range strings.Split(rendered, "\n") {
		if strings.TrimSpace(line) != "" {
			lines = append(lines, strings.TrimRight(line, " "))
		}
	}
	require.GreaterOrEqual(t, len(lines), 4, "nested item should wrap: %q", rendered)
	assert.Equal(t, "• outer", strings.TrimSpace(lines[0]))
	assert.True(t, strings.HasPrefix(strings.TrimSpace(lines[1]), "• inner item"), "%q", rendered)
	assert.Equal(t, "• sibling", strings.TrimSpace(lines[len(lines)-1]))
	indent := func(line string) int { return len(line) - len(strings.TrimLeft(line, " ")) }
	assert.Greater(t, indent(lines[1]), indent(lines[0]), "nested list must remain indented")
	assert.Equal(t, indent(lines[1])+2, indent(lines[2]), "wrapped text must align after the nested bullet")
}
