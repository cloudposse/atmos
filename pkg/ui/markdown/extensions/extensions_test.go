package extensions

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
)

// inlineParserTestable defines the interface for testable inline parsers.
type inlineParserTestable interface {
	parser.InlineParser
}

// parserTestCase defines test inputs for inline parser tests.
type parserTestCase struct {
	name          string
	input         string
	expectNil     bool
	checkNodeType func(ast.Node) bool // optional type check
}

// runInlineParserTests runs common parser tests for any inline parser.
func runInlineParserTests(t *testing.T, p inlineParserTestable, trigger byte, cases []parserTestCase) {
	t.Helper()

	t.Run("Trigger returns expected byte", func(t *testing.T) {
		assert.Equal(t, []byte{trigger}, p.Trigger())
	})

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			reader := text.NewReader([]byte(tc.input))
			result := p.Parse(nil, reader, nil)
			if tc.expectNil {
				assert.Nil(t, result)
			} else {
				require.NotNil(t, result)
				if tc.checkNodeType != nil {
					assert.True(t, tc.checkNodeType(result))
				}
			}
		})
	}
}

func TestHighlightExtension(t *testing.T) {
	md := goldmark.New(
		goldmark.WithExtensions(NewHighlightExtension()),
	)

	tests := []struct {
		name        string
		input       string
		mustContain string
	}{
		{
			name:        "parses highlight syntax",
			input:       "This is ==highlighted== text",
			mustContain: "highlighted",
		},
		{
			name:        "parses multiple highlights",
			input:       "==one== and ==two==",
			mustContain: "one",
		},
		{
			name:        "ignores single equals",
			input:       "a = b",
			mustContain: "a = b",
		},
		{
			name:        "handles unclosed highlight",
			input:       "==unclosed",
			mustContain: "==unclosed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			err := md.Convert([]byte(tt.input), &buf)
			assert.NoError(t, err)
			assert.Contains(t, buf.String(), tt.mustContain)
		})
	}
}

func TestHighlight_Kind(t *testing.T) {
	h := NewHighlight()
	assert.Equal(t, HighlightKind, h.Kind())
}

func TestHighlight_Dump(t *testing.T) {
	h := NewHighlight()
	// Should not panic.
	h.Dump([]byte("test"), 0)
}

func TestHighlightDelimiterProcessor(t *testing.T) {
	p := &highlightDelimiterProcessor{}

	t.Run("IsDelimiter returns true for equals", func(t *testing.T) {
		assert.True(t, p.IsDelimiter('='))
		assert.False(t, p.IsDelimiter('*'))
		assert.False(t, p.IsDelimiter('-'))
	})

	t.Run("CanOpenCloser requires length >= 2", func(t *testing.T) {
		opener := &parser.Delimiter{Char: '=', Length: 2}
		closer := &parser.Delimiter{Char: '=', Length: 2}
		assert.True(t, p.CanOpenCloser(opener, closer))

		opener.Length = 1
		assert.False(t, p.CanOpenCloser(opener, closer))
	})

	t.Run("OnMatch returns Highlight node", func(t *testing.T) {
		node := p.OnMatch(2)
		_, ok := node.(*Highlight)
		assert.True(t, ok)
	})
}

func TestHighlightParser(t *testing.T) {
	runInlineParserTests(t, &highlightParser{}, '=', []parserTestCase{
		{name: "short input", input: "=", expectNil: true},
		{name: "non-highlight", input: "=abc", expectNil: true},
		{name: "unclosed highlight", input: "==unclosed", expectNil: true},
		{name: "valid syntax", input: "==text==", expectNil: false, checkNodeType: func(n ast.Node) bool { _, ok := n.(*Highlight); return ok }},
	})
}

