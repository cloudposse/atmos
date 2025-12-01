package markdown

import (
	"encoding/json"
	"testing"

	"github.com/charmbracelet/glamour/ansi"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestApplyStyleSafelyDirect(t *testing.T) {
	tests := []struct {
		name          string
		style         *ansi.StylePrimitive
		color         string
		expectedColor *string
	}{
		{
			name:          "nil style pointer",
			style:         nil,
			color:         "#FF0000",
			expectedColor: nil,
		},
		{
			name: "existing color pointer",
			style: &ansi.StylePrimitive{
				Color: stringPtr("#000000"),
			},
			color:         "#FF0000",
			expectedColor: stringPtr("#FF0000"),
		},
		{
			name:          "nil color pointer",
			style:         &ansi.StylePrimitive{},
			color:         "#FF0000",
			expectedColor: stringPtr("#FF0000"),
		},
		{
			name: "overwrite existing color",
			style: &ansi.StylePrimitive{
				Color: stringPtr("#AAAAAA"),
			},
			color:         "#FFFFFF",
			expectedColor: stringPtr("#FFFFFF"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			applyStyleSafely(tt.style, tt.color)
			if tt.style == nil {
				// Should not panic on nil style
				assert.Nil(t, tt.style)
			} else if tt.expectedColor != nil {
				require.NotNil(t, tt.style.Color)
				assert.Equal(t, *tt.expectedColor, *tt.style.Color)
			}
		})
	}
}

func TestGetBuiltinDefaultStyle(t *testing.T) {
	styleBytes, err := getBuiltinDefaultStyle()
	require.NoError(t, err)
	require.NotNil(t, styleBytes)

	var style ansi.StyleConfig
	err = json.Unmarshal(styleBytes, &style)
	require.NoError(t, err)

	// Test key style elements are present
	t.Run("document style", func(t *testing.T) {
		require.NotNil(t, style.Document.Color)
		assert.Equal(t, White, *style.Document.Color)
		assert.Equal(t, newline, style.Document.BlockSuffix)
	})

	t.Run("heading styles", func(t *testing.T) {
		require.NotNil(t, style.H1.Color)
		assert.Equal(t, White, *style.H1.Color)
		require.NotNil(t, style.H1.BackgroundColor)
		assert.Equal(t, Purple, *style.H1.BackgroundColor)
		require.NotNil(t, style.H1.Bold)
		assert.True(t, *style.H1.Bold)

		require.NotNil(t, style.H2.Color)
		assert.Equal(t, Purple, *style.H2.Color)
		assert.Equal(t, "## ", style.H2.Prefix)

		require.NotNil(t, style.H3.Color)
		assert.Equal(t, Blue, *style.H3.Color)
		assert.Equal(t, "### ", style.H3.Prefix)
	})

	t.Run("text formatting styles", func(t *testing.T) {
		require.NotNil(t, style.Strong.Color)
		assert.Equal(t, Purple, *style.Strong.Color)
		require.NotNil(t, style.Strong.Bold)
		assert.True(t, *style.Strong.Bold)

		require.NotNil(t, style.Emph.Color)
		assert.Equal(t, Purple, *style.Emph.Color)
		require.NotNil(t, style.Emph.Italic)
		assert.True(t, *style.Emph.Italic)
	})

	t.Run("code styles", func(t *testing.T) {
		require.NotNil(t, style.Code.Color)
		assert.Equal(t, Purple, *style.Code.Color)
		assert.Equal(t, " ", style.Code.Prefix)

		require.NotNil(t, style.CodeBlock.Color)
		assert.Equal(t, Blue, *style.CodeBlock.Color)
	})

	t.Run("link styles", func(t *testing.T) {
		require.NotNil(t, style.Link.Color)
		assert.Equal(t, Blue, *style.Link.Color)
		require.NotNil(t, style.Link.Underline)
		assert.True(t, *style.Link.Underline)

		require.NotNil(t, style.LinkText.Color)
		assert.Equal(t, Purple, *style.LinkText.Color)
	})

	t.Run("list styles", func(t *testing.T) {
		assert.Equal(t, uint(4), style.List.LevelIndent)
		assert.Equal(t, "• ", style.Item.BlockPrefix)
		assert.Equal(t, ". ", style.Enumeration.BlockPrefix)
	})

	t.Run("blockquote style", func(t *testing.T) {
		require.NotNil(t, style.BlockQuote.Color)
		assert.Equal(t, Purple, *style.BlockQuote.Color)
		require.NotNil(t, style.BlockQuote.IndentToken)
		assert.Equal(t, "│ ", *style.BlockQuote.IndentToken)
	})

	t.Run("table styles", func(t *testing.T) {
		require.NotNil(t, style.Table.CenterSeparator)
		assert.Equal(t, "┼", *style.Table.CenterSeparator)
		require.NotNil(t, style.Table.ColumnSeparator)
		assert.Equal(t, "│", *style.Table.ColumnSeparator)
		require.NotNil(t, style.Table.RowSeparator)
		assert.Equal(t, "─", *style.Table.RowSeparator)
	})
}

func TestGetDefaultStyle_EmptyConfig(t *testing.T) {
	atmosConfig := schema.AtmosConfiguration{}

	styleBytes, err := GetDefaultStyle(atmosConfig)
	require.NoError(t, err)
	require.NotNil(t, styleBytes)

	var style ansi.StyleConfig
	err = json.Unmarshal(styleBytes, &style)
	require.NoError(t, err)

	// Should return built-in defaults
	require.NotNil(t, style.Document.Color)
	assert.Equal(t, White, *style.Document.Color)
}

func TestGetDefaultStyle_CustomDocumentColor(t *testing.T) {
	atmosConfig := schema.AtmosConfiguration{
		Settings: schema.AtmosSettings{
			Markdown: schema.MarkdownSettings{
				Document: schema.MarkdownStyle{
					Color: "#CUSTOM1",
				},
			},
		},
	}

	styleBytes, err := GetDefaultStyle(atmosConfig)
	require.NoError(t, err)
	require.NotNil(t, styleBytes)

	var style ansi.StyleConfig
	err = json.Unmarshal(styleBytes, &style)
	require.NoError(t, err)

	require.NotNil(t, style.Document.Color)
	assert.Equal(t, "#CUSTOM1", *style.Document.Color)
}

func TestGetDefaultStyle_CustomHeadingStyles(t *testing.T) {
	atmosConfig := schema.AtmosConfiguration{
		Settings: schema.AtmosSettings{
			Markdown: schema.MarkdownSettings{
				Heading: schema.MarkdownStyle{
					Color: "#HEADING",
					Bold:  false,
				},
				H1: schema.MarkdownStyle{
					Color:           "#H1COLOR",
					BackgroundColor: "#H1BG",
					Bold:            false,
					Margin:          5,
				},
				H2: schema.MarkdownStyle{
					Color: "#H2COLOR",
					Bold:  false,
				},
				H3: schema.MarkdownStyle{
					Color: "#H3COLOR",
					Bold:  true,
				},
			},
		},
	}

	styleBytes, err := GetDefaultStyle(atmosConfig)
	require.NoError(t, err)

	var style ansi.StyleConfig
	err = json.Unmarshal(styleBytes, &style)
	require.NoError(t, err)

	// Check custom heading style
	require.NotNil(t, style.Heading.Color)
	assert.Equal(t, "#HEADING", *style.Heading.Color)
	require.NotNil(t, style.Heading.Bold)
	assert.False(t, *style.Heading.Bold)

	// Check custom H1 style
	require.NotNil(t, style.H1.Color)
	assert.Equal(t, "#H1COLOR", *style.H1.Color)
	require.NotNil(t, style.H1.BackgroundColor)
	assert.Equal(t, "#H1BG", *style.H1.BackgroundColor)
	require.NotNil(t, style.H1.Bold)
	assert.False(t, *style.H1.Bold)
	require.NotNil(t, style.H1.Margin)
	assert.Equal(t, uint(5), *style.H1.Margin)

	// Check custom H2 style
	require.NotNil(t, style.H2.Color)
	assert.Equal(t, "#H2COLOR", *style.H2.Color)

	// Check custom H3 style
	require.NotNil(t, style.H3.Color)
	assert.Equal(t, "#H3COLOR", *style.H3.Color)
}

func TestGetDefaultStyle_CustomCodeBlock(t *testing.T) {
	atmosConfig := schema.AtmosConfiguration{
		Settings: schema.AtmosSettings{
			Markdown: schema.MarkdownSettings{
				CodeBlock: schema.MarkdownStyle{
					Color:  "#CODECOLOR",
					Margin: 3,
				},
			},
		},
	}

	styleBytes, err := GetDefaultStyle(atmosConfig)
	require.NoError(t, err)

	var style ansi.StyleConfig
	err = json.Unmarshal(styleBytes, &style)
	require.NoError(t, err)

	require.NotNil(t, style.CodeBlock.Color)
	assert.Equal(t, "#CODECOLOR", *style.CodeBlock.Color)
	require.NotNil(t, style.CodeBlock.Margin)
	assert.Equal(t, uint(3), *style.CodeBlock.Margin)
}

func TestGetDefaultStyle_CustomLinkStyle(t *testing.T) {
	atmosConfig := schema.AtmosConfiguration{
		Settings: schema.AtmosSettings{
			Markdown: schema.MarkdownSettings{
				Link: schema.MarkdownStyle{
					Color:     "#LINKCOLOR",
					Underline: false,
				},
			},
		},
	}

	styleBytes, err := GetDefaultStyle(atmosConfig)
	require.NoError(t, err)

	var style ansi.StyleConfig
	err = json.Unmarshal(styleBytes, &style)
	require.NoError(t, err)

	require.NotNil(t, style.Link.Color)
	assert.Equal(t, "#LINKCOLOR", *style.Link.Color)
	require.NotNil(t, style.Link.Underline)
	assert.False(t, *style.Link.Underline)
}

func TestGetDefaultStyle_CustomTextFormatting(t *testing.T) {
	atmosConfig := schema.AtmosConfiguration{
		Settings: schema.AtmosSettings{
			Markdown: schema.MarkdownSettings{
				Strong: schema.MarkdownStyle{
					Color: "#STRONGCOLOR",
					Bold:  false,
				},
				Emph: schema.MarkdownStyle{
					Color:  "#EMPHCOLOR",
					Italic: false,
				},
			},
		},
	}

	styleBytes, err := GetDefaultStyle(atmosConfig)
	require.NoError(t, err)

	var style ansi.StyleConfig
	err = json.Unmarshal(styleBytes, &style)
	require.NoError(t, err)

	// Check custom strong style
	require.NotNil(t, style.Strong.Color)
	assert.Equal(t, "#STRONGCOLOR", *style.Strong.Color)
	require.NotNil(t, style.Strong.Bold)
	assert.False(t, *style.Strong.Bold)

	// Check custom emph style
	require.NotNil(t, style.Emph.Color)
	assert.Equal(t, "#EMPHCOLOR", *style.Emph.Color)
	require.NotNil(t, style.Emph.Italic)
	assert.False(t, *style.Emph.Italic)
}

func TestGetDefaultStyle_AllCustomStyles(t *testing.T) {
	atmosConfig := schema.AtmosConfiguration{
		Settings: schema.AtmosSettings{
			Markdown: schema.MarkdownSettings{
				Document: schema.MarkdownStyle{
					Color: "#DOC",
				},
				Heading: schema.MarkdownStyle{
					Color: "#HEAD",
					Bold:  false,
				},
				H1: schema.MarkdownStyle{
					Color:           "#H1",
					BackgroundColor: "#H1BG",
					Bold:            true,
					Margin:          2,
				},
				H2: schema.MarkdownStyle{
					Color: "#H2",
					Bold:  true,
				},
				H3: schema.MarkdownStyle{
					Color: "#H3",
					Bold:  true,
				},
				CodeBlock: schema.MarkdownStyle{
					Color:  "#CODE",
					Margin: 2,
				},
				Link: schema.MarkdownStyle{
					Color:     "#LINK",
					Underline: true,
				},
				Strong: schema.MarkdownStyle{
					Color: "#STRONG",
					Bold:  true,
				},
				Emph: schema.MarkdownStyle{
					Color:  "#EMPH",
					Italic: true,
				},
			},
		},
	}

	styleBytes, err := GetDefaultStyle(atmosConfig)
	require.NoError(t, err)

	var style ansi.StyleConfig
	err = json.Unmarshal(styleBytes, &style)
	require.NoError(t, err)

	// Verify all customizations are applied
	assert.Equal(t, "#DOC", *style.Document.Color)
	assert.Equal(t, "#HEAD", *style.Heading.Color)
	assert.Equal(t, "#H1", *style.H1.Color)
	assert.Equal(t, "#H1BG", *style.H1.BackgroundColor)
	assert.Equal(t, "#H2", *style.H2.Color)
	assert.Equal(t, "#H3", *style.H3.Color)
	assert.Equal(t, "#CODE", *style.CodeBlock.Color)
	assert.Equal(t, "#LINK", *style.Link.Color)
	assert.Equal(t, "#STRONG", *style.Strong.Color)
	assert.Equal(t, "#EMPH", *style.Emph.Color)
}

func TestHelperFunctions(t *testing.T) {
	t.Run("stringPtr", func(t *testing.T) {
		s := "test"
		ptr := stringPtr(s)
		require.NotNil(t, ptr)
		assert.Equal(t, s, *ptr)
	})

	t.Run("boolPtr", func(t *testing.T) {
		b := true
		ptr := boolPtr(b)
		require.NotNil(t, ptr)
		assert.Equal(t, b, *ptr)

		b2 := false
		ptr2 := boolPtr(b2)
		require.NotNil(t, ptr2)
		assert.Equal(t, b2, *ptr2)
	})

	t.Run("uintPtr", func(t *testing.T) {
		u := uint(42)
		ptr := uintPtr(u)
		require.NotNil(t, ptr)
		assert.Equal(t, u, *ptr)
	})
}
