package markdown

import (
	"encoding/json"

	"github.com/charmbracelet/glamour/ansi"
	"github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
)

// GetDefaultStyle returns the markdown style configuration from atmos.yaml settings
// or falls back to built-in defaults if not configured
func GetDefaultStyle() ([]byte, error) {
	atmosConfig, err := config.InitCliConfig(schema.ConfigAndStacksInfo{}, false)
	if err != nil {
		return getBuiltinDefaultStyle()
	}

	// Get the built-in default style
	defaultBytes, err := getBuiltinDefaultStyle()
	if err != nil {
		return nil, err
	}

	if atmosConfig.Settings.Markdown.Document.Color == "" &&
		atmosConfig.Settings.Markdown.Heading.Color == "" &&
		atmosConfig.Settings.Markdown.H1.Color == "" &&
		atmosConfig.Settings.Markdown.H2.Color == "" &&
		atmosConfig.Settings.Markdown.H3.Color == "" {
		return defaultBytes, nil
	}

	var style ansi.StyleConfig
	if err := json.Unmarshal(defaultBytes, &style); err != nil {
		return nil, err
	}

	// Apply custom styles on top of defaults
	if atmosConfig.Settings.Markdown.Document.Color != "" {
		style.Document.Color = &atmosConfig.Settings.Markdown.Document.Color
	}

	if atmosConfig.Settings.Markdown.Heading.Color != "" {
		style.Heading.Color = &atmosConfig.Settings.Markdown.Heading.Color
		style.Heading.Bold = &atmosConfig.Settings.Markdown.Heading.Bold
	}

	if atmosConfig.Settings.Markdown.H1.Color != "" {
		style.H1.Color = &atmosConfig.Settings.Markdown.H1.Color
		if atmosConfig.Settings.Markdown.H1.BackgroundColor != "" {
			style.H1.BackgroundColor = &atmosConfig.Settings.Markdown.H1.BackgroundColor
		}
		style.H1.Bold = &atmosConfig.Settings.Markdown.H1.Bold
		style.H1.Margin = uintPtr(uint(atmosConfig.Settings.Markdown.H1.Margin))
	}

	if atmosConfig.Settings.Markdown.H2.Color != "" {
		style.H2.Color = &atmosConfig.Settings.Markdown.H2.Color
		style.H2.Bold = &atmosConfig.Settings.Markdown.H2.Bold
	}

	if atmosConfig.Settings.Markdown.H3.Color != "" {
		style.H3.Color = &atmosConfig.Settings.Markdown.H3.Color
		style.H3.Bold = &atmosConfig.Settings.Markdown.H3.Bold
	}

	if atmosConfig.Settings.Markdown.CodeBlock.Color != "" {
		if style.CodeBlock.StyleBlock.StylePrimitive.Color == nil {
			style.CodeBlock.StyleBlock.StylePrimitive.Color = &atmosConfig.Settings.Markdown.CodeBlock.Color
		} else {
			*style.CodeBlock.StyleBlock.StylePrimitive.Color = atmosConfig.Settings.Markdown.CodeBlock.Color
		}
		style.CodeBlock.Margin = uintPtr(uint(atmosConfig.Settings.Markdown.CodeBlock.Margin))
	}

	if atmosConfig.Settings.Markdown.Link.Color != "" {
		style.Link.Color = &atmosConfig.Settings.Markdown.Link.Color
		style.Link.Underline = &atmosConfig.Settings.Markdown.Link.Underline
	}

	if atmosConfig.Settings.Markdown.Strong.Color != "" {
		style.Strong.Color = &atmosConfig.Settings.Markdown.Strong.Color
		style.Strong.Bold = &atmosConfig.Settings.Markdown.Strong.Bold
	}

	if atmosConfig.Settings.Markdown.Emph.Color != "" {
		style.Emph.Color = &atmosConfig.Settings.Markdown.Emph.Color
		style.Emph.Italic = &atmosConfig.Settings.Markdown.Emph.Italic
	}

	return json.Marshal(style)
}

