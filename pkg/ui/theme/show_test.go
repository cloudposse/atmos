package theme

import (
	"strings"
	"testing"
)

func TestShowTheme(t *testing.T) {
	tests := []struct {
		name        string
		themeName   string
		wantErr     bool
		errContains string
	}{
		{
			name:      "valid theme - Dracula",
			themeName: "Dracula",
			wantErr:   false,
		},
		{
			name:      "valid theme - Material",
			themeName: "Material",
			wantErr:   false,
		},
		{
			name:        "invalid theme",
			themeName:   "nonexistent-theme",
			wantErr:     true,
			errContains: "theme not found",
		},
		{
			name:        "empty theme name",
			themeName:   "",
			wantErr:     true,
			errContains: "theme not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ShowTheme(ShowThemeOptions{
				ThemeName: tt.themeName,
			})

			if tt.wantErr {
				if result.Error == nil {
					t.Errorf("ShowTheme() expected error, got nil")
					return
				}
				if tt.errContains != "" && !strings.Contains(result.Error.Error(), tt.errContains) {
					t.Errorf("ShowTheme() error = %v, want error containing %q", result.Error, tt.errContains)
				}
				return
			}

			if result.Error != nil {
				t.Errorf("ShowTheme() unexpected error = %v", result.Error)
				return
			}

			if result.Output == "" {
				t.Error("ShowTheme() output is empty")
			}

			// Verify output contains expected sections.
			expectedSections := []string{
				"Theme:",
				"COLOR PALETTE",
				"LOG OUTPUT PREVIEW",
				"MARKDOWN PREVIEW",
				"SAMPLE UI ELEMENTS",
			}

			for _, section := range expectedSections {
				if !strings.Contains(result.Output, section) {
					t.Errorf("ShowTheme() output missing section: %q", section)
				}
			}
		})
	}
}

func TestFormatColorPalette(t *testing.T) {
	// Create a test theme.
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
		Background:    "#1E1E1E",
		Foreground:    "#D4D4D4",
	}

	output := FormatColorPalette(testTheme)

	if output == "" {
		t.Error("FormatColorPalette() returned empty output")
	}

	// Verify all colors are included.
	expectedColors := []string{
		"Black",
		"Red",
		"Green",
		"Yellow",
		"Blue",
		"Magenta",
		"Cyan",
		"White",
		"Bright Black",
		"Bright Red",
		"Bright Green",
		"Bright Yellow",
		"Bright Blue",
		"Bright Magenta",
		"Bright Cyan",
		"Bright White",
		"Background",
		"Foreground",
	}

	for _, color := range expectedColors {
		if !strings.Contains(output, color) {
			t.Errorf("FormatColorPalette() missing color: %q", color)
		}
	}

	// Verify hex values are included.
	expectedHexValues := []string{
		"#000000",
		"#FF0000",
		"#00FF00",
		"#FFFF00",
		"#0000FF",
		"#FF00FF",
		"#00FFFF",
		"#FFFFFF",
	}

	for _, hex := range expectedHexValues {
		if !strings.Contains(output, hex) {
			t.Errorf("FormatColorPalette() missing hex value: %q", hex)
		}
	}
}

func TestGetContrastColor(t *testing.T) {
	tests := []struct {
		name      string
		hexColor  string
		wantColor string
	}{
		{
			name:      "black background - light text",
			hexColor:  "#000000",
			wantColor: "#ffffff",
		},
		{
			name:      "white background - dark text",
			hexColor:  "#FFFFFF",
			wantColor: "#000000",
		},
		{
			name:      "dark background - light text",
			hexColor:  "#1E1E1E",
			wantColor: "#ffffff",
		},
		{
			name:      "light background - dark text",
			hexColor:  "#F0F0F0",
			wantColor: "#000000",
		},
		{
			name:      "short hex format - white",
			hexColor:  "#FFF",
			wantColor: "#000000",
		},
		{
			name:      "short hex format - black",
			hexColor:  "#000",
			wantColor: "#ffffff",
		},
		{
			name:      "no hash prefix",
			hexColor:  "000000",
			wantColor: "#ffffff",
		},
		{
			name:      "invalid color - default to black text",
			hexColor:  "invalid",
			wantColor: "#000000",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GetContrastColor(tt.hexColor)
			if got != tt.wantColor {
				t.Errorf("GetContrastColor(%q) = %q, want %q", tt.hexColor, got, tt.wantColor)
			}
		})
	}
}

func TestGenerateLogDemo(t *testing.T) {
	// Create a test color scheme.
	scheme := &ColorScheme{
		Primary: "#00A3E0",
		Success: "#00FF00",
		Error:   "#FF0000",
		Warning: "#FFA500",

		Border:     "#5F5FD7",
		Background: "#1E1E1E",
	}

	output := GenerateLogDemo(scheme)

	if output == "" {
		t.Error("GenerateLogDemo() returned empty output")
	}

	// Verify log levels are present (4-character format for alignment).
	expectedLevels := []string{
		"DEBU",
		"INFO",
		"WARN",
		"ERRO",
	}

	for _, level := range expectedLevels {
		if !strings.Contains(output, level) {
			t.Errorf("GenerateLogDemo() missing log level: %q", level)
		}
	}

	// Verify sample messages are present.
	expectedMessages := []string{
		"Processing component",
		"Terraform init completed",
		"Resource already exists",
		"Failed to connect",
	}

	for _, msg := range expectedMessages {
		if !strings.Contains(output, msg) {
			t.Errorf("GenerateLogDemo() missing message: %q", msg)
		}
	}
}

