package markdown

import (
	"encoding/json"
	"testing"

	"github.com/charmbracelet/glamour/ansi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

func TestApplyStyleSafely(t *testing.T) {
	tests := []struct {
		name     string
		style    *ansi.StylePrimitive
		color    string
		expected string
	}{
		{
			name:     "Apply to style with existing color",
			style:    &ansi.StylePrimitive{Color: stringPtr("#FF0000")},
			color:    "#00FF00",
			expected: "#00FF00",
		},
		{
			name:     "Apply to style without existing color",
			style:    &ansi.StylePrimitive{},
			color:    "#0000FF",
			expected: "#0000FF",
		},
		{
			name:     "Apply to nil style - should not panic",
			style:    nil,
			color:    "#FFFFFF",
			expected: "", // Nothing happens for nil style
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			applyStyleSafely(tt.style, tt.color)

			if tt.style != nil {
				assert.NotNil(t, tt.style.Color)
				assert.Equal(t, tt.expected, *tt.style.Color)
			}
			// Test should not panic even with nil style
		})
	}
}

// verifyStyleCustomizations checks if style customizations were applied correctly.
func verifyStyleCustomizations(t *testing.T, atmosConfig *schema.AtmosConfiguration, style ansi.StyleConfig) {
	if atmosConfig.Settings.Markdown.Document.Color != "" {
		assert.NotNil(t, style.Document.Color)
		assert.Equal(t, atmosConfig.Settings.Markdown.Document.Color, *style.Document.Color)
	}

	if atmosConfig.Settings.Markdown.Heading.Color != "" {
		assert.NotNil(t, style.Heading.Color)
		assert.Equal(t, atmosConfig.Settings.Markdown.Heading.Color, *style.Heading.Color)
	}

	if atmosConfig.Settings.Markdown.H1.Color != "" {
		assert.NotNil(t, style.H1.Color)
		assert.Equal(t, atmosConfig.Settings.Markdown.H1.Color, *style.H1.Color)
	}

	if atmosConfig.Settings.Markdown.H1.BackgroundColor != "" {
		assert.NotNil(t, style.H1.BackgroundColor)
		assert.Equal(t, atmosConfig.Settings.Markdown.H1.BackgroundColor, *style.H1.BackgroundColor)
	}
}

