package theme

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewRegistry(t *testing.T) {
	registry, err := NewRegistry()
	
	assert.NoError(t, err)
	assert.NotNil(t, registry)
	assert.Greater(t, len(registry.themes), 300)
	assert.Greater(t, len(registry.sorted), 300)
	
	// Verify default theme exists
	defaultTheme, exists := registry.Get("default")
	assert.True(t, exists)
	assert.NotNil(t, defaultTheme)
	assert.Equal(t, "default", defaultTheme.Name)
}

func TestRegistryGet(t *testing.T) {
	registry, err := NewRegistry()
	assert.NoError(t, err)
	
	// Test exact match
	theme, exists := registry.Get("default")
	assert.True(t, exists)
	assert.Equal(t, "default", theme.Name)
	
	// Test case-insensitive
	theme, exists = registry.Get("DEFAULT")
	assert.True(t, exists)
	assert.Equal(t, "default", theme.Name)
	
	// Test with spaces
	theme, exists = registry.Get("catppuccin mocha")
	assert.True(t, exists)
	assert.Equal(t, "Catppuccin Mocha", theme.Name)
	
	// Test non-existent theme
	theme, exists = registry.Get("NonExistentTheme")
	assert.False(t, exists)
	assert.Nil(t, theme)
}

func TestRegistryGetOrDefault(t *testing.T) {
	registry, err := NewRegistry()
	assert.NoError(t, err)
	
	// Test existing theme
	theme := registry.GetOrDefault("Dracula")
	assert.NotNil(t, theme)
	assert.Equal(t, "Dracula", theme.Name)
	
	// Test non-existent theme returns default
	theme = registry.GetOrDefault("NonExistentTheme")
	assert.NotNil(t, theme)
	assert.Equal(t, "default", theme.Name)
	
	// Test empty string returns default
	theme = registry.GetOrDefault("")
	assert.NotNil(t, theme)
	assert.Equal(t, "default", theme.Name)
}

func TestRegistryList(t *testing.T) {
	registry, err := NewRegistry()
	assert.NoError(t, err)
	
	themes := registry.List()
	assert.NotEmpty(t, themes)
	assert.Greater(t, len(themes), 300)
	
	// Verify themes are sorted alphabetically
	for i := 1; i < len(themes); i++ {
		assert.True(t, 
			strings.ToLower(themes[i-1].Name) <= strings.ToLower(themes[i].Name),
			"Themes should be sorted: %s should come before %s",
			themes[i-1].Name, themes[i].Name,
		)
	}
}

func TestRegistryListRecommended(t *testing.T) {
	registry, err := NewRegistry()
	assert.NoError(t, err)
	
	recommended := registry.ListRecommended()
	assert.NotEmpty(t, recommended)
	
	// All returned themes should be recommended
	for _, theme := range recommended {
		assert.True(t, IsRecommended(theme.Name), 
			"Theme %s should be recommended", theme.Name)
	}
	
	// Check that specific recommended themes are present
	themeNames := make(map[string]bool)
	for _, theme := range recommended {
		themeNames[theme.Name] = true
	}
	
	assert.True(t, themeNames["default"])
	assert.True(t, themeNames["Dracula"])
}

func TestRegistrySearch(t *testing.T) {
	registry, err := NewRegistry()
	assert.NoError(t, err)
	
	// Test empty query returns all themes
	results := registry.Search("")
	assert.Equal(t, len(registry.sorted), len(results))
	
	// Test partial match
	results = registry.Search("dark")
	assert.NotEmpty(t, results)
	for _, theme := range results {
		assert.True(t, 
			strings.Contains(strings.ToLower(theme.Name), "dark"),
			"Theme %s should contain 'dark'", theme.Name,
		)
	}
	
	// Test case-insensitive search
	results = registry.Search("DRACULA")
	assert.NotEmpty(t, results)
	found := false
	for _, theme := range results {
		if theme.Name == "Dracula" {
			found = true
			break
		}
	}
	assert.True(t, found, "Should find Dracula theme")
	
	// Test search with no results
	results = registry.Search("XYZ123NonExistent")
	assert.Empty(t, results)
}

func TestRegistryCount(t *testing.T) {
	registry, err := NewRegistry()
	assert.NoError(t, err)
	
	count := registry.Count()
	assert.Greater(t, count, 300)
	assert.Equal(t, len(registry.themes), count)
}

func TestRegistryCountRecommended(t *testing.T) {
	registry, err := NewRegistry()
	assert.NoError(t, err)
	
	count := registry.CountRecommended()
	assert.Greater(t, count, 0)
	assert.LessOrEqual(t, count, len(RecommendedThemes))
	
	// Verify count matches actual recommended themes
	actualCount := 0
	for _, themeName := range RecommendedThemes {
		if _, exists := registry.Get(themeName); exists {
			actualCount++
		}
	}
	assert.Equal(t, actualCount, count)
}