func TestBadgeExtension(t *testing.T) {
	md := goldmark.New(
		goldmark.WithExtensions(NewBadgeExtension()),
	)

	tests := []struct {
		name        string
		input       string
		mustContain string
	}{
		{
			name:        "parses default badge",
			input:       "[!BADGE EXPERIMENTAL]",
			mustContain: "EXPERIMENTAL",
		},
		{
			name:        "parses warning badge",
			input:       "[!BADGE:warning DEPRECATED]",
			mustContain: "DEPRECATED",
		},
		{
			name:        "parses success badge",
			input:       "[!BADGE:success OK]",
			mustContain: "OK",
		},
		{
			name:        "parses error badge",
			input:       "[!BADGE:error FAILED]",
			mustContain: "FAILED",
		},
		{
			name:        "parses info badge",
			input:       "[!BADGE:info NEW]",
			mustContain: "NEW",
		},
		{
			name:        "parses badge with spaces",
			input:       "[!BADGE coming soon]",
			mustContain: "coming soon",
		},
		{
			name:        "handles missing text",
			input:       "[!BADGE]",
			mustContain: "[!BADGE]", // Should remain unchanged.
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			err := md.Convert([]byte(tt.input), &buf)
			assert.NoError(t, err)
			assert.Contains(t, buf.String(), tt.mustContain)
		})
	}
}

func TestBadge_Kind(t *testing.T) {
	b := NewBadge("warning", "TEST")
	assert.Equal(t, BadgeKind, b.Kind())
}

func TestBadge_Dump(t *testing.T) {
	b := NewBadge("info", "TEST")
	// Should not panic.
	b.Dump([]byte("test"), 0)
}

func TestBadgeParser(t *testing.T) {
	p := &badgeParser{}

	t.Run("Trigger returns bracket byte", func(t *testing.T) {
		assert.Equal(t, []byte{'['}, p.Trigger())
	})

	t.Run("Parse returns nil for short input", func(t *testing.T) {
		reader := text.NewReader([]byte("[!BA"))
		result := p.Parse(nil, reader, nil)
		assert.Nil(t, result)
	})

	t.Run("Parse returns nil for non-badge bracket", func(t *testing.T) {
		reader := text.NewReader([]byte("[link](url)"))
		result := p.Parse(nil, reader, nil)
		assert.Nil(t, result)
	})

	t.Run("Parse returns Badge for valid syntax", func(t *testing.T) {
		reader := text.NewReader([]byte("[!BADGE TEST]"))
		result := p.Parse(nil, reader, nil)
		require.NotNil(t, result)
		badge, ok := result.(*Badge)
		assert.True(t, ok)
		assert.Equal(t, "", badge.BadgeVariant)
		assert.Equal(t, "TEST", badge.BadgeText)
	})

	t.Run("Parse extracts variant", func(t *testing.T) {
		reader := text.NewReader([]byte("[!BADGE:warning ALERT]"))
		result := p.Parse(nil, reader, nil)
		require.NotNil(t, result)
		badge := result.(*Badge)
		assert.Equal(t, "warning", badge.BadgeVariant)
		assert.Equal(t, "ALERT", badge.BadgeText)
	})
}

func TestBadgeColors(t *testing.T) {
	// Verify all expected badge colors are defined.
	expectedVariants := []string{"", "warning", "success", "error", "info"}
	for _, variant := range expectedVariants {
		_, ok := badgeColors[variant]
		assert.True(t, ok, "badge color should be defined for variant %q", variant)
	}
}

func TestAdmonitionExtension(t *testing.T) {
	md := goldmark.New(
		goldmark.WithExtensions(NewAdmonitionExtension()),
	)

	tests := []struct {
		name        string
		input       string
		mustContain []string
	}{
		{
			name:        "parses NOTE admonition",
			input:       "> [!NOTE]\n> This is a note",
			mustContain: []string{"Note", "This is a note"},
		},
		{
			name:        "parses WARNING admonition",
			input:       "> [!WARNING]\n> Be careful",
			mustContain: []string{"Warning", "Be careful"},
		},
		{
			name:        "parses TIP admonition",
			input:       "> [!TIP]\n> Pro tip",
			mustContain: []string{"Tip", "Pro tip"},
		},
		{
			name:        "parses IMPORTANT admonition",
			input:       "> [!IMPORTANT]\n> This is important",
			mustContain: []string{"Important", "This is important"},
		},
		{
			name:        "parses CAUTION admonition",
			input:       "> [!CAUTION]\n> Danger",
			mustContain: []string{"Caution", "Danger"},
		},
		{
			name:        "parses inline content",
			input:       "> [!NOTE] Quick note",
			mustContain: []string{"Note", "Quick note"},
		},
		{
			name:        "parses multi-line content",
			input:       "> [!NOTE]\n> Line 1\n> Line 2",
			mustContain: []string{"Line 1", "Line 2"},
		},
		{
			name:        "handles regular blockquote",
			input:       "> Just a quote",
			mustContain: []string{"Just a quote"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			err := md.Convert([]byte(tt.input), &buf)
			assert.NoError(t, err)
			for _, expected := range tt.mustContain {
				assert.Contains(t, buf.String(), expected, "should contain %q", expected)
			}
		})
	}
}