func TestGetDefaultStyle(t *testing.T) {
	tests := []struct {
		name        string
		atmosConfig schema.AtmosConfiguration
		expectError bool
	}{
		{
			name: "Default style with no customizations",
			atmosConfig: schema.AtmosConfiguration{
				Settings: schema.AtmosSettings{
					Markdown: schema.MarkdownSettings{},
				},
			},
			expectError: false,
		},
		{
			name: "Style with document color customization",
			atmosConfig: schema.AtmosConfiguration{
				Settings: schema.AtmosSettings{
					Markdown: schema.MarkdownSettings{
						Document: schema.MarkdownStyle{
							Color: "#FF0000",
						},
					},
				},
			},
			expectError: false,
		},
		{
			name: "Style with heading customizations",
			atmosConfig: schema.AtmosConfiguration{
				Settings: schema.AtmosSettings{
					Markdown: schema.MarkdownSettings{
						Heading: schema.MarkdownStyle{
							Color: "#00FF00",
							Bold:  true,
						},
					},
				},
			},
			expectError: false,
		},
		{
			name: "Style with H1 customizations including background",
			atmosConfig: schema.AtmosConfiguration{
				Settings: schema.AtmosSettings{
					Markdown: schema.MarkdownSettings{
						H1: schema.MarkdownStyle{
							Color:           "#0000FF",
							BackgroundColor: "#FFFFFF",
							Bold:            true,
						},
					},
				},
			},
			expectError: false,
		},
		{
			name: "Style with H2 through H6 customizations",
			atmosConfig: schema.AtmosConfiguration{
				Settings: schema.AtmosSettings{
					Markdown: schema.MarkdownSettings{
						H2: schema.MarkdownStyle{
							Color: "#FF00FF",
							Bold:  true,
						},
						H3: schema.MarkdownStyle{
							Color: "#00FFFF",
							Bold:  false,
						},
						H4: schema.MarkdownStyle{
							Color: "#FFFF00",
							Bold:  true,
						},
						H5: schema.MarkdownStyle{
							Color: "#FF00FF",
						},
						H6: schema.MarkdownStyle{
							Color: "#00FF00",
						},
					},
				},
			},
			expectError: false,
		},
		{
			name: "Style with text and paragraph customizations",
			atmosConfig: schema.AtmosConfiguration{
				Settings: schema.AtmosSettings{
					Markdown: schema.MarkdownSettings{
						Paragraph: schema.MarkdownStyle{
							Color: "#808080",
						},
						Text: schema.MarkdownStyle{
							Color: "#404040",
						},
					},
				},
			},
			expectError: false,
		},
		{
			name: "Style with code and block quote customizations",
			atmosConfig: schema.AtmosConfiguration{
				Settings: schema.AtmosSettings{
					Markdown: schema.MarkdownSettings{
						Code: schema.MarkdownStyle{
							Color:           "#00FF00",
							BackgroundColor: "#000000",
						},
						BlockQuote: schema.MarkdownStyle{
							Color: "#666666",
						},
					},
				},
			},
			expectError: false,
		},
		{
			name: "Style with list and table customizations",
			atmosConfig: schema.AtmosConfiguration{
				Settings: schema.AtmosSettings{
					Markdown: schema.MarkdownSettings{
						List: schema.MarkdownStyle{
							Color: "#FF0000",
						},
						Table: schema.MarkdownStyle{
							Color: "#0000FF",
						},
					},
				},
			},
			expectError: false,
		},
		{
			name: "Style with link customizations",
			atmosConfig: schema.AtmosConfiguration{
				Settings: schema.AtmosSettings{
					Markdown: schema.MarkdownSettings{
						Link: schema.MarkdownStyle{
							Color:     "#00FFFF",
							Underline: true,
						},
						LinkText: schema.MarkdownStyle{
							Color: "#FF00FF",
							Bold:  true,
						},
					},
				},
			},
			expectError: false,
		},
		{
			name: "Style with image alt text customization",
			atmosConfig: schema.AtmosConfiguration{
				Settings: schema.AtmosSettings{
					Markdown: schema.MarkdownSettings{
						// ImageAlt is not a field in MarkdownSettings
						// ImageAlt: schema.MarkdownStyle{
						//	Color: "#FFAA00",
						// },
					},
				},
			},
			expectError: false,
		},
		{
			name: "Style with emph and strong customizations",
			atmosConfig: schema.AtmosConfiguration{
				Settings: schema.AtmosSettings{
					Markdown: schema.MarkdownSettings{
						Emph: schema.MarkdownStyle{
							Color:  "#123456",
							Italic: true,
						},
						Strong: schema.MarkdownStyle{
							Color: "#654321",
							Bold:  true,
						},
					},
				},
			},
			expectError: false,
		},
		{
			name: "Style with horizontal rule customization",
			atmosConfig: schema.AtmosConfiguration{
				Settings: schema.AtmosSettings{
					Markdown: schema.MarkdownSettings{
						Hr: schema.MarkdownStyle{
							Color: "#AAAAAA",
						},
					},
				},
			},
			expectError: false,
		},
		{
			name: "Style with definition customizations",
			atmosConfig: schema.AtmosConfiguration{
				Settings: schema.AtmosSettings{
					Markdown: schema.MarkdownSettings{
						DefinitionList: schema.MarkdownStyle{
							Color: "#111111",
						},
						DefinitionTerm: schema.MarkdownStyle{
							Color: "#222222",
						},
						DefinitionDescription: schema.MarkdownStyle{
							Color: "#333333",
						},
					},
				},
			},
			expectError: false,
		},
		{
			name: "Style with HTML block and span customizations",
			atmosConfig: schema.AtmosConfiguration{
				Settings: schema.AtmosSettings{
					Markdown: schema.MarkdownSettings{
						HtmlBlock: schema.MarkdownStyle{
							Color:           "#444444",
							BackgroundColor: "#555555",
						},
						HtmlSpan: schema.MarkdownStyle{
							Color: "#666666",
						},
					},
				},
			},
			expectError: false,
		},
		{
			name: "Style with all major customizations",
			atmosConfig: schema.AtmosConfiguration{
				Settings: schema.AtmosSettings{
					Markdown: schema.MarkdownSettings{
						Document: schema.MarkdownStyle{
							Color: "#FFFFFF",
						},
						Heading: schema.MarkdownStyle{
							Color: "#FF0000",
							Bold:  true,
						},
						H1: schema.MarkdownStyle{
							Color:           "#00FF00",
							BackgroundColor: "#000000",
							Bold:            true,
						},
						Code: schema.MarkdownStyle{
							Color:           "#00FFFF",
							BackgroundColor: "#002222",
						},
						Link: schema.MarkdownStyle{
							Color:     "#FF00FF",
							Underline: true,
						},
						Strong: schema.MarkdownStyle{
							Color: "#FFFF00",
							Bold:  true,
						},
					},
				},
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			styleBytes, err := GetDefaultStyle(tt.atmosConfig)

			if tt.expectError {
				assert.Error(t, err)
				return
			}

			assert.NoError(t, err)
			assert.NotNil(t, styleBytes)

			// Verify the returned JSON is valid
			var style ansi.StyleConfig
			err = json.Unmarshal(styleBytes, &style)
			assert.NoError(t, err)

			// Verify customizations were applied
			verifyStyleCustomizations(t, &tt.atmosConfig, style)
		})
	}
}

