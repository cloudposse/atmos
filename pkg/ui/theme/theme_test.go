package theme

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestLoadThemes(t *testing.T) {
	themes, err := LoadThemes()

	assert.NoError(t, err)
	assert.NotEmpty(t, themes)
	assert.GreaterOrEqual(t, len(themes), 200, "Should have at least 200 themes")

	// Find and verify the default theme
	var defaultTheme *Theme
	for _, theme := range themes {
		if theme.Name == "atmos" {
			defaultTheme = theme
			break
		}
	}
	assert.NotNil(t, defaultTheme, "Default theme should exist")
	assert.True(t, defaultTheme.Meta.IsDark, "Default theme should be dark")
}

func TestIsRecommended(t *testing.T) {
	assert.True(t, IsRecommended("atmos"))
	assert.True(t, IsRecommended("Dracula"))
	assert.True(t, IsRecommended("Catppuccin Mocha"))
	assert.True(t, IsRecommended("dracula")) // Case insensitive
	assert.True(t, IsRecommended("DRACULA")) // Case insensitive
	assert.False(t, IsRecommended("NotATheme"))
	assert.False(t, IsRecommended(""))
}

func TestSortThemes(t *testing.T) {
	themes := []*Theme{
		{Name: "Zebra"},
		{Name: "Apple"},
		{Name: "atmos"},
		{Name: "Banana"},
	}

	SortThemes(themes)

	// Sort order: case-insensitive alphabetical
	// So: Apple, atmos, Banana, Zebra
	assert.Equal(t, "Apple", themes[0].Name)
	assert.Equal(t, "atmos", themes[1].Name)
	assert.Equal(t, "Banana", themes[2].Name)
	assert.Equal(t, "Zebra", themes[3].Name)
}

func TestFilterRecommended(t *testing.T) {
	themes := []*Theme{
		{Name: "atmos"},
		{Name: "NotRecommended1"},
		{Name: "Dracula"},
		{Name: "NotRecommended2"},
		{Name: "Catppuccin Mocha"},
	}

	recommended := FilterRecommended(themes)

	assert.Len(t, recommended, 3)
	assert.Equal(t, "atmos", recommended[0].Name)
	assert.Equal(t, "Dracula", recommended[1].Name)
	assert.Equal(t, "Catppuccin Mocha", recommended[2].Name)
}

func TestFindTheme(t *testing.T) {
	themes := []*Theme{
		{Name: "atmos"},
		{Name: "Dracula"},
		{Name: "Solarized Dark"},
	}

	// Test exact match
	theme, found := FindTheme(themes, "atmos")
	assert.True(t, found)
	assert.Equal(t, "atmos", theme.Name)

	// Test case-insensitive match
	theme, found = FindTheme(themes, "DRACULA")
	assert.True(t, found)
	assert.Equal(t, "Dracula", theme.Name)

	// Test with spaces
	theme, found = FindTheme(themes, "solarized dark")
	assert.True(t, found)
	assert.Equal(t, "Solarized Dark", theme.Name)

	// Test not found
	theme, found = FindTheme(themes, "NotATheme")
	assert.False(t, found)
	assert.Nil(t, theme)
}

func TestThemeStructure(t *testing.T) {
	themes, err := LoadThemes()
	assert.NoError(t, err)

	// Find the default theme
	var defaultTheme *Theme
	for _, theme := range themes {
		if theme.Name == "atmos" {
			defaultTheme = theme
			break
		}
	}
	assert.NotNil(t, defaultTheme, "Default theme should exist")

	// Verify the structure of the default theme
	assert.Equal(t, "atmos", defaultTheme.Name)
	assert.NotEmpty(t, defaultTheme.Background)
	assert.NotEmpty(t, defaultTheme.Foreground)
	assert.NotEmpty(t, defaultTheme.Black)
	assert.NotEmpty(t, defaultTheme.Red)
	assert.NotEmpty(t, defaultTheme.Green)
	assert.NotEmpty(t, defaultTheme.Yellow)
	assert.NotEmpty(t, defaultTheme.Blue)
	assert.NotEmpty(t, defaultTheme.Magenta)
	assert.NotEmpty(t, defaultTheme.Cyan)
	assert.NotEmpty(t, defaultTheme.White)

	// Check that bright colors are present
	assert.NotEmpty(t, defaultTheme.BrightBlack)
	assert.NotEmpty(t, defaultTheme.BrightRed)
	assert.NotEmpty(t, defaultTheme.BrightGreen)
	assert.NotEmpty(t, defaultTheme.BrightYellow)
	assert.NotEmpty(t, defaultTheme.BrightBlue)
	assert.NotEmpty(t, defaultTheme.BrightMagenta)
	assert.NotEmpty(t, defaultTheme.BrightCyan)
	assert.NotEmpty(t, defaultTheme.BrightWhite)

	// Check metadata
	assert.True(t, defaultTheme.Meta.IsDark)
	assert.NotNil(t, defaultTheme.Meta.Credits)
	assert.Len(t, *defaultTheme.Meta.Credits, 1)
	assert.Equal(t, "Cloud Posse", (*defaultTheme.Meta.Credits)[0].Name)
	assert.Equal(t, "https://atmos.tools", (*defaultTheme.Meta.Credits)[0].Link)
}
