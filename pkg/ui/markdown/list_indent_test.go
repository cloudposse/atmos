package markdown

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestFixListHangingIndent covers the pure line-transform in isolation, using
// already-"glamour-shaped" plain text (no ANSI) so expectations are exact and
// independent of the actual glamour renderer's word-wrap decisions.
func TestFixListHangingIndent(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "bullet continuation aligns under item text",
			in: "  • Add your first workload component\n" +
				"  under components/terraform/ and wire it\n" +
				"  into the stage stacks.",
			want: "  • Add your first workload component\n" +
				"    under components/terraform/ and wire it\n" +
				"    into the stage stacks.",
		},
		{
			name: "two-digit ordinal continuation aligns under item text",
			in: "  12. In stacks/_defaults.yaml, delete the\n" +
				"  emulator component and everything",
			want: "  12. In stacks/_defaults.yaml, delete the\n" +
				"      emulator component and everything",
		},
		{
			name: "single-digit ordinal continuation aligns under item text",
			in: "  1. In stacks/_defaults.yaml, delete the\n" +
				"  emulator component and everything",
			want: "  1. In stacks/_defaults.yaml, delete the\n" +
				"     emulator component and everything",
		},
		{
			name: "blank line ends continuation tracking",
			in: "  • Item one wraps here\n" +
				"  continuation of item one\n" +
				"\n" +
				"  not part of any list",
			want: "  • Item one wraps here\n" +
				"    continuation of item one\n" +
				"\n" +
				"  not part of any list",
		},
		{
			name: "dedent ends continuation tracking",
			in: "    • Nested item wraps\n" +
				"    continuation nested\n" +
				"  outer text after dedent",
			want: "    • Nested item wraps\n" +
				"      continuation nested\n" +
				"  outer text after dedent",
		},
		{
			name: "nested ordered list tracked independently of its parent bullet",
			in: "  • Outer item\n" +
				"    1. Inner ordered item wraps here\n" +
				"    inner continuation",
			want: "  • Outer item\n" +
				"    1. Inner ordered item wraps here\n" +
				"       inner continuation",
		},
		{
			name: "second item's bullet line is not treated as a continuation",
			in: "  • First item wraps\n" +
				"  first continuation\n" +
				"  • Second item short",
			want: "  • First item wraps\n" +
				"    first continuation\n" +
				"  • Second item short",
		},
		{
			name: "outer item's wrapped text resumes correctly after a nested list ends",
			in: "  • Outer item wraps across\n" +
				"    1. Inner item\n" +
				"  outer continuation resumes here",
			want: "  • Outer item wraps across\n" +
				"    1. Inner item\n" +
				"    outer continuation resumes here",
		},
		{
			name: "no list content is left untouched",
			in:   "Just a paragraph\nwith two lines.",
			want: "Just a paragraph\nwith two lines.",
		},
		{
			name: "empty string is returned unchanged",
			in:   "",
			want: "",
		},
		{
			name: "single line without a newline is returned unchanged",
			in:   "• single line, nothing to wrap",
			want: "• single line, nothing to wrap",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FixListHangingIndent(tt.in)
			assert.Equal(t, tt.want, got)
		})
	}
}

// TestFixListHangingIndent_Integration renders real markdown through the full
// CustomRenderer pipeline at a narrow width that forces a wrap, then verifies
// -- after stripping ANSI codes -- that each continuation line's leading
// whitespace is exactly the bullet/number line's indent plus the width of its
// own prefix ("• " or "<n>. ").
func TestFixListHangingIndent_Integration(t *testing.T) {
	tests := []struct {
		name       string
		markdown   string
		prefixWant string // Expected literal prefix on the first content line (after indent).
	}{
		{
			name:       "bullet list wraps with hanging indent",
			markdown:   "- Add your first workload component under components/terraform/ and wire it into the stage stacks.",
			prefixWant: "• ",
		},
		{
			name: "ordered list wraps with hanging indent",
			markdown: "1. In stacks/_defaults.yaml, delete the emulator component and everything from " +
				"access_key down in the backend block; pick a globally unique state bucket name.",
			prefixWant: "1. ",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			renderer, err := NewCustomRenderer(WithWordWrap(40), WithColorProfile(termenv.TrueColor))
			require.NoError(t, err)

			out, err := renderer.Render(tt.markdown)
			require.NoError(t, err)

			lines := nonBlankLines(ansi.Strip(out))
			require.GreaterOrEqual(t, len(lines), 2, "expected the text to wrap onto at least two lines, got: %q", lines)

			firstIndent := leadingSpaces(lines[0])
			require.True(t, strings.HasPrefix(lines[0][firstIndent:], tt.prefixWant),
				"first line %q should start (after indent) with %q", lines[0], tt.prefixWant)

			wantContinuationIndent := firstIndent + utf8.RuneCountInString(tt.prefixWant)
			for _, line := range lines[1:] {
				assert.Equal(t, wantContinuationIndent, leadingSpaces(line),
					"continuation line %q should be indented under the item's own text", line)
			}
		})
	}
}

// nonBlankLines splits s into lines, dropping any that are empty once
// trimmed (glamour surrounds documents/lists with blank margin lines that
// aren't relevant to indent alignment).
func nonBlankLines(s string) []string {
	var out []string
	for _, line := range strings.Split(s, "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		out = append(out, line)
	}
	return out
}

// leadingSpaces returns the number of leading space characters in s.
func leadingSpaces(s string) int {
	return len(s) - len(strings.TrimLeft(s, " "))
}
