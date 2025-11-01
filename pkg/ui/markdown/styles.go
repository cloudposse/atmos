package markdown

import (
	"encoding/json"

	"github.com/charmbracelet/glamour/ansi"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui/theme"
)

// applyStyleSafely applies a color to a style primitive safely handling nil pointers.
func applyStyleSafely(style *ansi.StylePrimitive, color string) {
	if style == nil {
		return
	}
	if style.Color != nil {
		*style.Color = color
	} else {
		style.Color = &color
	}
}

// GetDefaultStyle returns the markdown style configuration from atmos.yaml settings
// or falls back to built-in defaults if not configured.
func GetDefaultStyle(atmosConfig schema.AtmosConfiguration) ([]byte, error) {
	// Check if a theme is specified
	if atmosConfig.Settings.Terminal.Theme != "" {
		themeStyle, err := theme.GetGlamourStyleForTheme(atmosConfig.Settings.Terminal.Theme)
		if err == nil {
			// Successfully loaded theme, check if there are custom overrides
			if hasCustomMarkdownStyles(&atmosConfig) {
				// Apply custom styles on top of theme
				return applyCustomStylesToTheme(themeStyle, &atmosConfig)
			}
			return themeStyle, nil
		}
		// If theme loading fails, fall back to default behavior
	}

	// Get the built-in default style
	defaultBytes, err := getBuiltinDefaultStyle()
	if err != nil {
		return nil, err
	}

	var style ansi.StyleConfig
	if err := json.Unmarshal(defaultBytes, &style); err != nil {
		return nil, err
	}

	// Apply custom styles on top of defaults
	if atmosConfig.Settings.Markdown.Document.Color != "" {
		applyStyleSafely(&style.Document.StylePrimitive, atmosConfig.Settings.Markdown.Document.Color)
	}

	if atmosConfig.Settings.Markdown.Heading.Color != "" {
		applyStyleSafely(&style.Heading.StylePrimitive, atmosConfig.Settings.Markdown.Heading.Color)
		style.Heading.Bold = &atmosConfig.Settings.Markdown.Heading.Bold
	}

	if atmosConfig.Settings.Markdown.H1.Color != "" {
		applyStyleSafely(&style.H1.StylePrimitive, atmosConfig.Settings.Markdown.H1.Color)
		if atmosConfig.Settings.Markdown.H1.BackgroundColor != "" {
			style.H1.BackgroundColor = &atmosConfig.Settings.Markdown.H1.BackgroundColor
		}
		style.H1.Bold = &atmosConfig.Settings.Markdown.H1.Bold
		if atmosConfig.Settings.Markdown.H1.Margin >= 0 {
			style.H1.Margin = uintPtr(safeIntToUint(atmosConfig.Settings.Markdown.H1.Margin))
		}
	}

	if atmosConfig.Settings.Markdown.H2.Color != "" {
		applyStyleSafely(&style.H2.StylePrimitive, atmosConfig.Settings.Markdown.H2.Color)
		style.H2.Bold = &atmosConfig.Settings.Markdown.H2.Bold
	} else if atmosConfig.Settings.Markdown.H2.Bold {
		style.H2.Bold = &atmosConfig.Settings.Markdown.H2.Bold
	}

	if atmosConfig.Settings.Markdown.H3.Color != "" {
		applyStyleSafely(&style.H3.StylePrimitive, atmosConfig.Settings.Markdown.H3.Color)
		style.H3.Bold = &atmosConfig.Settings.Markdown.H3.Bold
	} else if atmosConfig.Settings.Markdown.H3.Bold {
		style.H3.Bold = &atmosConfig.Settings.Markdown.H3.Bold
	}

	if atmosConfig.Settings.Markdown.CodeBlock.Color != "" {
		if style.CodeBlock.Color != nil {
			*style.CodeBlock.Color = atmosConfig.Settings.Markdown.CodeBlock.Color
		} else {
			style.CodeBlock.Color = &atmosConfig.Settings.Markdown.CodeBlock.Color
		}
	}
	if atmosConfig.Settings.Markdown.CodeBlock.Margin >= 0 {
		style.CodeBlock.Margin = uintPtr(safeIntToUint(atmosConfig.Settings.Markdown.CodeBlock.Margin))
	}
	if atmosConfig.Settings.Markdown.CodeBlock.Indent >= 0 {
		style.CodeBlock.Indent = uintPtr(safeIntToUint(atmosConfig.Settings.Markdown.CodeBlock.Indent))
	}

	if atmosConfig.Settings.Markdown.Link.Color != "" {
		applyStyleSafely(&style.Link, atmosConfig.Settings.Markdown.Link.Color)
		style.Link.Underline = &atmosConfig.Settings.Markdown.Link.Underline
	} else if atmosConfig.Settings.Markdown.Link.Underline {
		style.Link.Underline = &atmosConfig.Settings.Markdown.Link.Underline
	}

	if atmosConfig.Settings.Markdown.Strong.Color != "" {
		applyStyleSafely(&style.Strong, atmosConfig.Settings.Markdown.Strong.Color)
		style.Strong.Bold = &atmosConfig.Settings.Markdown.Strong.Bold
	} else if atmosConfig.Settings.Markdown.Strong.Bold {
		style.Strong.Bold = &atmosConfig.Settings.Markdown.Strong.Bold
	}

	if atmosConfig.Settings.Markdown.Emph.Color != "" {
		applyStyleSafely(&style.Emph, atmosConfig.Settings.Markdown.Emph.Color)
		style.Emph.Italic = &atmosConfig.Settings.Markdown.Emph.Italic
	} else if atmosConfig.Settings.Markdown.Emph.Italic {
		style.Emph.Italic = &atmosConfig.Settings.Markdown.Emph.Italic
	}

	return json.Marshal(style)
}

