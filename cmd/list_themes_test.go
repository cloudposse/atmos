package cmd

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/cloudposse/atmos/pkg/ui/theme"
)

func TestListThemes(t *testing.T) {
	themes, err := listThemes()

	assert.NoError(t, err)
	assert.NotEmpty(t, themes)

	// Verify default theme exists
	hasDefault := false
	for _, theme := range themes {
		if theme.Name == "default" {
			hasDefault = true
			break
		}
	}
	assert.True(t, hasDefault, "default theme should be present")
}

func TestFilterRecommendedThemes(t *testing.T) {
	// Create test themes
	themes := []*theme.Theme{
		{Name: "default"},
		{Name: "Dracula"},
		{Name: "NotRecommended"},
		{Name: "Catppuccin Mocha"},
	}

	recommended := filterRecommendedThemes(themes, "default")

	assert.Len(t, recommended, 3) // default, Dracula, Catppuccin Mocha

	// Verify all recommended themes are in the result
	names := make([]string, len(recommended))
	for i, t := range recommended {
		names[i] = t.Name
	}
	assert.Contains(t, names, "default")
	assert.Contains(t, names, "Dracula")
	assert.Contains(t, names, "Catppuccin Mocha")
	assert.NotContains(t, names, "NotRecommended")

	// Test with active theme that's not recommended
	recommended2 := filterRecommendedThemes(themes, "NotRecommended")
	assert.Len(t, recommended2, 4) // Should include the active theme even if not recommended

	names2 := make([]string, len(recommended2))
	for i, t := range recommended2 {
		names2[i] = t.Name
	}
	assert.Contains(t, names2, "NotRecommended")
}

func TestFormatThemesTable(t *testing.T) {
	// Create test themes
	themes := []*theme.Theme{
		{
			Name: "default",
			Meta: theme.Meta{
				IsDark: true,
				Credits: &[]theme.Credit{
					{Name: "Cloud Posse", Link: "https://cloudposse.com"},
				},
			},
		},
		{
			Name: "Dracula",
			Meta: theme.Meta{
				IsDark: true,
				Credits: &[]theme.Credit{
					{Name: "zenorocha", Link: "https://github.com/zenorocha/dracula-theme"},
				},
			},
		},
		{
			Name: "Solarized Light",
			Meta: theme.Meta{
				IsDark:  false,
				Credits: nil,
			},
		},
	}

	// Test formatting table with recommended themes
	output := formatThemesTable(themes, "default", true)
	assert.Contains(t, output, "default")
	assert.Contains(t, output, "Dracula")
	assert.Contains(t, output, "Solarized Light")
	assert.Contains(t, output, "Dark")
	assert.Contains(t, output, "Light")
	assert.Contains(t, output, "https://cloudposse.com")
	assert.Contains(t, output, "(recommended)")

	// Test formatting table with all themes
	output = formatThemesTable(themes, "default", false)
	assert.Contains(t, output, "3 themes available.")
	assert.NotContains(t, output, "(recommended)")
}

func TestFormatSimpleThemeList(t *testing.T) {
	// Create test themes
	themes := []*theme.Theme{
		{
			Name: "default",
			Meta: theme.Meta{
				IsDark: true,
				Credits: &[]theme.Credit{
					{Name: "Cloud Posse", Link: "https://cloudposse.com"},
				},
			},
		},
		{
			Name: "Dracula",
			Meta: theme.Meta{
				IsDark: true,
				Credits: &[]theme.Credit{
					{Name: "zenorocha", Link: "https://github.com/zenorocha/dracula-theme"},
				},
			},
		},
	}

	// Test simple output format (non-TTY) with stars enabled
	outputWithStars := formatSimpleThemeList(themes, "default", true, true)
	assert.Contains(t, outputWithStars, "> ") // Active indicator for default
	assert.Contains(t, outputWithStars, "default")
	assert.Contains(t, outputWithStars, "Dracula")
	assert.Contains(t, outputWithStars, "★") // Should have stars when showStars=true
	assert.Contains(t, outputWithStars, "(recommended)")

	// Test simple output format without stars
	outputNoStars := formatSimpleThemeList(themes, "default", false, false)
	assert.Contains(t, outputNoStars, "> ") // Active indicator for default
	assert.Contains(t, outputNoStars, "default")
	assert.Contains(t, outputNoStars, "Dracula")
	assert.NotContains(t, outputNoStars, "★")             // Should not have stars when showStars=false
	assert.NotContains(t, outputNoStars, "(recommended)") // No recommended message when not filtering
}

func TestGetThemeType(t *testing.T) {
	darkTheme := &theme.Theme{Meta: theme.Meta{IsDark: true}}
	lightTheme := &theme.Theme{Meta: theme.Meta{IsDark: false}}

	assert.Equal(t, "Dark", getThemeType(darkTheme))
	assert.Equal(t, "Light", getThemeType(lightTheme))
}

func TestGetThemeSource(t *testing.T) {
	testCases := []struct {
		name     string
		theme    *theme.Theme
		expected string
	}{
		{
			name: "theme with link",
			theme: &theme.Theme{
				Meta: theme.Meta{
					Credits: &[]theme.Credit{{Name: "author", Link: "https://example.com"}},
				},
			},
			expected: "https://example.com",
		},
		{
			name: "theme with name only",
			theme: &theme.Theme{
				Meta: theme.Meta{
					Credits: &[]theme.Credit{{Name: "author"}},
				},
			},
			expected: "author",
		},
		{
			name: "theme without credits",
			theme: &theme.Theme{
				Meta: theme.Meta{
					Credits: nil,
				},
			},
			expected: "",
		},
		{
			name: "theme with empty credits",
			theme: &theme.Theme{
				Meta: theme.Meta{
					Credits: &[]theme.Credit{},
				},
			},
			expected: "",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.expected, getThemeSource(tc.theme))
		})
	}
}

func TestListThemesCommand(t *testing.T) {
	// This tests that the command is properly registered
	cmd := listCmd
	found := false
	for _, subCmd := range cmd.Commands() {
		if subCmd.Name() != "themes" {
			continue
		}
		found = true
		assert.Equal(t, "themes", subCmd.Use)
		assert.Contains(t, strings.ToLower(subCmd.Short), "terminal themes")

		// Check that flags are registered
		assert.NotNil(t, subCmd.Flags().Lookup("all"))
		break
	}
	assert.True(t, found, "list themes command should be registered")
}