func TestGetBuiltinDefaultStyle(t *testing.T) {
	t.Run("Returns valid JSON style configuration", func(t *testing.T) {
		styleBytes, err := getBuiltinDefaultStyle()
		require.NoError(t, err)
		require.NotNil(t, styleBytes)

		// Verify the returned JSON is valid
		var style ansi.StyleConfig
		err = json.Unmarshal(styleBytes, &style)
		assert.NoError(t, err)

		// Verify some expected default values are present
		assert.NotNil(t, style.Document.Color)
		assert.NotNil(t, style.Heading.Color)
	})
}

func TestPointerHelpers(t *testing.T) {
	t.Run("stringPtr returns pointer to string", func(t *testing.T) {
		value := "test"
		ptr := stringPtr(value)
		assert.NotNil(t, ptr)
		assert.Equal(t, value, *ptr)
	})

	t.Run("boolPtr returns pointer to bool", func(t *testing.T) {
		value := true
		ptr := boolPtr(value)
		assert.NotNil(t, ptr)
		assert.Equal(t, value, *ptr)

		value = false
		ptr = boolPtr(value)
		assert.NotNil(t, ptr)
		assert.Equal(t, value, *ptr)
	})

	t.Run("uintPtr returns pointer to uint", func(t *testing.T) {
		value := uint(42)
		ptr := uintPtr(value)
		assert.NotNil(t, ptr)
		assert.Equal(t, value, *ptr)

		value = uint(0)
		ptr = uintPtr(value)
		assert.NotNil(t, ptr)
		assert.Equal(t, value, *ptr)
	})
}