func TestAdmonition_Kind(t *testing.T) {
	a := NewAdmonition(AdmonitionNote, "content")
	assert.Equal(t, AdmonitionKind, a.Kind())
}

func TestAdmonition_Dump(t *testing.T) {
	a := NewAdmonition(AdmonitionWarning, "test content")
	// Should not panic.
	a.Dump([]byte("test"), 0)
}

func TestAdmonitionParser(t *testing.T) {
	p := &admonitionParser{}

	t.Run("Trigger returns angle bracket", func(t *testing.T) {
		assert.Equal(t, []byte{'>'}, p.Trigger())
	})

	t.Run("CanInterruptParagraph returns true", func(t *testing.T) {
		assert.True(t, p.CanInterruptParagraph())
	})

	t.Run("CanAcceptIndentedLine returns false", func(t *testing.T) {
		assert.False(t, p.CanAcceptIndentedLine())
	})

	t.Run("Open returns nil for short input", func(t *testing.T) {
		reader := text.NewReader([]byte("> [!"))
		result, _ := p.Open(nil, reader, nil)
		assert.Nil(t, result)
	})

	t.Run("Open returns nil for non-admonition blockquote", func(t *testing.T) {
		reader := text.NewReader([]byte("> Regular quote"))
		result, _ := p.Open(nil, reader, nil)
		assert.Nil(t, result)
	})

	t.Run("Open returns Admonition for valid syntax", func(t *testing.T) {
		reader := text.NewReader([]byte("> [!NOTE]\n"))
		result, _ := p.Open(nil, reader, nil)
		require.NotNil(t, result)
		adm, ok := result.(*Admonition)
		assert.True(t, ok)
		assert.Equal(t, AdmonitionNote, adm.AdmonitionType)
	})

	t.Run("Open extracts inline content", func(t *testing.T) {
		reader := text.NewReader([]byte("> [!TIP] Quick tip here\n"))
		result, _ := p.Open(nil, reader, nil)
		require.NotNil(t, result)
		adm := result.(*Admonition)
		assert.Equal(t, "Quick tip here", adm.AdmonitionContent)
	})
}

func TestAdmonitionStyles(t *testing.T) {
	// Verify all admonition types have styles defined.
	expectedTypes := []AdmonitionType{
		AdmonitionNote,
		AdmonitionWarning,
		AdmonitionTip,
		AdmonitionImportant,
		AdmonitionCaution,
	}
	for _, adType := range expectedTypes {
		style, ok := admonitionStyles[adType]
		assert.True(t, ok, "style should be defined for type %q", adType)
		assert.NotEmpty(t, style.icon, "icon should be defined for %q", adType)
		assert.NotEmpty(t, style.label, "label should be defined for %q", adType)
		assert.NotEmpty(t, style.color, "color should be defined for %q", adType)
	}
}

func TestAdmonitionTypeConstants(t *testing.T) {
	// Verify constant values.
	assert.Equal(t, AdmonitionType("NOTE"), AdmonitionNote)
	assert.Equal(t, AdmonitionType("WARNING"), AdmonitionWarning)
	assert.Equal(t, AdmonitionType("TIP"), AdmonitionTip)
	assert.Equal(t, AdmonitionType("IMPORTANT"), AdmonitionImportant)
	assert.Equal(t, AdmonitionType("CAUTION"), AdmonitionCaution)
}

