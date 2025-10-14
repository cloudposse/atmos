package markdown

import (
	"encoding/json"

	"github.com/charmbracelet/glamour/ansi"

	"github.com/cloudposse/atmos/pkg/perf"
)

// Note: Cloud Posse color constants (CPLightGray, CPMidGray, CPDarkGray, CPPurple)
// are defined in codeblock.go to avoid duplication.

// GetHelpStyle returns a markdown style configuration optimized for command help text.
// This uses the Cloud Posse color scheme (grayscale + purple accent) with transparent backgrounds.
func GetHelpStyle() ([]byte, error) {
	defer perf.Track(nil, "markdown.GetHelpStyle")()

	style := ansi.StyleConfig{
		Document: ansi.StyleBlock{
			StylePrimitive: ansi.StylePrimitive{
				BlockPrefix: "",
				BlockSuffix: "\n",
				Color:       stringPtr(CPLightGray),
			},
			Margin: uintPtr(0),
		},
		BlockQuote: ansi.StyleBlock{
			StylePrimitive: ansi.StylePrimitive{
				Color: stringPtr(CPMidGray),
			},
			Indent:      uintPtr(1),
			IndentToken: stringPtr("│ "),
		},
		Paragraph: ansi.StyleBlock{
			StylePrimitive: ansi.StylePrimitive{
				BlockPrefix: "",
				BlockSuffix: "",
				Color:       stringPtr(CPLightGray),
			},
		},
		List: ansi.StyleList{
			LevelIndent: 4,
		},
		Heading: ansi.StyleBlock{
			StylePrimitive: ansi.StylePrimitive{
				BlockPrefix: "",
				BlockSuffix: "\n",
				Color:       stringPtr(CPPurple),
				Bold:        boolPtr(true),
			},
			Margin: uintPtr(0),
		},
		H1: ansi.StyleBlock{
			StylePrimitive: ansi.StylePrimitive{
				Prefix: "",
				Color:  stringPtr(CPPurple),
				Bold:   boolPtr(true),
			},
			Margin: uintPtr(1),
		},
		H2: ansi.StyleBlock{
			StylePrimitive: ansi.StylePrimitive{
				Prefix: "## ",
				Color:  stringPtr(CPPurple),
				Bold:   boolPtr(true),
			},
			Margin: uintPtr(1),
		},
		H3: ansi.StyleBlock{
			StylePrimitive: ansi.StylePrimitive{
				Prefix: "### ",
				Color:  stringPtr(CPPurple),
				Bold:   boolPtr(false),
			},
		},
		H4: ansi.StyleBlock{
			StylePrimitive: ansi.StylePrimitive{
				Prefix: "#### ",
				Color:  stringPtr(CPMidGray),
				Bold:   boolPtr(false),
			},
		},
		H5: ansi.StyleBlock{
			StylePrimitive: ansi.StylePrimitive{
				Prefix: "##### ",
				Color:  stringPtr(CPMidGray),
				Bold:   boolPtr(false),
			},
		},
		H6: ansi.StyleBlock{
			StylePrimitive: ansi.StylePrimitive{
				Prefix: "###### ",
				Color:  stringPtr(CPMidGray),
				Bold:   boolPtr(false),
			},
		},
		Text: ansi.StylePrimitive{
			Color: stringPtr(CPLightGray),
		},
		Strong: ansi.StylePrimitive{
			Color: stringPtr(CPWhite),
			Bold:  boolPtr(true),
		},
		Emph: ansi.StylePrimitive{
			Color:  stringPtr(CPPurple),
			Italic: boolPtr(true),
		},
		HorizontalRule: ansi.StylePrimitive{
			Color:  stringPtr(CPMidGray),
			Format: "\n--------\n",
		},
		Item: ansi.StylePrimitive{
			BlockPrefix: "• ",
		},
		Enumeration: ansi.StylePrimitive{
			BlockPrefix: ". ",
		},
		// Inline code - no background, just purple
		Code: ansi.StyleBlock{
			StylePrimitive: ansi.StylePrimitive{
				Color:  stringPtr(CPPurple),
				Prefix: "`",
				Suffix: "`",
			},
			Margin: uintPtr(0),
		},
		// Code blocks - no syntax highlighting, no backgrounds
		// Disable Chroma completely to prevent nested backgrounds
		CodeBlock: ansi.StyleCodeBlock{
			StyleBlock: ansi.StyleBlock{
				StylePrimitive: ansi.StylePrimitive{
					Color:           stringPtr(CPLightGray),
					BackgroundColor: stringPtr(""), // Explicitly no background
				},
				Margin: uintPtr(0),
			},
			Chroma: nil, // Disable chroma to avoid nested backgrounds
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
			Color:     stringPtr(CPPurple),
			Underline: boolPtr(true),
		},
		LinkText: ansi.StylePrimitive{
			Color: stringPtr(CPPurple),
			Bold:  boolPtr(true),
		},
	}

	return json.Marshal(style)
}