func TestFormatThemeHeader(t *testing.T) {
	scheme := &ColorScheme{
		Primary: "#00A3E0",
		Success: "#00FF00",
		Error:   "#FF0000",
		Warning: "#FFA500",

		Border:     "#5F5FD7",
		Background: "#1E1E1E",
	}
	styles := GetStyles(scheme)

	tests := []struct {
		name      string
		theme     *Theme
		wantTitle string
		wantBadge bool
	}{
		{
			name: "recommended theme",
			theme: &Theme{
				Name: "dracula",
			},
			wantTitle: "Theme: dracula",
			wantBadge: true,
		},
		{
			name: "non-recommended theme",
			theme: &Theme{
				Name: "custom-theme",
			},
			wantTitle: "Theme: custom-theme",
			wantBadge: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output := formatThemeHeader(tt.theme, styles)

			if !strings.Contains(output, tt.wantTitle) {
				t.Errorf("formatThemeHeader() output doesn't contain %q", tt.wantTitle)
			}

			hasRecommendedBadge := strings.Contains(output, "Recommended")
			if hasRecommendedBadge != tt.wantBadge {
				t.Errorf("formatThemeHeader() recommended badge = %v, want %v", hasRecommendedBadge, tt.wantBadge)
			}
		})
	}
}

func TestFormatThemeMetadata(t *testing.T) {
	scheme := &ColorScheme{
		Primary: "#00A3E0",
		Success: "#00FF00",
		Error:   "#FF0000",
		Warning: "#FFA500",

		Border:     "#5F5FD7",
		Background: "#1E1E1E",
	}
	styles := GetStyles(scheme)

	credits := []Credit{
		{
			Name: "Test Author",
			Link: "https://example.com",
		},
	}

	tests := []struct {
		name       string
		theme      *Theme
		wantType   string
		wantSource bool
	}{
		{
			name: "dark theme with credits",
			theme: &Theme{
				Name: "test-dark",
				Meta: Meta{
					IsDark:  true,
					Credits: &credits,
				},
			},
			wantType:   "Dark",
			wantSource: true,
		},
		{
			name: "light theme without credits",
			theme: &Theme{
				Name: "test-light",
				Meta: Meta{
					IsDark:  false,
					Credits: nil,
				},
			},
			wantType:   "Light",
			wantSource: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output := formatThemeMetadata(tt.theme, styles)

			if !strings.Contains(output, tt.wantType) {
				t.Errorf("formatThemeMetadata() output doesn't contain %q", tt.wantType)
			}

			hasSource := strings.Contains(output, "Source:")
			if hasSource != tt.wantSource {
				t.Errorf("formatThemeMetadata() has source = %v, want %v", hasSource, tt.wantSource)
			}
		})
	}
}

func TestFormatUIElements(t *testing.T) {
	scheme := &ColorScheme{
		Primary: "#00A3E0",
		Success: "#00FF00",
		Error:   "#FF0000",
		Warning: "#FFA500",

		Border:     "#5F5FD7",
		Background: "#1E1E1E",
	}
	styles := GetStyles(scheme)

	output := formatUIElements(styles)

	if output == "" {
		t.Error("formatUIElements() returned empty output")
	}

	// Verify status messages are present.
	expectedMessages := []string{
		"Success message",
		"Warning message",
		"Error message",
		"Info message",
	}

	for _, msg := range expectedMessages {
		if !strings.Contains(output, msg) {
			t.Errorf("formatUIElements() missing message: %q", msg)
		}
	}

	// Verify table is present.
	if !strings.Contains(output, "Sample Table:") {
		t.Error("formatUIElements() missing sample table")
	}

	// Verify command examples are present.
	expectedCommands := []string{
		"atmos terraform plan",
		"atmos describe stacks",
	}

	for _, cmd := range expectedCommands {
		if !strings.Contains(output, cmd) {
			t.Errorf("formatUIElements() missing command: %q", cmd)
		}
	}
}

func TestFormatUsageInstructions(t *testing.T) {
	scheme := &ColorScheme{
		Primary: "#00A3E0",
		Success: "#00FF00",
		Error:   "#FF0000",
		Warning: "#FFA500",

		Border:     "#5F5FD7",
		Background: "#1E1E1E",
	}
	styles := GetStyles(scheme)

	theme := &Theme{
		Name: "dracula",
	}

	output := formatUsageInstructions(theme, styles)

	expectedStrings := []string{
		"ATMOS_THEME=dracula",
		"theme: dracula",
		"settings:",
		"terminal:",
	}

	for _, expected := range expectedStrings {
		if !strings.Contains(output, expected) {
			t.Errorf("formatUsageInstructions() missing: %q", expected)
		}
	}
}
