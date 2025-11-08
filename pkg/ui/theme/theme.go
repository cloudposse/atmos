package theme

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// themes.json contains terminal themes from https://github.com/charmbracelet/vhs
// Licensed under MIT License
// Copyright (c) 2022 Charmbracelet, Inc
// The themes are sourced from various terminal theme projects with their respective attributions preserved in the meta.credits field.
//
//go:embed themes.json
var themesJSON []byte

// Credit represents theme author information.
type Credit struct {
	Name string `json:"name"`
	Link string `json:"link"`
}

// Meta holds theme metadata including whether the theme is designed for dark mode
// and optional credit information for the theme's creators or sources.
type Meta struct {
	IsDark  bool      `json:"isDark"`
	Credits *[]Credit `json:"credits,omitempty"`
}

// Theme represents a terminal color theme.
type Theme struct {
	Name          string `json:"name"`
	Black         string `json:"black"`
	Red           string `json:"red"`
	Green         string `json:"green"`
	Yellow        string `json:"yellow"`
	Blue          string `json:"blue"`
	Magenta       string `json:"magenta"`
	Cyan          string `json:"cyan"`
	White         string `json:"white"`
	BrightBlack   string `json:"brightBlack"`
	BrightRed     string `json:"brightRed"`
	BrightGreen   string `json:"brightGreen"`
	BrightYellow  string `json:"brightYellow"`
	BrightBlue    string `json:"brightBlue"`
	BrightMagenta string `json:"brightMagenta"`
	BrightCyan    string `json:"brightCyan"`
	BrightWhite   string `json:"brightWhite"`
	Background    string `json:"background"`
	Foreground    string `json:"foreground"`
	Cursor        string `json:"cursor"`
	Selection     string `json:"selection"`
	Meta          Meta   `json:"meta"`
}

// RecommendedThemes is a curated list of themes that work well with Atmos.
var RecommendedThemes = []string{
	"atmos",            // Atmos native theme
	"Dracula",          // Popular dark theme with excellent contrast
	"Catppuccin Mocha", // Modern dark theme, easy on the eyes
	"Catppuccin Latte", // Modern light theme
	"Tokyo Night",      // Clean, vibrant dark theme
	"Nord",             // Arctic-inspired dark theme
	"Gruvbox Dark",     // Retro dark theme with warm colors
	"Gruvbox Light",    // Retro light theme with warm colors
	"GitHub Dark",      // GitHub's dark mode
	"GitHub Light",     // GitHub's light mode
	"One Dark",         // Atom's iconic dark theme
	"Solarized Dark",   // Precision dark colors
	"Solarized Light",  // Precision light colors
	"Material",         // Material Design inspired
}

// IsRecommended checks if a theme is in the recommended list.
func IsRecommended(themeName string) bool {
	for _, recommended := range RecommendedThemes {
		if strings.EqualFold(recommended, themeName) {
			return true
		}
	}
	return false
}

// LoadThemes loads all themes from the embedded JSON file.
func LoadThemes() ([]*Theme, error) {
	var themes []*Theme
	if err := json.Unmarshal(themesJSON, &themes); err != nil {
		return nil, fmt.Errorf("failed to unmarshal themes: %w", err)
	}
	return themes, nil
}

// SortThemes sorts themes alphabetically by name.
func SortThemes(themes []*Theme) {
	sort.Slice(themes, func(i, j int) bool {
		return strings.ToLower(themes[i].Name) < strings.ToLower(themes[j].Name)
	})
}

// FilterRecommended returns only recommended themes from the given list.
func FilterRecommended(themes []*Theme) []*Theme {
	var recommended []*Theme
	for _, t := range themes {
		if IsRecommended(t.Name) {
			recommended = append(recommended, t)
		}
	}
	return recommended
}

// FindTheme searches for a theme by name (case-insensitive).
func FindTheme(themes []*Theme, name string) (*Theme, bool) {
	for _, t := range themes {
		if strings.EqualFold(t.Name, name) {
			return t, true
		}
	}
	return nil, false
}
