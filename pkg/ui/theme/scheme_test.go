package theme

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGenerateColorScheme(t *testing.T) {
	tests := []struct {
		name     string
		theme    *Theme
		validate func(*testing.T, ColorScheme)
	}{
		{
			name: "dark theme color mapping",
			theme: &Theme{
				Name:          "test-dark",
				Blue:          "#0000FF",
				Magenta:       "#FF00FF",
				Green:         "#00FF00",
				Yellow:        "#FFFF00",
				Red:           "#FF0000",
				White:         "#FFFFFF",
				BrightBlack:   "#808080",
				Background:    "#000000",
				BrightBlue:    "#0080FF",
				BrightGreen:   "#00FF80",
				BrightMagenta: "#FF00FF",
				BrightYellow:  "#FFFF80",
				Cyan:          "#00FFFF",
				Meta:          Meta{IsDark: true},
			},
			validate: func(t *testing.T, scheme ColorScheme) {
				assert.Equal(t, "#0000FF", scheme.Primary)
				assert.Equal(t, "#FF00FF", scheme.Secondary)
				assert.Equal(t, "#00FF00", scheme.Success)
				assert.Equal(t, "#FFFF00", scheme.Warning)
				assert.Equal(t, "#FF0000", scheme.Error)
				assert.Equal(t, "#FFFFFF", scheme.TextPrimary) // White for dark theme
				assert.Equal(t, "#808080", scheme.TextSecondary)
				assert.Equal(t, "#0000FF", scheme.Border)
				assert.Equal(t, "#0080FF", scheme.Link)
				assert.Equal(t, "#00FF80", scheme.Selected)
				assert.Equal(t, "#FF00FF", scheme.Highlight)
				assert.Equal(t, "#FFFF80", scheme.Gold)
				assert.Equal(t, "#00FFFF", scheme.LogDebug)
			},
		},
		{
			name: "light theme color mapping",
			theme: &Theme{
				Name:          "test-light",
				Blue:          "#0000FF",
				Magenta:       "#FF00FF",
				Green:         "#00FF00",
				Yellow:        "#FFFF00",
				Red:           "#FF0000",
				White:         "#FFFFFF",
				Black:         "#000000",
				BrightBlack:   "#808080",
				Background:    "#FFFFFF",
				BrightBlue:    "#0080FF",
				BrightGreen:   "#00FF80",
				BrightMagenta: "#FF00FF",
				BrightYellow:  "#FFFF80",
				Cyan:          "#00FFFF",
				Meta:          Meta{IsDark: false},
			},
			validate: func(t *testing.T, scheme ColorScheme) {
				assert.Equal(t, "#000000", scheme.TextPrimary) // Black for light theme
				assert.Equal(t, "#808080", scheme.TextSecondary)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			scheme := GenerateColorScheme(tt.theme)
			tt.validate(t, scheme)
		})
	}
}

func TestGetChromaThemeForAtmosTheme(t *testing.T) {
	tests := []struct {
		name           string
		theme          *Theme
		expectedChroma string
	}{
		{
			name:           "dracula theme",
			theme:          &Theme{Name: "dracula", Meta: Meta{IsDark: true}},
			expectedChroma: "dracula",
		},
		{
			name:           "nord theme",
			theme:          &Theme{Name: "nord", Meta: Meta{IsDark: true}},
			expectedChroma: "nord",
		},
		{
			name:           "github-light theme",
			theme:          &Theme{Name: "github-light", Meta: Meta{IsDark: false}},
			expectedChroma: "github",
		},
		{
			name:           "unknown dark theme defaults to dracula",
			theme:          &Theme{Name: "unknown-dark", Meta: Meta{IsDark: true}},
			expectedChroma: "dracula",
		},
		{
			name:           "unknown light theme defaults to github",
			theme:          &Theme{Name: "unknown-light", Meta: Meta{IsDark: false}},
			expectedChroma: "github",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			chroma := getChromaThemeForAtmosTheme(tt.theme)
			assert.Equal(t, tt.expectedChroma, chroma)
		})
	}
}

func TestGetColorSchemeForTheme(t *testing.T) {
	tests := []struct {
		name        string
		themeName   string
		expectError bool
	}{
		{
			name:        "valid theme - default",
			themeName:   "default",
			expectError: false,
		},
		{
			name:        "valid theme - dracula",
			themeName:   "dracula",
			expectError: false,
		},
		{
			name:        "invalid theme falls back to default",
			themeName:   "nonexistent-theme",
			expectError: false,
		},
		{
			name:        "empty theme uses default",
			themeName:   "",
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			scheme, err := GetColorSchemeForTheme(tt.themeName)
			if tt.expectError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.NotNil(t, scheme)
				// Verify essential fields are set
				assert.NotEmpty(t, scheme.Primary)
				assert.NotEmpty(t, scheme.Success)
				assert.NotEmpty(t, scheme.Error)
				assert.NotEmpty(t, scheme.ChromaTheme)
			}
		})
	}
}

func TestColorSchemeSemanticMapping(t *testing.T) {
	// Test that semantic colors are properly mapped from ANSI colors
	theme := &Theme{
		Name:          "test",
		Blue:          "#0000FF",
		Magenta:       "#FF00FF",
		Green:         "#00FF00",
		Yellow:        "#FFFF00",
		Red:           "#FF0000",
		Cyan:          "#00FFFF",
		White:         "#FFFFFF",
		BrightBlack:   "#808080",
		BrightBlue:    "#8080FF",
		BrightGreen:   "#80FF80",
		BrightMagenta: "#FF80FF",
		BrightYellow:  "#FFFF80",
		Background:    "#000000",
		Meta:          Meta{IsDark: true},
	}

	scheme := GenerateColorScheme(theme)

	// Core semantic colors mapped from ANSI
	assert.Equal(t, theme.Blue, scheme.Primary, "Primary should be Blue")
	assert.Equal(t, theme.Magenta, scheme.Secondary, "Secondary should be Magenta")
	assert.Equal(t, theme.Green, scheme.Success, "Success should be Green")
	assert.Equal(t, theme.Yellow, scheme.Warning, "Warning should be Yellow")
	assert.Equal(t, theme.Red, scheme.Error, "Error should be Red")

	// Bright variants for highlights
	assert.Equal(t, theme.BrightBlue, scheme.Link, "Link should be BrightBlue")
	assert.Equal(t, theme.BrightGreen, scheme.Selected, "Selected should be BrightGreen")
	assert.Equal(t, theme.BrightMagenta, scheme.Highlight, "Highlight should be BrightMagenta")
	assert.Equal(t, theme.BrightYellow, scheme.Gold, "Gold should be BrightYellow")

	// Log levels use standard colors as backgrounds
	assert.Equal(t, theme.Cyan, scheme.LogDebug, "LogDebug should be Cyan")
	assert.Equal(t, theme.Blue, scheme.LogInfo, "LogInfo should be Blue")
	assert.Equal(t, theme.Yellow, scheme.LogWarning, "LogWarning should be Yellow")
	assert.Equal(t, theme.Red, scheme.LogError, "LogError should be Red")
}