const newline = "\n"

// This only returns the built-in default style configuration.
func getBuiltinDefaultStyle() ([]byte, error) {
	style := ansi.StyleConfig{
		Document: ansi.StyleBlock{
			StylePrimitive: ansi.StylePrimitive{
				BlockPrefix: "",
				BlockSuffix: newline,
				Color:       stringPtr(White),
			},
			Margin: uintPtr(0),
		},
		BlockQuote: ansi.StyleBlock{
			StylePrimitive: ansi.StylePrimitive{
				Color: stringPtr(Purple),
			},
			Indent:      uintPtr(1),
			IndentToken: stringPtr("│ "),
		},
		Paragraph: ansi.StyleBlock{
			StylePrimitive: ansi.StylePrimitive{
				BlockPrefix: "",
				BlockSuffix: "",
				Color:       stringPtr(White),
			},
			Margin: uintPtr(1),
		},
		List: ansi.StyleList{
			LevelIndent: 4,
		},
		Heading: ansi.StyleBlock{
			StylePrimitive: ansi.StylePrimitive{
				BlockPrefix: "",
				BlockSuffix: newline,
				Color:       stringPtr(Blue),
				Bold:        boolPtr(true),
			},
			Margin: uintPtr(0),
		},
		H1: ansi.StyleBlock{
			StylePrimitive: ansi.StylePrimitive{
				Prefix:          "",
				Color:           stringPtr(White),
				BackgroundColor: stringPtr(Purple),
				Bold:            boolPtr(true),
			},
			Margin: uintPtr(2),
		},
		H2: ansi.StyleBlock{
			StylePrimitive: ansi.StylePrimitive{
				Prefix: "## ",
				Color:  stringPtr(Purple),
				Bold:   boolPtr(true),
			},
			Margin: uintPtr(1),
		},
		H3: ansi.StyleBlock{
			StylePrimitive: ansi.StylePrimitive{
				Prefix: "### ",
				Color:  stringPtr(Blue),
				Bold:   boolPtr(true),
			},
		},
		H4: ansi.StyleBlock{
			StylePrimitive: ansi.StylePrimitive{
				Prefix: "#### ",
				Color:  stringPtr(Blue),
				Bold:   boolPtr(true),
			},
		},
		H5: ansi.StyleBlock{
			StylePrimitive: ansi.StylePrimitive{
				Prefix: "##### ",
				Color:  stringPtr(Blue),
				Bold:   boolPtr(true),
			},
		},
		H6: ansi.StyleBlock{
			StylePrimitive: ansi.StylePrimitive{
				Prefix: "###### ",
				Color:  stringPtr(Blue),
				Bold:   boolPtr(true),
			},
		},
		Text: ansi.StylePrimitive{
			Color: stringPtr(White),
		},
		Strong: ansi.StylePrimitive{
			Color: stringPtr(Purple),
			Bold:  boolPtr(true),
		},
		Emph: ansi.StylePrimitive{
			Color:  stringPtr(Purple),
			Italic: boolPtr(true),
		},
		HorizontalRule: ansi.StylePrimitive{
			Color:  stringPtr(Purple),
			Format: "\n--------\n",
		},
		Item: ansi.StylePrimitive{
			BlockPrefix: "• ",
		},
		Enumeration: ansi.StylePrimitive{
			BlockPrefix: ". ",
		},
		Code: ansi.StyleBlock{
			StylePrimitive: ansi.StylePrimitive{
				Color:  stringPtr(Purple),
				Prefix: " ",
				Bold:   boolPtr(true),
			},
			Margin: uintPtr(0),
		},
		CodeBlock: ansi.StyleCodeBlock{
			StyleBlock: ansi.StyleBlock{
				StylePrimitive: ansi.StylePrimitive{
					Color: stringPtr(Blue),
				},
				Margin: uintPtr(1),
				Indent: uintPtr(2),
			},
			Chroma: &ansi.Chroma{
				Text: ansi.StylePrimitive{
					Color: stringPtr(Blue),
				},
				Keyword: ansi.StylePrimitive{
					Color: stringPtr(Purple),
				},
				Literal: ansi.StylePrimitive{
					Color: stringPtr(Blue),
				},
				LiteralString: ansi.StylePrimitive{
					Color: stringPtr(Blue),
				},
				Name: ansi.StylePrimitive{
					Color: stringPtr(Blue),
				},
				LiteralNumber: ansi.StylePrimitive{
					Color: stringPtr(Blue),
				},
				Comment: ansi.StylePrimitive{
					Color: stringPtr(Purple),
				},
			},
		},
		Table: ansi.StyleTable{
			StyleBlock:      ansi.StyleBlock{},
			CenterSeparator: stringPtr("┼"),
			ColumnSeparator: stringPtr("│"),
			RowSeparator:    stringPtr("─"),
		},
		DefinitionList: ansi.StyleBlock{},
		DefinitionTerm: ansi.StylePrimitive{},
		DefinitionDescription: ansi.StylePrimitive{
			BlockPrefix: "\n",
		},
		HTMLBlock: ansi.StyleBlock{},
		HTMLSpan:  ansi.StyleBlock{},
		Link: ansi.StylePrimitive{
			Color:     stringPtr(Blue),
			Underline: boolPtr(true),
		},
		LinkText: ansi.StylePrimitive{
			Color: stringPtr(Purple),
			Bold:  boolPtr(true),
		},
	}

	return json.Marshal(style)
}

