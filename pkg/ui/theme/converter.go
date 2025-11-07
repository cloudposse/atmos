package theme

import (
	"encoding/json"
	"fmt"

	"github.com/charmbracelet/glamour/ansi"
)

// ConvertToGlamourStyle converts a terminal theme to a glamour style configuration.
func ConvertToGlamourStyle(t *Theme) ([]byte, error) {
	style := createGlamourStyleFromTheme(t)
	return json.Marshal(style)
}

// createGlamourStyleFromTheme creates a glamour style config from a theme.
//
//revive:disable:function-length
//nolint:funlen // Complex style configuration better kept together.
func createGlamourStyleFromTheme(t *Theme) *ansi.StyleConfig {
	// Use the theme's foreground and background as base colors
	docColor := t.Foreground

	// Determine primary colors based on theme
	primaryColor := t.Blue
	secondaryColor := t.Magenta
	accentColor := t.Cyan

	// For headings, use brighter colors
	h1BgColor := primaryColor
	h1Color := t.Background // Inverted for better contrast
	if !t.Meta.IsDark {
		h1Color = t.White
	}

	style := &ansi.StyleConfig{
		Document: ansi.StyleBlock{
			StylePrimitive: ansi.StylePrimitive{
				BlockPrefix: "",
				BlockSuffix: "\n",
				Color:       &docColor,
			},
			Margin: uintPtr(0),
		},
		BlockQuote: ansi.StyleBlock{
			StylePrimitive: ansi.StylePrimitive{
				Color: &secondaryColor,
			},
			Indent:      uintPtr(1),
			IndentToken: stringPtr("│ "),
		},
		Paragraph: ansi.StyleBlock{
			StylePrimitive: ansi.StylePrimitive{
				BlockPrefix: "",
				BlockSuffix: "",
				Color:       &docColor,
			},
		},
		List: ansi.StyleList{
			LevelIndent: 4,
		},
		Heading: ansi.StyleBlock{
			StylePrimitive: ansi.StylePrimitive{
				BlockPrefix: "",
				BlockSuffix: "\n",
				Color:       &primaryColor,
				Bold:        boolPtr(true),
			},
			Margin: uintPtr(0),
		},
		H1: ansi.StyleBlock{
			StylePrimitive: ansi.StylePrimitive{
				Prefix:          "",
				Color:           &h1Color,
				BackgroundColor: &h1BgColor,
				Bold:            boolPtr(true),
			},
			Margin: uintPtr(2),
		},
		H2: ansi.StyleBlock{
			StylePrimitive: ansi.StylePrimitive{
				Prefix: "## ",
				Color:  &secondaryColor,
				Bold:   boolPtr(true),
			},
			Margin: uintPtr(1),
		},
		H3: ansi.StyleBlock{
			StylePrimitive: ansi.StylePrimitive{
				Prefix: "### ",
				Color:  &primaryColor,
				Bold:   boolPtr(true),
			},
		},
		H4: ansi.StyleBlock{
			StylePrimitive: ansi.StylePrimitive{
				Prefix: "#### ",
				Color:  &primaryColor,
				Bold:   boolPtr(true),
			},
		},
		H5: ansi.StyleBlock{
			StylePrimitive: ansi.StylePrimitive{
				Prefix: "##### ",
				Color:  &primaryColor,
				Bold:   boolPtr(true),
			},
		},
		H6: ansi.StyleBlock{
			StylePrimitive: ansi.StylePrimitive{
				Prefix: "###### ",
				Color:  &primaryColor,
				Bold:   boolPtr(true),
			},
		},
		Text: ansi.StylePrimitive{
			Color: &docColor,
		},
		Strong: ansi.StylePrimitive{
			Color: &secondaryColor,
			Bold:  boolPtr(true),
		},
		Emph: ansi.StylePrimitive{
			Color:  &secondaryColor,
			Italic: boolPtr(true),
		},
		HorizontalRule: ansi.StylePrimitive{
			Color:  &secondaryColor,
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
				Color:  &accentColor,
				Prefix: " ",
				Bold:   boolPtr(true),
			},
			Margin: uintPtr(0),
		},
		CodeBlock: ansi.StyleCodeBlock{
			StyleBlock: ansi.StyleBlock{
				StylePrimitive: ansi.StylePrimitive{
					Color: &primaryColor,
				},
				Margin: uintPtr(1),
				Indent: uintPtr(2),
			},
			Chroma: createChromaStyle(t),
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
			Color:     &primaryColor,
			Underline: boolPtr(true),
		},
		LinkText: ansi.StylePrimitive{
			Color: &secondaryColor,
			Bold:  boolPtr(true),
		},
	}

	return style
}

// createChromaStyle creates syntax highlighting configuration from theme.
//
//nolint:funlen // Syntax highlighting configuration better kept together.
func createChromaStyle(t *Theme) *ansi.Chroma {
	return &ansi.Chroma{
		Text: ansi.StylePrimitive{
			Color: &t.Foreground,
		},
		Keyword: ansi.StylePrimitive{
			Color: &t.Magenta,
		},
		Literal: ansi.StylePrimitive{
			Color: &t.Blue,
		},
		LiteralString: ansi.StylePrimitive{
			Color: &t.Green,
		},
		Name: ansi.StylePrimitive{
			Color: &t.Foreground,
		},
		LiteralNumber: ansi.StylePrimitive{
			Color: &t.Cyan,
		},
		Comment: ansi.StylePrimitive{
			Color: &t.BrightBlack,
		},
		GenericDeleted: ansi.StylePrimitive{
			Color: &t.Red,
		},
		GenericInserted: ansi.StylePrimitive{
			Color: &t.Green,
		},
		GenericSubheading: ansi.StylePrimitive{
			Color: &t.Blue,
		},
		GenericStrong: ansi.StylePrimitive{
			Bold: boolPtr(true),
		},
		GenericEmph: ansi.StylePrimitive{
			Italic: boolPtr(true),
		},
		NameBuiltin: ansi.StylePrimitive{
			Color: &t.Cyan,
		},
		NameClass: ansi.StylePrimitive{
			Color: &t.Yellow,
		},
		NameFunction: ansi.StylePrimitive{
			Color: &t.Blue,
		},
		NameConstant: ansi.StylePrimitive{
			Color: &t.Magenta,
		},
		NameTag: ansi.StylePrimitive{
			Color: &t.Red,
		},
		NameAttribute: ansi.StylePrimitive{
			Color: &t.Yellow,
		},
		Operator: ansi.StylePrimitive{
			Color: &t.Cyan,
		},
		Punctuation: ansi.StylePrimitive{
			Color: &t.Foreground,
		},
		Background: ansi.StylePrimitive{
			BackgroundColor: &t.Background,
		},
	}
}

// GetGlamourStyleForTheme returns a glamour style for a theme by name.
func GetGlamourStyleForTheme(themeName string) ([]byte, error) {
	registry, err := NewRegistry()
	if err != nil {
		return nil, fmt.Errorf("failed to create theme registry: %w", err)
	}

	theme := registry.GetOrDefault(themeName)
	return ConvertToGlamourStyle(theme)
}

// Helper functions for creating pointers.
func stringPtr(s string) *string {
	return &s
}

func boolPtr(b bool) *bool {
	return &b
}

func uintPtr(u uint) *uint {
	return &u
}
