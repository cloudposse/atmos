package theme

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetStyles(t *testing.T) {
	scheme := &ColorScheme{
		Primary:     "#0000FF",
		Secondary:   "#FF00FF",
		Success:     "#00FF00",
		Warning:     "#FFFF00",
		Error:       "#FF0000",
		TextPrimary: "#FFFFFF",
		TextMuted:   "#808080",
		Border:      "#0000FF",
	}

	styles := GetStyles(scheme)
	require.NotNil(t, styles)

	// Verify text styles are created
	assert.NotNil(t, styles.Title)
	assert.NotNil(t, styles.Heading)
	assert.NotNil(t, styles.Body)
	assert.NotNil(t, styles.Muted)

	// Verify status styles
	assert.NotNil(t, styles.Success)
	assert.NotNil(t, styles.Warning)
	assert.NotNil(t, styles.Error)
	assert.NotNil(t, styles.Info)

	// Verify nested structures
	assert.NotNil(t, styles.Pager.StatusBar)
	assert.NotNil(t, styles.TUI.ItemStyle)
	assert.NotNil(t, styles.Diff.Added)
	assert.NotNil(t, styles.Help.Heading)
}

func TestGetStyles_NilScheme(t *testing.T) {
	styles := GetStyles(nil)
	assert.Nil(t, styles)
}

func TestInitializeStyles(t *testing.T) {
	scheme := &ColorScheme{
		Primary: "#0000FF",
		Success: "#00FF00",
		Error:   "#FF0000",
	}

	InitializeStyles(scheme)
	assert.NotNil(t, CurrentStyles)
	assert.NotNil(t, CurrentStyles.Success)
}

func TestInitializeStylesFromTheme(t *testing.T) {
	err := InitializeStylesFromTheme("atmos")
	require.NoError(t, err)
	assert.NotNil(t, CurrentStyles)

	// Test invalid theme
	err = InitializeStylesFromTheme("")
	require.NoError(t, err) // Empty theme should default to "atmos"
	assert.NotNil(t, CurrentStyles)
}

func TestGetCurrentStyles(t *testing.T) {
	// Clean up environment
	os.Unsetenv("ATMOS_THEME")
	os.Unsetenv("THEME")

	styles := GetCurrentStyles()
	require.NotNil(t, styles)
	assert.NotNil(t, styles.Success)
	assert.NotNil(t, styles.Error)
}

func TestGetCurrentStyles_WithEnvironment(t *testing.T) {
	t.Setenv("ATMOS_THEME", "dracula")

	styles := GetCurrentStyles()
	require.NotNil(t, styles)
	assert.NotNil(t, styles.Success)
}

func TestGetSuccessStyle(t *testing.T) {
	// Initialize with a known theme
	InitializeStylesFromTheme("atmos")

	style := GetSuccessStyle()
	assert.NotNil(t, style)
}

func TestGetErrorStyle(t *testing.T) {
	InitializeStylesFromTheme("atmos")

	style := GetErrorStyle()
	assert.NotNil(t, style)
}

func TestGetWarningStyle(t *testing.T) {
	InitializeStylesFromTheme("atmos")

	style := GetWarningStyle()
	assert.NotNil(t, style)
}

func TestGetInfoStyle(t *testing.T) {
	InitializeStylesFromTheme("atmos")

	style := GetInfoStyle()
	assert.NotNil(t, style)
}

func TestGetDebugStyle(t *testing.T) {
	InitializeStylesFromTheme("atmos")

	style := GetDebugStyle()
	assert.NotNil(t, style)
}

func TestGetTraceStyle(t *testing.T) {
	InitializeStylesFromTheme("atmos")

	style := GetTraceStyle()
	assert.NotNil(t, style)
}

func TestGetPrimaryColor(t *testing.T) {
	color := GetPrimaryColor()
	assert.NotEmpty(t, color)
	assert.Contains(t, color, "#") // Should be hex color
}

func TestGetSuccessColor(t *testing.T) {
	color := GetSuccessColor()
	assert.NotEmpty(t, color)
	assert.Contains(t, color, "#")
}

func TestGetErrorColor(t *testing.T) {
	color := GetErrorColor()
	assert.NotEmpty(t, color)
	assert.Contains(t, color, "#")
}

func TestGetBorderColor(t *testing.T) {
	color := GetBorderColor()
	assert.NotEmpty(t, color)
	assert.Contains(t, color, "#")
}