func stringPtr(s string) *string {
	return &s
}

func boolPtr(b bool) *bool {
	return &b
}

func uintPtr(u uint) *uint {
	return &u
}

// safeIntToUint safely converts an int to uint, returning 0 if the value is negative.
func safeIntToUint(i int) uint {
	if i < 0 {
		return 0
	}
	return uint(i)
}

// hasCustomMarkdownStyles checks if any custom markdown styles are configured.
func hasCustomMarkdownStyles(atmosConfig *schema.AtmosConfiguration) bool {
	m := atmosConfig.Settings.Markdown

	// Check Document styles
	if hasAnyStyleSet(m.Document) {
		return true
	}

	// Check heading styles
	if hasAnyStyleSet(m.Heading) || hasAnyStyleSet(m.H1) || hasAnyStyleSet(m.H2) ||
		hasAnyStyleSet(m.H3) || hasAnyStyleSet(m.H4) || hasAnyStyleSet(m.H5) || hasAnyStyleSet(m.H6) {
		return true
	}

	// Check text styles
	if hasAnyStyleSet(m.Text) || hasAnyStyleSet(m.Strong) || hasAnyStyleSet(m.Emph) {
		return true
	}

	// Check block styles
	if hasAnyStyleSet(m.BlockQuote) || hasAnyStyleSet(m.Paragraph) || hasAnyStyleSet(m.Hr) {
		return true
	}

	// Check list styles
	if hasAnyStyleSet(m.List) || hasAnyStyleSet(m.ListItem) || hasAnyStyleSet(m.Item) || hasAnyStyleSet(m.Enumeration) {
		return true
	}

	// Check code styles
	if hasAnyStyleSet(m.Code) || hasAnyStyleSet(m.CodeBlock) {
		return true
	}

	// Check table and definition styles
	if hasAnyStyleSet(m.Table) || hasAnyStyleSet(m.DefinitionList) ||
		hasAnyStyleSet(m.DefinitionTerm) || hasAnyStyleSet(m.DefinitionDescription) {
		return true
	}

	// Check HTML and link styles
	if hasAnyStyleSet(m.HtmlBlock) || hasAnyStyleSet(m.HtmlSpan) ||
		hasAnyStyleSet(m.Link) || hasAnyStyleSet(m.LinkText) {
		return true
	}

	return false
}

// hasAnyStyleSet checks if any style property is set in a MarkdownStyle.
func hasAnyStyleSet(style schema.MarkdownStyle) bool {
	return style.BlockPrefix != "" ||
		style.BlockSuffix != "" ||
		style.Color != "" ||
		style.BackgroundColor != "" ||
		style.Bold ||
		style.Italic ||
		style.Underline ||
		style.Margin >= 0 ||
		style.Padding != 0 ||
		style.Indent != 0 ||
		style.IndentToken != "" ||
		style.LevelIndent != 0 ||
		style.Format != "" ||
		style.Prefix != "" ||
		style.StyleOverride ||
		len(style.Chroma) > 0
}

