package list

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"

	"github.com/cloudposse/atmos/pkg/ui/theme"
)

// TestListThemesFlags tests that the list themes command has the correct flags.
func TestListThemesFlags(t *testing.T) {
	cmd := &cobra.Command{
		Use:   "themes",
		Short: "List available terminal themes (alias for 'theme list')",
		Long:  "Display available terminal themes that can be used for markdown rendering. By default shows recommended themes.\nThis is an alias for 'atmos theme list'.",
		Args:  cobra.NoArgs,
	}

	cmd.PersistentFlags().Bool("all", false, "Show all available themes (default: show only recommended themes)")

	allFlag := cmd.PersistentFlags().Lookup("all")
	assert.NotNil(t, allFlag, "Expected all flag to exist")
	assert.Equal(t, "false", allFlag.DefValue)
}

// TestListThemesValidatesArgs tests that the command validates arguments.
func TestListThemesValidatesArgs(t *testing.T) {
	cmd := &cobra.Command{
		Use:  "themes",
		Args: cobra.NoArgs,
	}

	err := cmd.ValidateArgs([]string{})
	assert.NoError(t, err, "Validation should pass with no arguments")

	err = cmd.ValidateArgs([]string{"extra"})
	assert.Error(t, err, "Validation should fail with arguments")
}

// TestListThemesCommand tests the themes command structure.
func TestListThemesCommand(t *testing.T) {
	assert.Equal(t, "themes", themesCmd.Use)
	assert.Contains(t, themesCmd.Short, "List available terminal themes")
	assert.NotNil(t, themesCmd.RunE)
	assert.NotEmpty(t, themesCmd.Example)

	// Check that NoArgs validator is set
	err := themesCmd.Args(themesCmd, []string{"unexpected"})
	assert.Error(t, err, "Should reject extra arguments")

	err = themesCmd.Args(themesCmd, []string{})
	assert.NoError(t, err, "Should accept no arguments")
}

// TestThemesOptions tests the ThemesOptions structure.
func TestThemesOptions(t *testing.T) {
	opts := &ThemesOptions{
		All: true,
	}

	assert.True(t, opts.All)
}

// TestFilterRecommendedThemes tests the filterRecommendedThemes function.
func TestFilterRecommendedThemes(t *testing.T) {
	testCases := []struct {
		name           string
		themes         []*theme.Theme
		activeTheme    string
		expectedCount  int
		shouldContain  []string
		shouldNotExist []string
	}{
		{
			name: "filter only recommended themes",
			themes: []*theme.Theme{
				{Name: "Dracula"},
				{Name: "Solarized Dark"},
				{Name: "One Dark"},
				{Name: "unknown-theme"},
			},
			activeTheme:    "",
			expectedCount:  3,
			shouldContain:  []string{"Dracula", "Solarized Dark", "One Dark"},
			shouldNotExist: []string{"unknown-theme"},
		},
		{
			name: "include active theme even if not recommended",
			themes: []*theme.Theme{
				{Name: "Dracula"},
				{Name: "One Dark"},
				{Name: "custom-theme"},
			},
			activeTheme:    "custom-theme",
			expectedCount:  3,
			shouldContain:  []string{"Dracula", "One Dark", "custom-theme"},
			shouldNotExist: []string{},
		},
		{
			name: "active theme already recommended",
			themes: []*theme.Theme{
				{Name: "Dracula"},
				{Name: "One Dark"},
			},
			activeTheme:    "Dracula",
			expectedCount:  2,
			shouldContain:  []string{"Dracula", "One Dark"},
			shouldNotExist: []string{},
		},
		{
			name: "no active theme",
			themes: []*theme.Theme{
				{Name: "Dracula"},
				{Name: "One Dark"},
				{Name: "unknown-theme"},
			},
			activeTheme:    "",
			expectedCount:  2,
			shouldContain:  []string{"Dracula", "One Dark"},
			shouldNotExist: []string{"unknown-theme"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			filtered := filterRecommendedThemes(tc.themes, tc.activeTheme)

			assert.Len(t, filtered, tc.expectedCount, "Expected %d themes", tc.expectedCount)

			// Check that expected themes are present
			for _, expectedName := range tc.shouldContain {
				found := false
				for _, theme := range filtered {
					if theme.Name == expectedName {
						found = true
						break
					}
				}
				assert.True(t, found, "Expected theme %s to be in filtered list", expectedName)
			}

			// Check that unexpected themes are not present
			for _, unexpectedName := range tc.shouldNotExist {
				found := false
				for _, theme := range filtered {
					if theme.Name == unexpectedName {
						found = true
						break
					}
				}
				assert.False(t, found, "Expected theme %s to NOT be in filtered list", unexpectedName)
			}
		})
	}
}

