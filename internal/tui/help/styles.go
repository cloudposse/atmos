package help

import "github.com/charmbracelet/glamour/ansi"

func boolPtr(b bool) *bool       { return &b }
func stringPtr(s string) *string { return &s }
func uintPtr(u uint) *uint       { return &u }

const (
	defaultListIndent = 2
	defaultMargin     = 2
)

var (
	atmosHelpStyleConfig = ansi.StyleConfig{
		Document: ansi.StyleBlock{
			StylePrimitive: ansi.StylePrimitive{
				BlockPrefix: "\n",
				BlockSuffix: "\n",
				Color:       stringPtr("252"),
			},
			Margin: uintPtr(defaultMargin),
		},
		BlockQuote: ansi.StyleBlock{
			StylePrimitive: ansi.StylePrimitive{
				Color:  stringPtr("#f1fa8c"),
				Italic: boolPtr(true),
			},
			Indent:      uintPtr(1),
			IndentToken: stringPtr("│ "),
		},
		List: ansi.StyleList{
			LevelIndent: defaultListIndent,
		},
		Heading: ansi.StyleBlock{
			StylePrimitive: ansi.StylePrimitive{
				BlockSuffix: "\n",
				Color:       stringPtr("39"),
				Bold:        boolPtr(true),
			},
		},
		H1: ansi.StyleBlock{
			StylePrimitive: ansi.StylePrimitive{
				Prefix:          " ",
				Suffix:          " ",
				Color:           stringPtr("228"),
				BackgroundColor: stringPtr("63"),
				Bold:            boolPtr(true),
			},
		},
		H2: ansi.StyleBlock{
			StylePrimitive: ansi.StylePrimitive{
				Prefix:          " ",
				Suffix:          " ",
				Color:           stringPtr("228"),
				BackgroundColor: stringPtr("63"),
				Bold:            boolPtr(true),
			},
		},
		H3: ansi.StyleBlock{
			StylePrimitive: ansi.StylePrimitive{
				Prefix:          " ",
				Suffix:          " ",
				Color:           stringPtr("228"),
				BackgroundColor: stringPtr("63"),
				Bold:            boolPtr(true),
			},
		},
		H4: ansi.StyleBlock{
			StylePrimitive: ansi.StylePrimitive{
				Prefix:          " ",
				Suffix:          " ",
				Color:           stringPtr("228"),
				BackgroundColor: stringPtr("63"),
				Bold:            boolPtr(true),
			},
		},
		H5: ansi.StyleBlock{
			StylePrimitive: ansi.StylePrimitive{
				Prefix:          " ",
				Suffix:          " ",
				Color:           stringPtr("228"),
				BackgroundColor: stringPtr("63"),
				Bold:            boolPtr(true),
			},
		},
		H6: ansi.StyleBlock{
			StylePrimitive: ansi.StylePrimitive{
				Prefix:          " ",
				Suffix:          " ",
				Color:           stringPtr("228"),
				BackgroundColor: stringPtr("63"),
				Bold:            boolPtr(false),
			},
		},
		Strikethrough: ansi.StylePrimitive{
			CrossedOut: boolPtr(true),
		},
		Emph: ansi.StylePrimitive{
			Italic: boolPtr(true),
		},
		Strong: ansi.StylePrimitive{
			Bold: boolPtr(true),
		},
		HorizontalRule: ansi.StylePrimitive{
			Color:  stringPtr("240"),
			Format: "\n--------\n",
		},
		Item: ansi.StylePrimitive{
			BlockPrefix: "• ",
		},
		Enumeration: ansi.StylePrimitive{
			BlockPrefix: ". ",
		},
		Task: ansi.StyleTask{
			StylePrimitive: ansi.StylePrimitive{},
			Ticked:         "[✓] ",
			Unticked:       "[ ] ",
		},
		Link: ansi.StylePrimitive{
			Color:     stringPtr("30"),
			Underline: boolPtr(true),
		},
		LinkText: ansi.StylePrimitive{
			Color: stringPtr("35"),
			Bold:  boolPtr(true),
		},
		Image: ansi.StylePrimitive{
			Color:     stringPtr("212"),
			Underline: boolPtr(true),
		},
		ImageText: ansi.StylePrimitive{
			Color:  stringPtr("243"),
			Format: "Image: {{.text}} →",
		},
		Code: ansi.StyleBlock{
			StylePrimitive: ansi.StylePrimitive{
				Prefix:          "",
				Suffix:          "",
				Color:           stringPtr("#3ec1d5"),
				BackgroundColor: stringPtr("0"),
			},
		},
		CodeBlock: ansi.StyleCodeBlock{
			StyleBlock: ansi.StyleBlock{
				StylePrimitive: ansi.StylePrimitive{
					Color: stringPtr("244"),
				},
				Margin: uintPtr(defaultMargin),
			},
			Chroma: &ansi.Chroma{
				Text: ansi.StylePrimitive{
					Color: stringPtr("#C4C4C4"),
				},
				Error: ansi.StylePrimitive{
					Color:           stringPtr("#F1F1F1"),
					BackgroundColor: stringPtr("#F05B5B"),
				},
				Comment: ansi.StylePrimitive{
					Color: stringPtr("#676767"),
				},
				CommentPreproc: ansi.StylePrimitive{
					Color: stringPtr("#FF875F"),
				},
				Keyword: ansi.StylePrimitive{
					Color: stringPtr("#00AAFF"),
				},
				KeywordReserved: ansi.StylePrimitive{
					Color: stringPtr("#FF5FD2"),
				},
				KeywordNamespace: ansi.StylePrimitive{
					Color: stringPtr("#FF5F87"),
				},
				KeywordType: ansi.StylePrimitive{
					Color: stringPtr("#6E6ED8"),
				},
				Operator: ansi.StylePrimitive{
					Color: stringPtr("#EF8080"),
				},
				Punctuation: ansi.StylePrimitive{
					Color: stringPtr("#E8E8A8"),
				},
				Name: ansi.StylePrimitive{
					Color: stringPtr("#C4C4C4"),
				},
				NameBuiltin: ansi.StylePrimitive{
					Color: stringPtr("#FF8EC7"),
				},
				NameTag: ansi.StylePrimitive{
					Color: stringPtr("#B083EA"),
				},
				NameAttribute: ansi.StylePrimitive{
					Color: stringPtr("#7A7AE6"),
				},
				NameClass: ansi.StylePrimitive{
					Color:     stringPtr("#F1F1F1"),
					Underline: boolPtr(true),
					Bold:      boolPtr(true),
				},
				NameDecorator: ansi.StylePrimitive{
					Color: stringPtr("#FFFF87"),
				},
				NameFunction: ansi.StylePrimitive{
					Color: stringPtr("#00D787"),
				},
				LiteralNumber: ansi.StylePrimitive{
					Color: stringPtr("#6EEFC0"),
				},
				LiteralString: ansi.StylePrimitive{
					Color: stringPtr("#C69669"),
				},
				LiteralStringEscape: ansi.StylePrimitive{
					Color: stringPtr("#AFFFD7"),
				},
				GenericDeleted: ansi.StylePrimitive{
					Color: stringPtr("#FD5B5B"),
				},
				GenericEmph: ansi.StylePrimitive{
					Italic: boolPtr(true),
				},
				GenericInserted: ansi.StylePrimitive{
					Color: stringPtr("#00D787"),
				},
				GenericStrong: ansi.StylePrimitive{
					Bold: boolPtr(true),
				},
				GenericSubheading: ansi.StylePrimitive{
					Color: stringPtr("#777777"),
				},
				Background: ansi.StylePrimitive{
					BackgroundColor: stringPtr("#373737"),
				},
			},
		},
		Table: ansi.StyleTable{
			StyleBlock: ansi.StyleBlock{
				StylePrimitive: ansi.StylePrimitive{},
			},
			CenterSeparator: stringPtr("┼"),
			ColumnSeparator: stringPtr("│"),
			RowSeparator:    stringPtr("─"),
		},
		DefinitionDescription: ansi.StylePrimitive{
			BlockPrefix: "\n🠶 ",
		},
	}
)
