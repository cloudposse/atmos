package theme

import (
	"strings"
	"testing"
)

func TestListThemes(t *testing.T) {
	tests := []struct {
		name            string
		opts            ListThemesOptions
		wantErr         bool
		wantRecommended bool
	}{
		{
			name: "all themes",
			opts: ListThemesOptions{
				RecommendedOnly: false,
				ActiveTheme:     "",
			},
			wantErr:         false,
			wantRecommended: false,
		},
		{
			name: "recommended only",
			opts: ListThemesOptions{
				RecommendedOnly: true,
				ActiveTheme:     "",
			},
			wantErr:         false,
			wantRecommended: true,
		},
		{
			name: "with active theme",
			opts: ListThemesOptions{
				RecommendedOnly: false,
				ActiveTheme:     "dracula",
			},
			wantErr:         false,
			wantRecommended: false,
		},
		{
			name: "recommended with non-recommended active",
			opts: ListThemesOptions{
				RecommendedOnly: true,
				ActiveTheme:     "custom-theme",
			},
			wantErr:         false,
			wantRecommended: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ListThemes(tt.opts)

			if tt.wantErr {
				if result.Error == nil {
					t.Error("ListThemes() expected error, got nil")
				}
				return
			}

			if result.Error != nil {
				t.Errorf("ListThemes() unexpected error = %v", result.Error)
				return
			}

			if result.Output == "" {
				t.Error("ListThemes() output is empty")
			}

			// Verify output contains theme count.
			if !strings.Contains(result.Output, "theme") {
				t.Error("ListThemes() output missing theme count")
			}

			if tt.wantRecommended {
				if !strings.Contains(result.Output, "recommended") {
					t.Error("ListThemes() output should mention recommended themes")
				}
			}

			if tt.opts.ActiveTheme != "" {
				if !strings.Contains(result.Output, "Active theme:") {
					t.Error("ListThemes() output missing active theme indicator")
				}
			}
		})
	}
}

func TestFilterRecommendedWithActive(t *testing.T) {
	// Create test themes.
	allThemes := []*Theme{
		{Name: "Dracula"},          // Recommended.
		{Name: "Catppuccin Mocha"}, // Recommended.
		{Name: "Catppuccin Latte"}, // Recommended.
		{Name: "custom-theme"},     // Not recommended.
		{Name: "another-custom"},   // Not recommended.
	}

	tests := []struct {
		name        string
		activeTheme string
		wantCount   int
		wantActive  bool
	}{
		{
			name:        "no active theme",
			activeTheme: "",
			wantCount:   3, // Only recommended.
			wantActive:  false,
		},
		{
			name:        "active is recommended",
			activeTheme: "Dracula",
			wantCount:   3, // Only recommended.
			wantActive:  true,
		},
		{
			name:        "active is not recommended",
			activeTheme: "custom-theme",
			wantCount:   4, // Recommended + active.
			wantActive:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := filterRecommended(allThemes, tt.activeTheme)

			if len(result) != tt.wantCount {
				t.Errorf("filterRecommended() count = %d, want %d", len(result), tt.wantCount)
			}

			if tt.wantActive && tt.activeTheme != "" {
				found := false
				for _, theme := range result {
					if strings.EqualFold(theme.Name, tt.activeTheme) {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("filterRecommended() missing active theme %q", tt.activeTheme)
				}
			}

			// Verify themes are sorted.
			for i := 1; i < len(result); i++ {
				if result[i-1].Name > result[i].Name {
					t.Error("filterRecommended() result is not sorted")
					break
				}
			}
		})
	}
}

func TestBuildThemeRows(t *testing.T) {
	themes := []*Theme{
		{
			Name: "dracula",
			Meta: Meta{IsDark: true},
		},
		{
			Name: "solarized-light",
			Meta: Meta{IsDark: false},
		},
	}

	tests := []struct {
		name        string
		activeTheme string
		showStars   bool
	}{
		{
			name:        "no active, no stars",
			activeTheme: "",
			showStars:   false,
		},
		{
			name:        "with active, with stars",
			activeTheme: "dracula",
			showStars:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rows := buildThemeRows(themes, tt.activeTheme, tt.showStars)

			if len(rows) != len(themes) {
				t.Errorf("buildThemeRows() count = %d, want %d", len(rows), len(themes))
			}

			for _, row := range rows {
				if len(row) != 5 {
					t.Errorf("buildThemeRows() row length = %d, want 5", len(row))
				}
			}
		})
	}
}