func TestGetActiveThemeName_Precedence(t *testing.T) {
	tests := []struct {
		name          string
		atmosTheme    string
		themeEnv      string
		expectedTheme string
	}{
		{
			name:          "ATMOS_THEME takes precedence",
			atmosTheme:    "dracula",
			themeEnv:      "nord",
			expectedTheme: "dracula",
		},
		{
			name:          "THEME as fallback",
			atmosTheme:    "",
			themeEnv:      "nord",
			expectedTheme: "nord",
		},
		{
			name:          "defaults to default",
			atmosTheme:    "",
			themeEnv:      "",
			expectedTheme: "atmos",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.atmosTheme != "" {
				t.Setenv("ATMOS_THEME", tt.atmosTheme)
			} else {
				os.Unsetenv("ATMOS_THEME")
			}

			if tt.themeEnv != "" {
				t.Setenv("THEME", tt.themeEnv)
			} else {
				os.Unsetenv("THEME")
			}

			themeName := getActiveThemeName()
			assert.Equal(t, tt.expectedTheme, themeName)
		})
	}
}

func TestStyleSetStructure(t *testing.T) {
	scheme := &ColorScheme{
		Primary:             "#0000FF",
		Success:             "#00FF00",
		Error:               "#FF0000",
		TextPrimary:         "#FFFFFF",
		Border:              "#0000FF",
		BackgroundDark:      "#000000",
		BackgroundHighlight: "#222222",
		Selected:            "#00FF80",
	}

	styles := GetStyles(scheme)
	require.NotNil(t, styles)

	// Test all major style categories exist
	t.Run("text styles", func(t *testing.T) {
		assert.NotNil(t, styles.Title)
		assert.NotNil(t, styles.Heading)
		assert.NotNil(t, styles.Body)
		assert.NotNil(t, styles.Muted)
	})

	t.Run("status styles", func(t *testing.T) {
		assert.NotNil(t, styles.Success)
		assert.NotNil(t, styles.Warning)
		assert.NotNil(t, styles.Error)
		assert.NotNil(t, styles.Info)
		assert.NotNil(t, styles.Debug)
		assert.NotNil(t, styles.Trace)
	})

	t.Run("UI element styles", func(t *testing.T) {
		assert.NotNil(t, styles.Selected)
		assert.NotNil(t, styles.Link)
		assert.NotNil(t, styles.Command)
		assert.NotNil(t, styles.Description)
		assert.NotNil(t, styles.Label)
	})

	t.Run("table styles", func(t *testing.T) {
		assert.NotNil(t, styles.TableHeader)
		assert.NotNil(t, styles.TableRow)
		assert.NotNil(t, styles.TableActive)
		assert.NotNil(t, styles.TableBorder)
		assert.NotNil(t, styles.TableSpecial)
	})

	t.Run("pager styles", func(t *testing.T) {
		assert.NotNil(t, styles.Pager.StatusBar)
		assert.NotNil(t, styles.Pager.StatusBarHelp)
		assert.NotNil(t, styles.Pager.StatusBarMessage)
		assert.NotNil(t, styles.Pager.ErrorMessage)
		assert.NotNil(t, styles.Pager.Highlight)
		assert.NotNil(t, styles.Pager.HelpView)
	})

	t.Run("TUI component styles", func(t *testing.T) {
		assert.NotNil(t, styles.TUI.ItemStyle)
		assert.NotNil(t, styles.TUI.SelectedItemStyle)
		assert.NotNil(t, styles.TUI.BorderFocused)
		assert.NotNil(t, styles.TUI.BorderUnfocused)
	})

	t.Run("diff styles", func(t *testing.T) {
		assert.NotNil(t, styles.Diff.Added)
		assert.NotNil(t, styles.Diff.Removed)
		assert.NotNil(t, styles.Diff.Changed)
		assert.NotNil(t, styles.Diff.Header)
	})

	t.Run("help styles", func(t *testing.T) {
		assert.NotNil(t, styles.Help.Heading)
		assert.NotNil(t, styles.Help.CommandName)
		assert.NotNil(t, styles.Help.CommandDesc)
		assert.NotNil(t, styles.Help.FlagName)
		assert.NotNil(t, styles.Help.FlagDesc)
		assert.NotNil(t, styles.Help.FlagDataType)
		assert.NotNil(t, styles.Help.UsageBlock)
		assert.NotNil(t, styles.Help.ExampleBlock)
		assert.NotNil(t, styles.Help.Code)
	})
}