// this only returns the built-in default style configuration
func getBuiltinDefaultStyle() ([]byte, error) {
	style := ansi.StyleConfig{
		Document: ansi.StyleBlock{
			StylePrimitive: ansi.StylePrimitive{
				BlockPrefix: "",
				BlockSuffix: "\n",
				Color:       stringPtr("#FFFFFF"),
			},
			Margin: uintPtr(0),
		},
		BlockQuote: ansi.StyleBlock{
			StylePrimitive: ansi.StylePrimitive{
				Color: stringPtr("#9B51E0"),
			},
			Indent:      uintPtr(1),
			IndentToken: stringPtr("│ "),
		},
		Paragraph: ansi.StyleBlock{
			StylePrimitive: ansi.StylePrimitive{
				BlockPrefix: "",
				BlockSuffix: "",
				Color:       stringPtr("#FFFFFF"),
			},
		},
		List: ansi.StyleList{
			LevelIndent: 4,
		},
		Heading: ansi.StyleBlock{
			StylePrimitive: ansi.StylePrimitive{
				BlockPrefix: "",
				BlockSuffix: "\n",
				Color:       stringPtr("#00A3E0"),
				Bold:        boolPtr(true),
			},
			Margin: uintPtr(0),
		},
		H1: ansi.StyleBlock{
			StylePrimitive: ansi.StylePrimitive{
				Prefix:          "",
				Color:           stringPtr("#FFFFFF"),
				BackgroundColor: stringPtr("#9B51E0"),
				Bold:            boolPtr(true),
			},
			Margin: uintPtr(2),
		},
		H2: ansi.StyleBlock{
			StylePrimitive: ansi.StylePrimitive{
				Prefix: "## ",
				Color:  stringPtr("#9B51E0"),
				Bold:   boolPtr(true),
			},
			Margin: uintPtr(1),
		},
		H3: ansi.StyleBlock{
			StylePrimitive: ansi.StylePrimitive{
				Prefix: "### ",
				Color:  stringPtr("#00A3E0"),
				Bold:   boolPtr(true),
			},
		},
		H4: ansi.StyleBlock{
			StylePrimitive: ansi.StylePrimitive{
				Prefix: "#### ",
				Color:  stringPtr("#00A3E0"),
				Bold:   boolPtr(true),
			},
		},
		H5: ansi.StyleBlock{
			StylePrimitive: ansi.StylePrimitive{
				Prefix: "##### ",
				Color:  stringPtr("#00A3E0"),
				Bold:   boolPtr(true),
			},
		},
		H6: ansi.StyleBlock{
			StylePrimitive: ansi.StylePrimitive{
				Prefix: "###### ",
				Color:  stringPtr("#00A3E0"),
				Bold:   boolPtr(true),
			},
		},
		Text: ansi.StylePrimitive{
			Color: stringPtr("#FFFFFF"),
		},
		Strong: ansi.StylePrimitive{
			Color: stringPtr("#9B51E0"),
			Bold:  boolPtr(true),
		},
		Emph: ansi.StylePrimitive{
			Color:  stringPtr("#9B51E0"),
			Italic: boolPtr(true),
		},
		HorizontalRule: ansi.StylePrimitive{
			Color:  stringPtr("#9B51E0"),
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
				Color:  stringPtr("#9B51E0"),
				Prefix: " ",
				Bold:   boolPtr(true),
			},
			Margin: uintPtr(0),
		},
		CodeBlock: ansi.StyleCodeBlock{
			StyleBlock: ansi.StyleBlock{
				StylePrimitive: ansi.StylePrimitive{
					Color: stringPtr("#00A3E0"),
				},
				Margin: uintPtr(1),
			},
			Chroma: &ansi.Chroma{
				Text: ansi.StylePrimitive{
					Color: stringPtr("#00A3E0"),
				},
				Keyword: ansi.StylePrimitive{
					Color: stringPtr("#9B51E0"),
				},
				Literal: ansi.StylePrimitive{
					Color: stringPtr("#00A3E0"),
				},
				LiteralString: ansi.StylePrimitive{
					Color: stringPtr("#00A3E0"),
				},
				Name: ansi.StylePrimitive{
					Color: stringPtr("#00A3E0"),
				},
				LiteralNumber: ansi.StylePrimitive{
					Color: stringPtr("#00A3E0"),
				},
				Comment: ansi.StylePrimitive{
					Color: stringPtr("#9B51E0"),
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
			Color:     stringPtr("#00A3E0"),
			Underline: boolPtr(true),
		},
		LinkText: ansi.StylePrimitive{
			Color: stringPtr("#9B51E0"),
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