func TestStyleBlocksAndInline(t *testing.T) {
	tests := []struct {
		name        string
		atmosConfig schema.AtmosConfiguration
		verifyStyle func(t *testing.T, style *ansi.StyleConfig)
	}{
		{
			name: "Strong customization",
			atmosConfig: schema.AtmosConfiguration{
				Settings: schema.AtmosSettings{
					Markdown: schema.MarkdownSettings{
						Strong: schema.MarkdownStyle{
							Color: "#FF0000",
							Bold:  true,
						},
					},
				},
			},
			verifyStyle: func(t *testing.T, style *ansi.StyleConfig) {
				assert.NotNil(t, style.Strong.Color)
				assert.Equal(t, "#FF0000", *style.Strong.Color)
				assert.NotNil(t, style.Strong.Bold)
				assert.True(t, *style.Strong.Bold)
			},
		},
		{
			name: "Emph customization",
			atmosConfig: schema.AtmosConfiguration{
				Settings: schema.AtmosSettings{
					Markdown: schema.MarkdownSettings{
						Emph: schema.MarkdownStyle{
							Color:  "#00FF00",
							Italic: true,
						},
					},
				},
			},
			verifyStyle: func(t *testing.T, style *ansi.StyleConfig) {
				assert.NotNil(t, style.Emph.Color)
				assert.Equal(t, "#00FF00", *style.Emph.Color)
				assert.NotNil(t, style.Emph.Italic)
				assert.True(t, *style.Emph.Italic)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			styleBytes, err := GetDefaultStyle(tt.atmosConfig)
			require.NoError(t, err)
			require.NotNil(t, styleBytes)

			var style ansi.StyleConfig
			err = json.Unmarshal(styleBytes, &style)
			require.NoError(t, err)

			if tt.verifyStyle != nil {
				tt.verifyStyle(t, &style)
			}
		})
	}
}

func TestIndentationStyles(t *testing.T) {
	tests := []struct {
		name        string
		atmosConfig schema.AtmosConfiguration
	}{
		{
			name: "List with level indent",
			atmosConfig: schema.AtmosConfiguration{
				Settings: schema.AtmosSettings{
					Markdown: schema.MarkdownSettings{
						List: schema.MarkdownStyle{
							Color:       "#AAAAAA",
							LevelIndent: 2,
						},
					},
				},
			},
		},
		{
			name: "Code block with indentation settings",
			atmosConfig: schema.AtmosConfiguration{
				Settings: schema.AtmosSettings{
					Markdown: schema.MarkdownSettings{
						CodeBlock: schema.MarkdownStyle{
							Color:           "#00FF00",
							BackgroundColor: "#002200",
							Indent:          2,
						},
					},
				},
			},
		},
		{
			name: "Blockquote with indentation",
			atmosConfig: schema.AtmosConfiguration{
				Settings: schema.AtmosSettings{
					Markdown: schema.MarkdownSettings{
						BlockQuote: schema.MarkdownStyle{
							Color:  "#666666",
							Indent: 1,
						},
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			styleBytes, err := GetDefaultStyle(tt.atmosConfig)
			assert.NoError(t, err)
			assert.NotNil(t, styleBytes)

			// Verify JSON is valid
			var style ansi.StyleConfig
			err = json.Unmarshal(styleBytes, &style)
			assert.NoError(t, err)
		})
	}
}

func TestMarginAndPaddingStyles(t *testing.T) {
	atmosConfig := schema.AtmosConfiguration{
		Settings: schema.AtmosSettings{
			Markdown: schema.MarkdownSettings{
				H1: schema.MarkdownStyle{
					Color:  "#FFFFFF",
					Margin: 2,
				},
				CodeBlock: schema.MarkdownStyle{
					Color:  "#EEEEEE",
					Margin: 3,
				},
				List: schema.MarkdownStyle{
					Color:       "#DDDDDD",
					LevelIndent: 2,
				},
			},
		},
	}

	styleBytes, err := GetDefaultStyle(atmosConfig)
	assert.NoError(t, err)
	assert.NotNil(t, styleBytes)

	var style ansi.StyleConfig
	err = json.Unmarshal(styleBytes, &style)
	assert.NoError(t, err)

	// Verify the H1 margin was set
	assert.NotNil(t, style.H1.Margin)
	assert.Equal(t, uint(2), *style.H1.Margin)

	// Verify the CodeBlock margin was set
	assert.NotNil(t, style.CodeBlock.Margin)
	assert.Equal(t, uint(3), *style.CodeBlock.Margin)
}
