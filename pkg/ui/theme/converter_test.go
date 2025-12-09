package theme

import (
	"encoding/json"
	"testing"

	"github.com/charmbracelet/glamour/ansi"
	"github.com/stretchr/testify/assert"
)

func TestConvertToGlamourStyle(t *testing.T) {
	// Create a test theme
	testTheme := &Theme{
		Name:          "test",
		Black:         "#000000",
		Red:           "#FF0000",
		Green:         "#00FF00",
		Yellow:        "#FFFF00",
		Blue:          "#0000FF",
		Magenta:       "#FF00FF",
		Cyan:          "#00FFFF",
		White:         "#FFFFFF",
		BrightBlack:   "#808080",
		BrightRed:     "#FF8080",
		BrightGreen:   "#80FF80",
		BrightYellow:  "#FFFF80",
		BrightBlue:    "#8080FF",
		BrightMagenta: "#FF80FF",
		BrightCyan:    "#80FFFF",
		BrightWhite:   "#FFFFFF",
		Background:    "#1A1A1A",
		Foreground:    "#E0E0E0",
		Cursor:        "#E0E0E0",
		Selection:     "#404040",
		Meta: Meta{
			IsDark: true,
		},
	}

	styleBytes, err := ConvertToGlamourStyle(testTheme)
	assert.NoError(t, err)
	assert.NotNil(t, styleBytes)

	// Parse the result to verify structure
	var style ansi.StyleConfig
	err = json.Unmarshal(styleBytes, &style)
	assert.NoError(t, err)

	// Verify colors were applied
	assert.NotNil(t, style.Document.Color)
	assert.Equal(t, "#E0E0E0", *style.Document.Color)

	assert.NotNil(t, style.Heading.Color)
	assert.Equal(t, "#0000FF", *style.Heading.Color)

	assert.NotNil(t, style.H1.Color)
	assert.NotNil(t, style.H1.BackgroundColor)
	assert.Equal(t, "#0000FF", *style.H1.BackgroundColor)

	assert.NotNil(t, style.H2.Color)
	assert.Equal(t, "#FF00FF", *style.H2.Color)

	assert.NotNil(t, style.Strong.Color)
	assert.Equal(t, "#FF00FF", *style.Strong.Color)
	assert.NotNil(t, style.Strong.Bold)
	assert.True(t, *style.Strong.Bold)

	assert.NotNil(t, style.Link.Color)
	assert.Equal(t, "#0000FF", *style.Link.Color)
	assert.NotNil(t, style.Link.Underline)
	assert.True(t, *style.Link.Underline)
}

func TestCreateGlamourStyleFromTheme(t *testing.T) {
	// Test with a light theme
	lightTheme := &Theme{
		Name:       "light-test",
		Background: "#FFFFFF",
		Foreground: "#000000",
		Blue:       "#0000FF",
		Magenta:    "#FF00FF",
		Cyan:       "#00FFFF",
		White:      "#FFFFFF",
		Meta: Meta{
			IsDark: false,
		},
	}

	style := createGlamourStyleFromTheme(lightTheme)
	assert.NotNil(t, style)

	// For light themes, H1 should use white color on colored background
	assert.NotNil(t, style.H1.Color)
	assert.Equal(t, "#FFFFFF", *style.H1.Color)
	assert.NotNil(t, style.H1.BackgroundColor)
	assert.Equal(t, "#0000FF", *style.H1.BackgroundColor)

	// Test with a dark theme
	darkTheme := &Theme{
		Name:       "dark-test",
		Background: "#1A1A1A",
		Foreground: "#E0E0E0",
		Blue:       "#6495ED",
		Magenta:    "#DA70D6",
		Cyan:       "#48D1CC",
		Meta: Meta{
			IsDark: true,
		},
	}

	style = createGlamourStyleFromTheme(darkTheme)
	assert.NotNil(t, style)

	// For dark themes, H1 should use background color on colored background
	assert.NotNil(t, style.H1.Color)
	assert.Equal(t, "#1A1A1A", *style.H1.Color)
	assert.NotNil(t, style.H1.BackgroundColor)
	assert.Equal(t, "#6495ED", *style.H1.BackgroundColor)
}

func TestCreateChromaStyle(t *testing.T) {
	testTheme := &Theme{
		Foreground:  "#E0E0E0",
		Background:  "#1A1A1A",
		BrightBlack: "#808080",
		Red:         "#FF0000",
		Green:       "#00FF00",
		Yellow:      "#FFFF00",
		Blue:        "#0000FF",
		Magenta:     "#FF00FF",
		Cyan:        "#00FFFF",
	}

	chroma := createChromaStyle(testTheme)
	assert.NotNil(t, chroma)

	// Verify chroma colors
	assert.NotNil(t, chroma.Text.Color)
	assert.Equal(t, "#E0E0E0", *chroma.Text.Color)

	assert.NotNil(t, chroma.Keyword.Color)
	assert.Equal(t, "#FF00FF", *chroma.Keyword.Color)

	assert.NotNil(t, chroma.LiteralString.Color)
	assert.Equal(t, "#00FF00", *chroma.LiteralString.Color)

	assert.NotNil(t, chroma.Comment.Color)
	assert.Equal(t, "#808080", *chroma.Comment.Color)

	assert.NotNil(t, chroma.Background.BackgroundColor)
	assert.Equal(t, "#1A1A1A", *chroma.Background.BackgroundColor)
}

func TestGetGlamourStyleForTheme(t *testing.T) {
	// Test with existing theme
	styleBytes, err := GetGlamourStyleForTheme("atmos")
	assert.NoError(t, err)
	assert.NotNil(t, styleBytes)

	var style ansi.StyleConfig
	err = json.Unmarshal(styleBytes, &style)
	assert.NoError(t, err)

	// Test with non-existent theme (should fall back to default)
	styleBytes, err = GetGlamourStyleForTheme("NonExistentTheme")
	assert.NoError(t, err)
	assert.NotNil(t, styleBytes)

	err = json.Unmarshal(styleBytes, &style)
	assert.NoError(t, err)

	// Test with empty theme name (should fall back to default)
	styleBytes, err = GetGlamourStyleForTheme("")
	assert.NoError(t, err)
	assert.NotNil(t, styleBytes)
}

func TestHelperFunctions(t *testing.T) {
	// Test stringPtr
	str := "test"
	ptr := stringPtr(str)
	assert.NotNil(t, ptr)
	assert.Equal(t, str, *ptr)

	// Test boolPtr
	b := true
	bPtr := boolPtr(b)
	assert.NotNil(t, bPtr)
	assert.Equal(t, b, *bPtr)

	// Test uintPtr
	u := uint(10)
	uPtr := uintPtr(u)
	assert.NotNil(t, uPtr)
	assert.Equal(t, u, *uPtr)
}