func TestMutedExtension(t *testing.T) {
	// Need GFM strikethrough extension because our muted transformer
	// converts MutedNode to Strikethrough nodes.
	md := goldmark.New(
		goldmark.WithExtensions(
			extension.Strikethrough,
			NewMutedExtension(),
		),
	)

	tests := []struct {
		name        string
		input       string
		mustContain string
	}{
		{
			name:        "parses muted syntax",
			input:       "This is ((muted)) text",
			mustContain: "muted",
		},
		{
			name:        "parses multiple muted",
			input:       "((one)) and ((two))",
			mustContain: "one",
		},
		{
			name:        "ignores single parens",
			input:       "(normal) text",
			mustContain: "(normal)",
		},
		{
			name:        "handles unclosed muted",
			input:       "((unclosed",
			mustContain: "((unclosed",
		},
		{
			name:        "handles empty muted",
			input:       "(()) text",
			mustContain: "(())",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			err := md.Convert([]byte(tt.input), &buf)
			assert.NoError(t, err)
			assert.Contains(t, buf.String(), tt.mustContain)
		})
	}
}

func TestMuted_Kind(t *testing.T) {
	m := NewMuted()
	assert.Equal(t, MutedKind, m.Kind())
}

func TestMuted_Dump(t *testing.T) {
	m := NewMuted()
	// Should not panic.
	m.Dump([]byte("test"), 0)
}

func TestMutedParser(t *testing.T) {
	runInlineParserTests(t, &mutedParser{}, '(', []parserTestCase{
		{name: "short input", input: "(", expectNil: true},
		{name: "single paren", input: "(normal)", expectNil: true},
		{name: "unclosed muted", input: "((unclosed", expectNil: true},
		{name: "valid syntax", input: "((text))", expectNil: false, checkNodeType: func(n ast.Node) bool { _, ok := n.(*Muted); return ok }},
	})
}

func TestStrictEmailRegexp(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		matches bool
	}{
		// Valid emails should match.
		{
			name:    "simple email",
			input:   "user@example.com",
			matches: true,
		},
		{
			name:    "email with subdomain",
			input:   "support@mail.company.org",
			matches: true,
		},
		{
			name:    "email with plus",
			input:   "user+tag@example.com",
			matches: true,
		},
		{
			name:    "email with dots in local part",
			input:   "first.last@example.com",
			matches: true,
		},
		// Package references should NOT match.
		{
			name:    "package with org/repo@version",
			input:   "replicatedhq/replicated@0.124.1",
			matches: false,
		},
		{
			name:    "package with simple path",
			input:   "foo/bar@1.0.0",
			matches: false,
		},
		{
			name:    "npm scoped package",
			input:   "@scope/package@2.0.0",
			matches: false,
		},
		{
			name:    "go module with path",
			input:   "github.com/user/repo@v1.2.3",
			matches: false,
		},
		// Edge cases.
		{
			name:    "no TLD",
			input:   "user@localhost",
			matches: false,
		},
		{
			name:    "just at sign",
			input:   "@",
			matches: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			match := StrictEmailRegexp.MatchString(tt.input)
			assert.Equal(t, tt.matches, match, "input: %s", tt.input)
		})
	}
}

func TestStrictLinkifyExtension(t *testing.T) {
	// Create markdown with GFM (includes default Linkify) then override with strict version.
	md := goldmark.New(
		goldmark.WithExtensions(
			extension.GFM,
			NewStrictLinkifyExtension(),
		),
	)

	tests := []struct {
		name           string
		input          string
		mustContain    string
		mustNotContain string
	}{
		{
			name:        "valid email gets linked",
			input:       "Contact user@example.com for help",
			mustContain: "mailto:user@example.com",
		},
		{
			name:           "package reference stays plain text",
			input:          "Install failed replicatedhq/replicated@0.124.1",
			mustContain:    "replicatedhq/replicated@0.124.1",
			mustNotContain: "mailto:",
		},
		{
			name:           "simple package reference",
			input:          "Use foo/bar@1.0.0",
			mustContain:    "foo/bar@1.0.0",
			mustNotContain: "mailto:",
		},
		{
			name:           "npm scoped package",
			input:          "Install @scope/package@2.0.0",
			mustContain:    "@scope/package@2.0.0",
			mustNotContain: "mailto:",
		},
		{
			name:           "go module path",
			input:          "Use github.com/user/repo@v1.2.3",
			mustContain:    "github.com/user/repo@v1.2.3",
			mustNotContain: "mailto:",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			err := md.Convert([]byte(tt.input), &buf)
			assert.NoError(t, err)
			output := buf.String()
			assert.Contains(t, output, tt.mustContain, "output: %s", output)
			if tt.mustNotContain != "" {
				assert.NotContains(t, output, tt.mustNotContain, "output: %s", output)
			}
		})
	}
}