// TestGetThemeType tests the getThemeType function.
func TestGetThemeType(t *testing.T) {
	testCases := []struct {
		name     string
		theme    *theme.Theme
		expected string
	}{
		{
			name: "dark theme",
			theme: &theme.Theme{
				Name: "Dracula",
				Meta: theme.Meta{
					IsDark: true,
				},
			},
			expected: "Dark",
		},
		{
			name: "light theme",
			theme: &theme.Theme{
				Name: "Solarized Light",
				Meta: theme.Meta{
					IsDark: false,
				},
			},
			expected: "Light",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := getThemeType(tc.theme)
			assert.Equal(t, tc.expected, result)
		})
	}
}

// TestGetThemeSource tests the getThemeSource function.
func TestGetThemeSource(t *testing.T) {
	testCases := []struct {
		name     string
		theme    *theme.Theme
		expected string
	}{
		{
			name: "theme with link in credits",
			theme: &theme.Theme{
				Name: "Dracula",
				Meta: theme.Meta{
					Credits: &[]theme.Credit{
						{
							Name: "Dracula Theme",
							Link: "https://draculatheme.com",
						},
					},
				},
			},
			expected: "https://draculatheme.com",
		},
		{
			name: "theme with only name in credits",
			theme: &theme.Theme{
				Name: "custom",
				Meta: theme.Meta{
					Credits: &[]theme.Credit{
						{
							Name: "Custom Author",
							Link: "",
						},
					},
				},
			},
			expected: "Custom Author",
		},
		{
			name: "theme with no credits",
			theme: &theme.Theme{
				Name: "nocredit",
				Meta: theme.Meta{
					Credits: nil,
				},
			},
			expected: "",
		},
		{
			name: "theme with empty credits",
			theme: &theme.Theme{
				Name: "emptycredit",
				Meta: theme.Meta{
					Credits: &[]theme.Credit{},
				},
			},
			expected: "",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := getThemeSource(tc.theme)
			assert.Equal(t, tc.expected, result)
		})
	}
}

// TestFormatSimpleOutput tests the formatSimpleOutput function.
func TestFormatSimpleOutput(t *testing.T) {
	themes := []*theme.Theme{
		{
			Name: "Dracula",
			Meta: theme.Meta{
				IsDark: true,
				Credits: &[]theme.Credit{
					{Name: "Dracula", Link: "https://draculatheme.com"},
				},
			},
		},
	}

	opts := &ThemesOptions{All: false}
	output := formatSimpleOutput(opts, themes, "Dracula", true)

	assert.Contains(t, output, "Dracula")
	assert.Contains(t, output, "Dark")
	assert.Contains(t, output, "Active theme: Dracula")
	assert.Contains(t, output, "1 theme (recommended)")
}

// TestFormatThemesTable tests the formatThemesTable function.
func TestFormatThemesTable(t *testing.T) {
	themes := []*theme.Theme{
		{
			Name: "Dracula",
			Meta: theme.Meta{
				IsDark: true,
				Credits: &[]theme.Credit{
					{Name: "Dracula", Link: "https://draculatheme.com"},
				},
			},
		},
		{
			Name: "One Dark",
			Meta: theme.Meta{
				IsDark: true,
				Credits: &[]theme.Credit{
					{Name: "One Dark", Link: "https://onedark.pro"},
				},
			},
		},
	}

	opts := &ThemesOptions{All: true}
	output := formatThemesTable(opts, themes, "Dracula", false)

	assert.Contains(t, output, "Dracula")
	assert.Contains(t, output, "One Dark")
	assert.Contains(t, output, "2 themes available")
	assert.Contains(t, output, "Active theme: Dracula")
}