// applyCustomStylesToTheme applies custom markdown styles on top of a theme style.
func applyCustomStylesToTheme(themeStyleBytes []byte, atmosConfig *schema.AtmosConfiguration) ([]byte, error) {
	var style ansi.StyleConfig
	if err := json.Unmarshal(themeStyleBytes, &style); err != nil {
		return nil, err
	}

	// Apply custom styles on top of theme
	if atmosConfig.Settings.Markdown.Document.Color != "" {
		applyStyleSafely(&style.Document.StylePrimitive, atmosConfig.Settings.Markdown.Document.Color)
	}

	if atmosConfig.Settings.Markdown.Heading.Color != "" {
		applyStyleSafely(&style.Heading.StylePrimitive, atmosConfig.Settings.Markdown.Heading.Color)
		style.Heading.Bold = &atmosConfig.Settings.Markdown.Heading.Bold
	}

	if atmosConfig.Settings.Markdown.H1.Color != "" {
		applyStyleSafely(&style.H1.StylePrimitive, atmosConfig.Settings.Markdown.H1.Color)
		if atmosConfig.Settings.Markdown.H1.BackgroundColor != "" {
			style.H1.BackgroundColor = &atmosConfig.Settings.Markdown.H1.BackgroundColor
		}
		style.H1.Bold = &atmosConfig.Settings.Markdown.H1.Bold
		if atmosConfig.Settings.Markdown.H1.Margin >= 0 {
			style.H1.Margin = uintPtr(safeIntToUint(atmosConfig.Settings.Markdown.H1.Margin))
		}
	}

	if atmosConfig.Settings.Markdown.H2.Color != "" {
		applyStyleSafely(&style.H2.StylePrimitive, atmosConfig.Settings.Markdown.H2.Color)
		style.H2.Bold = &atmosConfig.Settings.Markdown.H2.Bold
	} else if atmosConfig.Settings.Markdown.H2.Bold {
		style.H2.Bold = &atmosConfig.Settings.Markdown.H2.Bold
	}

	if atmosConfig.Settings.Markdown.H3.Color != "" {
		applyStyleSafely(&style.H3.StylePrimitive, atmosConfig.Settings.Markdown.H3.Color)
		style.H3.Bold = &atmosConfig.Settings.Markdown.H3.Bold
	} else if atmosConfig.Settings.Markdown.H3.Bold {
		style.H3.Bold = &atmosConfig.Settings.Markdown.H3.Bold
	}

	if atmosConfig.Settings.Markdown.CodeBlock.Color != "" {
		if style.CodeBlock.Color != nil {
			*style.CodeBlock.Color = atmosConfig.Settings.Markdown.CodeBlock.Color
		} else {
			style.CodeBlock.Color = &atmosConfig.Settings.Markdown.CodeBlock.Color
		}
	}
	if atmosConfig.Settings.Markdown.CodeBlock.Margin >= 0 {
		style.CodeBlock.Margin = uintPtr(safeIntToUint(atmosConfig.Settings.Markdown.CodeBlock.Margin))
	}
	if atmosConfig.Settings.Markdown.CodeBlock.Indent >= 0 {
		style.CodeBlock.Indent = uintPtr(safeIntToUint(atmosConfig.Settings.Markdown.CodeBlock.Indent))
	}

	if atmosConfig.Settings.Markdown.Link.Color != "" {
		applyStyleSafely(&style.Link, atmosConfig.Settings.Markdown.Link.Color)
		style.Link.Underline = &atmosConfig.Settings.Markdown.Link.Underline
	} else if atmosConfig.Settings.Markdown.Link.Underline {
		style.Link.Underline = &atmosConfig.Settings.Markdown.Link.Underline
	}

	if atmosConfig.Settings.Markdown.Strong.Color != "" {
		applyStyleSafely(&style.Strong, atmosConfig.Settings.Markdown.Strong.Color)
		style.Strong.Bold = &atmosConfig.Settings.Markdown.Strong.Bold
	} else if atmosConfig.Settings.Markdown.Strong.Bold {
		style.Strong.Bold = &atmosConfig.Settings.Markdown.Strong.Bold
	}

	if atmosConfig.Settings.Markdown.Emph.Color != "" {
		applyStyleSafely(&style.Emph, atmosConfig.Settings.Markdown.Emph.Color)
		style.Emph.Italic = &atmosConfig.Settings.Markdown.Emph.Italic
	} else if atmosConfig.Settings.Markdown.Emph.Italic {
		style.Emph.Italic = &atmosConfig.Settings.Markdown.Emph.Italic
	}

	return json.Marshal(style)
}