func TestFormatThemeRow(t *testing.T) {
	tests := []struct {
		name        string
		theme       *Theme
		activeTheme string
		showStars   bool
		wantActive  bool
		wantStar    bool
		wantType    string
	}{
		{
			name: "active recommended dark theme with stars",
			theme: &Theme{
				Name: "dracula",
				Meta: Meta{IsDark: true},
			},
			activeTheme: "dracula",
			showStars:   true,
			wantActive:  true,
			wantStar:    false, // Active takes precedence over star
			wantType:    "Dark",
		},
		{
			name: "inactive light theme without stars",
			theme: &Theme{
				Name: "custom-light",
				Meta: Meta{IsDark: false},
			},
			activeTheme: "dracula",
			showStars:   false,
			wantActive:  false,
			wantStar:    false,
			wantType:    "Light",
		},
		{
			name: "case-insensitive active check",
			theme: &Theme{
				Name: "Dracula",
				Meta: Meta{IsDark: true},
			},
			activeTheme: "dracula",
			showStars:   false,
			wantActive:  true,
			wantStar:    false,
			wantType:    "Dark",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			row := formatThemeRow(tt.theme, tt.activeTheme, tt.showStars)

			if len(row) != 5 {
				t.Errorf("formatThemeRow() returned %d columns, want 5", len(row))
				return
			}

			// Check active indicator (now uses IconActive instead of ">").
			hasActiveIndicator := strings.Contains(row[0], IconActive)
			if hasActiveIndicator != tt.wantActive {
				t.Errorf("formatThemeRow() active indicator = %v, want %v", hasActiveIndicator, tt.wantActive)
			}

			// Check star (now in status column instead of name column).
			hasStar := strings.Contains(row[0], IconRecommended)
			if hasStar != tt.wantStar {
				t.Errorf("formatThemeRow() star = %v, want %v", hasStar, tt.wantStar)
			}

			// Check type (now at index 2).
			if row[2] != tt.wantType {
				t.Errorf("formatThemeRow() type = %q, want %q", row[2], tt.wantType)
			}
		})
	}
}

func TestGetThemeTypeString(t *testing.T) {
	tests := []struct {
		name  string
		theme *Theme
		want  string
	}{
		{
			name: "dark theme",
			theme: &Theme{
				Meta: Meta{IsDark: true},
			},
			want: "Dark",
		},
		{
			name: "light theme",
			theme: &Theme{
				Meta: Meta{IsDark: false},
			},
			want: "Light",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := getThemeTypeString(tt.theme)
			if got != tt.want {
				t.Errorf("getThemeTypeString() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestGetThemeSourceString(t *testing.T) {
	tests := []struct {
		name  string
		theme *Theme
		want  string
	}{
		{
			name: "theme with link",
			theme: &Theme{
				Meta: Meta{
					Credits: &[]Credit{
						{
							Name: "Author",
							Link: "https://example.com",
						},
					},
				},
			},
			want: "https://example.com",
		},
		{
			name: "theme with name only",
			theme: &Theme{
				Meta: Meta{
					Credits: &[]Credit{
						{
							Name: "Author",
							Link: "",
						},
					},
				},
			},
			want: "Author",
		},
		{
			name: "theme without credits",
			theme: &Theme{
				Meta: Meta{
					Credits: nil,
				},
			},
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := getThemeSourceString(tt.theme)
			if got != tt.want {
				t.Errorf("getThemeSourceString() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestFormatThemeTable(t *testing.T) {
	themes := []*Theme{
		{
			Name: "dracula",
			Meta: Meta{IsDark: true},
		},
		{
			Name: "monokai",
			Meta: Meta{IsDark: true},
		},
	}

	output := formatThemeTable(themes, "dracula", true)

	if output == "" {
		t.Error("formatThemeTable() returned empty output")
	}

	// Verify theme names are present.
	if !strings.Contains(output, "dracula") {
		t.Error("formatThemeTable() missing dracula theme")
	}
	if !strings.Contains(output, "monokai") {
		t.Error("formatThemeTable() missing monokai theme")
	}

	// Verify output ends with newline for spacing before footer.
	if !strings.HasSuffix(output, "\n") {
		t.Error("formatThemeTable() should end with newline")
	}
}

func TestFormatSimpleThemeList(t *testing.T) {
	themes := []*Theme{
		{
			Name: "dracula",
			Meta: Meta{IsDark: true},
		},
		{
			Name: "monokai",
			Meta: Meta{IsDark: true},
		},
	}

	tests := []struct {
		name               string
		activeTheme        string
		showingRecommended bool
		showStars          bool
	}{
		{
			name:               "all themes with stars",
			activeTheme:        "dracula",
			showingRecommended: false,
			showStars:          true,
		},
		{
			name:               "recommended only without stars",
			activeTheme:        "",
			showingRecommended: true,
			showStars:          false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output := formatSimpleThemeList(themes, tt.activeTheme, tt.showingRecommended, tt.showStars)

			if output == "" {
				t.Error("formatSimpleThemeList() returned empty output")
			}

			// Verify header is present.
			if !strings.Contains(output, "Name") || !strings.Contains(output, "Type") {
				t.Error("formatSimpleThemeList() missing header")
			}

			// Verify separator is present.
			if !strings.Contains(output, "=") {
				t.Error("formatSimpleThemeList() missing separator")
			}

			// Verify theme names are present.
			if !strings.Contains(output, "dracula") {
				t.Error("formatSimpleThemeList() missing dracula theme")
			}
			if !strings.Contains(output, "monokai") {
				t.Error("formatSimpleThemeList() missing monokai theme")
			}

			// Verify active indicator if applicable.
			if tt.activeTheme != "" {
				if !strings.Contains(output, IconActive) {
					t.Error("formatSimpleThemeList() missing active indicator")
				}
			}
		})
	}
}
