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
		{Name: "dracula"},        // Recommended.
		{Name: "monokai"},        // Recommended.
		{Name: "solarized-dark"}, // Recommended.
		{Name: "custom-theme"},   // Not recommended.
		{Name: "another-custom"}, // Not recommended.
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
			activeTheme: "dracula",
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
				if len(row) != 4 {
					t.Errorf("buildThemeRows() row length = %d, want 4", len(row))
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
			wantStar:    true,
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

			if len(row) != 4 {
				t.Errorf("formatThemeRow() returned %d columns, want 4", len(row))
				return
			}

			// Check active indicator.
			hasActiveIndicator := strings.Contains(row[0], ">")
			if hasActiveIndicator != tt.wantActive {
				t.Errorf("formatThemeRow() active indicator = %v, want %v", hasActiveIndicator, tt.wantActive)
			}

			// Check star.
			hasStar := strings.Contains(row[1], "★")
			if hasStar != tt.wantStar {
				t.Errorf("formatThemeRow() star = %v, want %v", hasStar, tt.wantStar)
			}

			// Check type.
			if row[2] != tt.wantType {
				t.Errorf("formatThemeRow() type = %q, want %q", row[2], tt.wantType)
			}
		})
	}
}

func TestBuildFooterMessage(t *testing.T) {
	tests := []struct {
		name                string
		themeCount          int
		showingRecommended  bool
		showStars           bool
		activeTheme         string
		wantPlural          bool
		wantRecommendedNote bool
		wantStarNote        bool
		wantActiveTheme     bool
	}{
		{
			name:                "one theme, not showing recommended",
			themeCount:          1,
			showingRecommended:  false,
			showStars:           false,
			activeTheme:         "",
			wantPlural:          false,
			wantRecommendedNote: false,
			wantStarNote:        false,
			wantActiveTheme:     false,
		},
		{
			name:                "multiple themes with stars",
			themeCount:          10,
			showingRecommended:  false,
			showStars:           true,
			activeTheme:         "dracula",
			wantPlural:          true,
			wantRecommendedNote: false,
			wantStarNote:        true,
			wantActiveTheme:     true,
		},
		{
			name:                "recommended only",
			themeCount:          5,
			showingRecommended:  true,
			showStars:           false,
			activeTheme:         "",
			wantPlural:          true,
			wantRecommendedNote: true,
			wantStarNote:        false,
			wantActiveTheme:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			footer := buildFooterMessage(tt.themeCount, tt.showingRecommended, tt.showStars, tt.activeTheme)

			// Check plural.
			hasPlural := strings.Contains(footer, "themes")
			if hasPlural != tt.wantPlural {
				t.Errorf("buildFooterMessage() plural = %v, want %v", hasPlural, tt.wantPlural)
			}

			// Check recommended note.
			hasRecommendedNote := strings.Contains(footer, "recommended")
			if hasRecommendedNote != tt.wantRecommendedNote {
				t.Errorf("buildFooterMessage() recommended note = %v, want %v", hasRecommendedNote, tt.wantRecommendedNote)
			}

			// Check star note.
			hasStarNote := strings.Contains(footer, "★")
			if hasStarNote != tt.wantStarNote {
				t.Errorf("buildFooterMessage() star note = %v, want %v", hasStarNote, tt.wantStarNote)
			}

			// Check active theme.
			hasActiveTheme := strings.Contains(footer, "Active theme:")
			if hasActiveTheme != tt.wantActiveTheme {
				t.Errorf("buildFooterMessage() active theme = %v, want %v", hasActiveTheme, tt.wantActiveTheme)
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

	output := formatThemeTable(themes, "dracula", false, true)

	if output == "" {
		t.Error("formatThemeTable() returned empty output")
	}

	// Verify footer is present.
	if !strings.Contains(output, "theme") {
		t.Error("formatThemeTable() missing footer")
	}

	// Verify theme names are present.
	if !strings.Contains(output, "dracula") {
		t.Error("formatThemeTable() missing dracula theme")
	}
	if !strings.Contains(output, "monokai") {
		t.Error("formatThemeTable() missing monokai theme")
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
				if !strings.Contains(output, ">") {
					t.Error("formatSimpleThemeList() missing active indicator")
				}
			}
		})
	}
}
